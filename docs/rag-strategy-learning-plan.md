# RAG Strategy Learning Plan

A focused, ROI-driven guide to improving RAG quality through chunking and retrieval strategies. The goal is not to implement everything — it's to learn the concepts that give the biggest measurable improvement with the least code.

---

## Philosophy

Each strategy should earn its place by answering two questions:

1. **Does it fix a measurable weakness?** — Every strategy targets a specific signal (Recall@K, Precision@K, NDCG@K, MRR). If a strategy doesn't move its target, drop it.
2. **Is the overhead worth the gain?** — Lines of code, API cost per query, indexing complexity, and maintenance burden must be proportional to the improvement.

```
Implement → eval → compare → keep or discard
```

The eval pipeline produces HitRate@K, MRR, NDCG@K, Precision@K, Recall@K, and answer quality scores — so every change is measurable.

---

## Current State (Already Implemented)

| Layer | Strategy | Status |
| ----- | -------- | ------ |
| Chunker | `fixed` (word-window, size=512, overlap=64) | Done — baseline |
| Chunker | `recursive` (structure-aware, heading hierarchy) | Done — step 1 |
| Retriever | `naive-search` (embed query → dense NN) | Done — baseline |

---

## Strategy Candidates (In Suggested Order)

### Step 2: MMR Reranker

**Why**: Top-K results from similarity search are often redundant — the top 5 might all be variations of the same paragraph. MMR (Maximal Marginal Relevance) diversifies the result set. Starting here teaches the diversity concept with zero API cost before adding more complex strategies.

**How it works**:
1. Run `naive-search` to get top-K×3 candidates (with vectors)
2. Fetch each candidate's embedding vector (Qdrant already returns these with `WithVectors: true`)
3. Greedily build result set maximizing:
   ```
   MMR = λ · Sim(q, d) − (1−λ) · max(Sim(d, dⱼ)) for dⱼ already selected
   ```
4. λ=0.5–0.7 controls relevance vs. diversity

**Store changes needed**: `Search` needs to return vectors. Add `WithVectors: true` in Qdrant's `SearchPoints` call and populate a `Vector` field on `SearchResult`.

**What you learn**: Why similarity search alone produces redundant results; diversity-aware ranking; greedy optimization; the relevance–diversity tradeoff (λ).

**Expected impact**: NDCG@K improves because the ranked list covers more ground. Answer quality may improve because the generator sees more diverse context.

| Layer | Lines | Cost per Query | Difficulty |
| ----- | ----- | -------------- | ---------- |
| retriever | ~80 | 0 (pure Go) | Easy |

---

### Step 3: Hybrid Search (Dense + Keyword)

**Why**: Dense embeddings miss exact keyword matches. For a technical handbook full of codes, abbreviations, names, and version numbers, "CI/CD pipeline" won't match "continuous integration" in dense space alone. Hybrid search combines semantic similarity with term-level precision.

**Approach** — Qdrant supports sparse vectors natively. Two implementation options:

**Option A (Qdrant sparse API)**:
- `EnsureCollection` creates two vector configs: dense (1536d) + sparse
- Indexing stores both a dense embedding and a TF-IDF sparse vector per chunk
- Search sends both vectors; Qdrant fuses results internally

**Option B (two-pass + RRF, simpler)**:
- Run dense search + keyword search (payload filter via `Scroll`) independently
- Fuse with Reciprocal Rank Fusion: `score(d) = Σ 1 / (k + rankᵢ(d))`

**What you learn**: Dense vs. sparse representations; tokenization for sparse vectors; Qdrant's multi-vector API; RRF fusion; why exact-match matters alongside semantics.

**Expected impact**: Precision@K improves for named entities and domain terms. Overall F1 improves across the board.

| Layer | Lines | Cost per Query | Difficulty |
| ----- | ----- | -------------- | ---------- |
| store + retriever | ~250 | 2 searches | Hard |

---

### Step 4: LLM Reranker (Two-Stage Retrieval)

**Why**: Embedding similarity is fast but imperfect. A second-stage LLM relevance judgment is the single highest-leverage improvement for precision — production RAG systems almost universally use reranking.

**How it works**:
1. Retrieve top-20 candidates via any base strategy (`naive-search`, `mmr-rerank`, or `hybrid-search`)
2. Send all candidates + query in a single prompt: _"Rank these excerpts by relevance to the question"_
3. Parse the ranked list, return top-K

**Batch rerank prompt**:
```
Question: "{query}"

Below are {N} excerpts. Rank them from most relevant (1) to least relevant ({N}).
Output only the ranked list of excerpt IDs.

--- Excerpts ---
{id_1}: {content_1}
{id_2}: {content_2}
...
```

**What you learn**: Two-stage retrieval tradeoffs (speed vs. accuracy); LLM-based relevance vs. embedding similarity; prompt engineering for structured ranking output; cost analysis (1 gen call per query).

**Expected impact**: Precision@K and MRR improve significantly. This is the most accurate retriever strategy, at the cost of ~1 LLM call per query.

| Layer | Lines | Cost per Query | Difficulty |
| ----- | ----- | -------------- | ---------- |
| retriever | ~100 | 1 gen call | Medium |

---

## Not Pursuing (Why)

| Strategy | Why Not |
| -------- | ------- |
| **Query Expansion** | Overlaps with hybrid search for recall improvement. The embed API cost (N calls per query) is small, but RRF is already learned in Step 2. Marginal learning is low once you understand fusion. |
| **HyDE** | Inconsistent results — helps for some query types, hurts for others. Adds LLM latency and hallucination risk. The embedding gap problem is better addressed by chunking (recursive) and query understanding (hybrid search). |

If eval reveals a specific gap these would fill, reconsider. Otherwise, time is better spent on tuning and productionizing the higher-ROI strategies.

---

## Comparison Table

| Step | Strategy | Layer | Lines | Target Signal | Cost per Query | Difficulty |
| ---- | -------- | ----- | ----- | ------------- | -------------- | ---------- |
| 0 | Baseline (fixed + naive) | — | — | — | 1 embed | — |
| 1 | Recursive chunker | chunker | ~120 | Recall@K | 0 (indexing) | Easy |
| 2 | MMR reranker | retriever | ~80 | NDCG@K | 0 (pure Go) | Easy |
| 3 | Hybrid search | store + retriever | ~250 | Precision@K | 2 searches | Hard |
| 4 | LLM reranker | retriever | ~100 | Precision@K, MRR | 1 gen call | Medium |

---

## Running Eval

```powershell
# Index with a chunker
curl -X POST http://localhost:8080/api/v1/workflows/index -d "{""input_tag"":""handbook-v2"",""chunker"":""recursive"",""chunk_size"":512,""chunk_overlap"":64}"

# Run eval with a retriever
curl -X POST http://localhost:8080/api/v1/workflows/eval -d "{""input_tag"":""handbook-v2"",""retriever"":""hybrid-search"",""top_k"":10,""workers"":4}"

# Compare runs
curl http://localhost:8080/api/v1/eval/runs
```

---

## Tracking Progress

| Date | Chunker | Retriever | HR@3 | HR@5 | MRR | NDCG@5 | P@5 | R@5 | Notes |
| ---- | ------- | --------- | ---- | ---- | --- | ------ | --- | --- | ----- |
| — | fixed | naive-search | — | — | — | — | — | — | baseline |
| — | recursive | naive-search | — | — | — | — | — | — | step 1 |
| — | recursive | mmr-rerank | — | — | — | — | — | — | step 2 |
| — | recursive | hybrid-search | — | — | — | — | — | — | step 3 |
| — | recursive | llm-rerank | — | — | — | — | — | — | step 4 |
