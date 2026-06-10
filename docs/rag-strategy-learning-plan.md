# RAG Strategy Learning Plan

An iterative, learn-by-building guide to improving RAG quality through chunking and retrieval strategies.

---

## Approach

Each step introduces one RAG concept, implements it in the existing registry pattern (`RegisterChunker` / `RegisterRetriever`), and measures the impact using the existing eval pipeline. Steps build on each other conceptually but are implemented independently — you can skip or reorder freely.

```
flow:
  Step N: Learn concept → implement strategy → run eval → compare metrics → repeat
```

The eval pipeline produces: HitRate@K, MRR, NDCG@K, Precision@K, Recall@K, and answer quality scores — so every change is measurable.

---

## Step 0: Baseline

Run eval against the current implementation to establish a control.

- **Chunker**: `fixed` (word-window, size=512, overlap=64)
- **Retriever**: `naive-search` (embed query → nearest neighbor in dense vector space)
- **Run**: `POST /api/v1/workflows/eval` with a representative eval dataset
- **Save**: baseline metrics as a comment in this doc

---

## Step 1: Recursive Chunker

**Concept**: Document structure matters for retrieval quality. A word-window split can cut a section in half and scatter a single topic across multiple random chunks. By respecting the document's heading hierarchy, each chunk maps to a self-contained topic, making it easier for the retriever to find the right match.

**How it works** — The `ElementReader` already yields structured elements:

```
Element{Kind: "heading", Level: 1, Text: "# Travel Policy"}
Element{Kind: "heading", Level: 2, Text: "## Booking Flights"}
Element{Kind: "paragraph", Text: "Employees must use the corporate portal..."}
```

The recursive chunker:

1. Tracks an open section stack (heading → subsection → sub-subsection)
2. Walks elements sequentially
3. When a heading is encountered, finalizes the current section's chunk and starts a new one
4. If a section's text exceeds `maxSize`, falls back to word-window splitting within that section
5. Each chunk includes the full section heading path as prefix (e.g., `"Travel Policy > Booking Flights > ..."`)

**Implementation**: `internal/chunker/recursive.go`

```go
type RecursiveChunker struct {
    MaxSize   int
    MinSize   int  // discard sections smaller than this
}

func NewRecursiveChunker(maxSize, minSize int) *RecursiveChunker

func (c *RecursiveChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
```

**Registration**:

```go
func init() {
    chunker.RegisterChunker("recursive", func(size, overlap int) chunker.Chunker {
        return NewRecursiveChunker(size, overlap)
    })
}
```

**What you learn**: Why naive fixed-size chunking loses topical coherence; how document structure (headings, sections) correlates with retrieval relevance; how to handle edge cases (nested sections, sections with no headings, oversized sections).

**Expected impact**: Recall@K improves because relevant content is concentrated in fewer, more coherent chunks rather than diluted across arbitrary word boundaries.

---

## Step 2: MMR Reranker (Retriever Strategy)

**Concept**: Top-K results from a similarity search are often redundant — the top 5 might all be variations of the same paragraph. MMR (Maximal Marginal Relevance) balances relevance against diversity so the final set covers more distinct content.

**How it works**:

1. Run the base search strategy (e.g., `naive-search`) to get initial top-K results (e.g., K=20)
2. Also fetch or recompute each result's embedding vector
3. Iteratively build the result set:
   - At each step, pick the result from the remaining candidates that maximizes:
     ```
     MMR = λ · Sim(q, d) − (1−λ) · max(Sim(d, dⱼ)) for dⱼ already selected
     ```
   - `λ` controls the relevance–diversity tradeoff (0.5–0.7 is typical)
4. Return top-N after MMR reordering

**Implementation**: `internal/retriever/mmr.go`

```go
const StrategyMMR = "mmr-rerank"

func (r *Retriever) mmrSearch(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
    // 1. fetch topK * 3 candidates via naive-search
    // 2. fetch their vectors from store (or store them in payload)
    // 3. compute MMR scores
    // 4. return topK
}
```

**Store changes needed**: The current `Search` returns `SearchResult` (score + metadata) but no vector. Add a `SearchWithVectors` method or store vectors in the payload.

**Alternative (simpler)**: Pass `WithVectors: true` in the Qdrant `SearchPoints` call — the response already contains vectors. Store them in `SearchResult.Vector`.

```go
// internal/types/query.go
type SearchResult struct {
    ChunkID      string
    DocumentPath string
    Content      string
    Score        float32
    TokenCount   int
    ChunkIndex   int
    Vector       []float32  // <-- add this
}
```

**What you learn**: Why similarity search alone produces redundant results; how diversity metrics work; the tradeoff between precision and diversity (controlled by λ); the cost of fetching additional data (vectors) from the store.

**Expected impact**: NDCG@K improves because the ranked list covers more ground. Answer quality (via LLM) may improve because the generator sees more diverse context.

---

## Step 3: Query Expansion (Retriever Strategy)

**Concept**: A single query embedding captures only one facet of the user's intent. By generating multiple reformulations and merging their results, we cast a wider net and improve recall.

**How it works**:

1. Send the original query to the generator with a prompt like: _"Generate 5 different versions of this question that ask about the same topic but use different wording and angles"_
2. Embed the original query + all expansions
3. Search with each embedding independently
4. Merge all result sets using Reciprocal Rank Fusion (RRF):
   ```
   score(d) = Σ 1 / (k + rankᵢ(d))   for i in {original, expansionᵢ}
   ```
   where k=60 (standard constant)
5. Return the top-K by fused score

**Implementation**: `internal/retriever/query_expansion.go`

```go
const StrategyQueryExpansion = "query-expansion"

type QueryExpansionConfig struct {
    BaseStrategy   string  // wraps any strategy (e.g., naive-search, mmr-rerank)
    NumExpansions  int     // default 3
    MergeK         int     // RRF constant (default 60)
}

func (r *Retriever) queryExpansionSearch(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error)
```

**Prompt template** (keep it simple):

```
Given the question: "{query}"
Generate {N} different rephrasings that cover alternative phrasings and perspectives.
Return one per line, no numbering.
```

**What you learn**: How query quality affects retrieval; how RRF works as a merge strategy; the cost of N additional embedding API calls per query; how to design effective expansion prompts.

**Expected impact**: HitRate@K improves because multiple query facets have a higher chance of matching different relevant documents.

---

## Step 4: Hybrid Search (Dense + Sparse)

**Concept**: Dense embeddings capture semantic similarity but miss exact keyword matches (names, codes, abbreviations). Sparse retrieval (BM25-like) captures exact term matches but misses synonyms and paraphrases. Hybrid search combines both.

**How it works** — Qdrant supports sparse vectors natively:

1. `EnsureCollection` creates two vector configs: a dense vector (e.g., 1536d) and a sparse vector
2. During indexing, each chunk gets both a dense embedding (from the embedder) and a sparse vector. For the sparse vector, we can use Qdrant's built-in sparse vector support with a simple approach: tokenize the chunk text and use TF-IDF-like term frequencies directly as the sparse vector representation
3. During search, send both a dense vector and the query's sparse vector
4. Qdrant returns combined results (or you fuse them yourself with RRF)

**Implementation plan**:

**4a. Store layer changes** (`internal/store/store.go`, `qdrant.go`):

```go
// store.go
type VectorStore interface {
    // ... existing methods ...

    // New: multi-vector search
    SearchHybrid(ctx context.Context, collectionName string, denseVector []float32, queryText string, topK int) ([]types.SearchResult, error)

    // New: return vectors alongside results
    SearchWithVectors(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error)
}
```

```go
// qdrant.go — EnsureCollection adds sparse config:
// Existing: VectorsConfig with dense params
// Add: SparseVectorsConfig with "sparse" key

_, err = s.client.Collections().Create(ctx, &qdrant.CreateCollection{
    CollectionName: name,
    VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{...dense...}),
    SparseVectorsConfig: qdrant.NewSparseVectorsConfig(&qdrant.SparseVectorParams{
        Index: &qdrant.SparseIndexConfig{...},
    }),
})
```

```go
// Store — also needs to include sparse vectors in upsert
// Create a simple sparse vector from term frequency in the chunk
func chunkToSparseVector(text string) *qdrant.SparseVector {
    // tokenize, count, sort by index
}
```

**4b. Retriever strategy** (`internal/retriever/hybrid.go`):

```go
const StrategyHybrid = "hybrid-search"

func (r *Retriever) hybridSearch(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
    // 1. embed query (dense)
    // 2. build query sparse vector (same tokenizer as indexing)
    // 3. store.SearchHybrid(ctx, collection, denseVec, query, topK)
    // 4. OR run two searches + RRF-fuse yourself
}
```

**Simpler alternative** — Run two searches (dense + keyword via payload filter) and RRF-fuse in Go, avoiding Qdrant sparse vector API complexity. Use Qdrant's `Scroll` with `Filter` for keyword matching.

**What you learn**: Why dense alone misses exact matches; how sparse vectors work conceptually (term frequency per dimension); the Qdrant multi-vector API; the HyDE-like insight that different representations capture different relevance signals.

**Expected impact**: Precision@K improves for queries with named entities, codes, or domain-specific terms. Overall F1 (balance of precision and recall) improves across the board.

---

## Step 5: HyDE (Hypothetical Document Embeddings)

**Concept**: The query-to-document embedding gap can hurt retrieval — a question like "How do I file expenses?" is linguistically distant from a document titled "Travel and Expense Policy". HyDE bridges this gap by generating a hypothetical answer first, then searching with its embedding.

**How it works**:

1. Send query to generator: _"Write a paragraph that answers this question in the style of a company handbook"_
2. Embed the generated hypothetical document (not the query)
3. Search with that embedding
4. (Optional) filter/normalize the hypothetical doc if it hallucinates specifics

**Implementation**: `internal/retriever/hyde.go`

```go
const StrategyHyDE = "hyde"

func (r *Retriever) hydeSearch(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error) {
    // 1. generate hypothetical doc
    // 2. embed it
    // 3. search with the embedding
    // 4. return results
}
```

**Prompt**:

```
Given the question: "{query}"

Write a detailed paragraph that answers this question as if it were
excerpted from a company handbook. Be specific and factual-sounding.
Use the style and tone of internal documentation.
```

**What you learn**: The embedding gap problem (query vs. document distribution); how generative models can serve as retrieval augmentation; when HyDE helps vs. hurts (it works well for queries that map to a clear answer style, less for open-ended queries); the risk of hallucination propagating through the pipeline.

**Expected impact**: Recall@K improves for questions with clear expected answers. May hurt for ambiguous or multi-faceted questions where the hallucinated answer goes in the wrong direction.

---

## Step 6: LLM Reranker (Two-Stage Retrieval)

**Concept**: Embedding-based search is fast but imperfect. A second stage using an LLM to score/rerank the top candidates is slower but more accurate. This is the classic "retrieve then rerank" pattern.

**How it works**:

1. Retrieve top-20 (or top-30) candidates using any base strategy
2. Send each candidate + the query to the generator with an instruction to classify relevance (0–5 scale, or a yes/no "relevant" decision)
3. Rerank by LLM relevance score
4. Return top-5 (or top-K) from the reranked list

**Optimization**: Use a single prompt with all candidates and ask the LLM to rank them, reducing N API calls to 1.

**Batch rerank prompt**:

```
Given the question: "{query}"

Below are {N} document excerpts. Rank them by relevance to the question
from most relevant (1) to least relevant ({N}).

Output only the ranked list of excerpt IDs.

--- Excerpts ---
{id_1}: {content_1}
{id_2}: {content_2}
...
```

**Implementation**: `internal/retriever/llm_rerank.go`

```go
const StrategyLLMRerank = "llm-rerank"

type LLMRerankConfig struct {
    BaseStrategy    string  // strategy to get initial candidates
    InitialK        int     // e.g., 20
    FinalK          int     // e.g., 5
    BatchRerank     bool    // single prompt vs. per-doc scoring
}
```

**What you learn**: Two-stage retrieval tradeoffs (speed vs. accuracy); how LLM-based relevance scoring differs from embedding similarity; prompt engineering for ranking tasks; cost implications (N additional LLM calls per query).

**Expected impact**: Precision@K and MRR improve significantly. Latency increases by ~1 LLM call per query. This is the most accurate but most expensive strategy.

---

## Comparison Table

| Step | Strategy                 | Layer             | Lines | Eval Signal      | Cost per Query    | Difficulty |
| ---- | ------------------------ | ----------------- | ----- | ---------------- | ----------------- | ---------- |
| 0    | Baseline (fixed + naive) | —                 | —     | —                | 1 embed           | —          |
| 1    | Recursive chunker        | chunker           | ~120  | Recall@K         | 0 (indexing only) | Easy       |
| 2    | MMR reranker             | retriever         | ~80   | NDCG@K           | 0 (pure Go)       | Easy       |
| 3    | Query expansion          | retriever         | ~100  | HitRate@K        | N embeds          | Medium     |
| 4    | Hybrid search            | store + retriever | ~250  | Precision@K      | 2 searches        | Hard       |
| 5    | HyDE                     | retriever         | ~60   | Recall@K         | 1 gen + 1 embed   | Medium     |
| 6    | LLM reranker             | retriever         | ~100  | Precision@K, MRR | 1 gen             | Medium     |

## Running Eval After Each Step

```powershell
# 1. Index with new chunker
curl -X POST http://localhost:8080/api/v1/workflows/index -d "{""input_tag"":""handbook-v2"",""chunker"":""recursive"",""chunk_size"":512,""chunk_overlap"":64}"

# 2. Run eval with new retriever
curl -X POST http://localhost:8080/api/v1/workflows/eval -d "{""input_tag"":""handbook-v2"",""retriever"":""mmr-rerank"",""top_k"":10,""workers"":4}"

# 3. Compare metrics via API
curl http://localhost:8080/api/v1/eval/runs
```

You can also create a small eval runner script (`scripts/run-eval-comparison.ps1`) that iterates through all strategy combinations and prints a comparison table.

## Tracking Progress

Keep a running table in this file:

| Date | Chunker   | Retriever       | HR@3 | HR@5 | MRR | NDCG@5 | P@5 | R@5 | Notes    |
| ---- | --------- | --------------- | ---- | ---- | --- | ------ | --- | --- | -------- |
| —    | fixed     | naive-search    | —    | —    | —   | —      | —   | —   | baseline |
| —    | recursive | naive-search    | —    | —    | —   | —      | —   | —   | step 1   |
| —    | recursive | mmr-rerank      | —    | —    | —   | —      | —   | —   | step 2   |
| —    | recursive | query-expansion | —    | —    | —   | —      | —   | —   | step 3   |
| —    | recursive | hybrid-search   | —    | —    | —   | —      | —   | —   | step 4   |
| —    | recursive | hyde            | —    | —    | —   | —      | —   | —   | step 5   |
| —    | recursive | llm-rerank      | —    | —    | —   | —      | —   | —   | step 6   |
