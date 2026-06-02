# RAG Evaluation System — Design Plan

## Why Evaluate?

The GitLab handbook RAG pipeline has ~4,500 markdown files, 3 chunking strategies, 2 embedding backends, and 2 vector stores. Without a systematic evaluation framework, we cannot:

- Compare chunking strategies objectively (fixed vs. semantic vs. recursive)
- Tune parameters (chunk size, overlap, batch size)
- Choose between embedding models (OpenAI vs. local)
- Detect regressions when code changes
- Measure real-world retrieval quality for handbook Q&A

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                    Evaluation System                         │
│                                                              │
│  ┌───────────────────┐    ┌──────────────────────────────┐   │
│  │  Ground Truth     │    │     Evaluation Harness       │   │
│  │  Dataset          │───>│                              │   │
│  │  (testdata/eval/) │    │  ┌────────────────────────┐  │   │
│  └───────────────────┘    │  │  Indexing Evaluator    │  │   │
│                           │  │  - chunk quality       │  │   │
│  ┌───────────────────┐    │  │  - embedding fidelity  │  │   │
│  │  Query Runner     │───>│  └────────────────────────┘  │   │
│  │  (retrieve for    │    │  ┌────────────────────────┐  │   │
│  │   each question)  │    │  │  Retrieval Evaluator   │  │   │
│  └───────────────────┘    │  │  - HitRate, MRR, NDCG  │  │   │
│                           │  │  - Precision, Recall   │  │   │
│  ┌───────────────────┐    │  └────────────────────────┘  │   │
│  │  Config Matrix    │───>│  ┌────────────────────────┐  │   │
│  │  (all strategy    │    │  │  End-to-End Eval       │  │   │
│  │   combos)         │    │  │  - faithfulness (opt)  │  │   │
│  └───────────────────┘    │  │  - hallucination (opt) │  │   │
│                           │  └────────────────────────┘  │   │
│  ┌───────────────────┐    │  ┌────────────────────────┐  │   │
│  │  Journal (prev    │<───│  │  Performance Eval      │  │   │
│  │  run for diff)    │    │  │  - latency, throughput │  │   │
│  └───────────────────┘    │  └────────────────────────┘  │   │
│                           └──────────────────────────────┘   │
│                                       │                      │
│                                       ▼                      │
│                          ┌──────────────────────────────┐    │
│                          │     Output Report            │    │
│                          │  - JSON report (machine)     │    │
│                          │  - Summary table (terminal)  │    │
│                          │  - Diff from previous run    │    │
│                          └──────────────────────────────┘    │
└──────────────────────────────────────────────────────────────┘
```

---

## Ground Truth Dataset Format

Location: `testdata/eval/questions.json`

```json
{
  "meta": {
    "created": "2026-06-02",
    "version": 1,
    "description": "GitLab handbook RAG evaluation questions"
  },
  "questions": [
    {
      "id": "q001",
      "category": "travel",
      "difficulty": "easy",
      "question": "What is GitLab's policy on economy class flights for business travel?",
      "answer": "GitLab requires all business travel flights to be booked in economy class...",
      "source_paths": ["handbook/travel-policy.md"],
      "keywords": ["economy", "flight", "travel", "booking"],
      "expected_chunk_topics": ["Flight Booking Policy", "Travel Classes"]
    }
  ]
}
```

**Design principles:**

- Each question maps to 1+ handbook source files (ground truth relevant docs)
- `source_paths` are relative paths matching those in `output/`
- `difficulty` tiers: easy (single page), medium (cross-page), hard (inference required)
- Target: **100–150 questions** minimum for statistical significance
- Categories should cover all major handbook sections

---

## Evaluation Metrics — Detailed Specification

### 1. Indexing Quality Metrics

| Metric                      | Formula                                       | Threshold | What it catches                   |
| --------------------------- | --------------------------------------------- | --------- | --------------------------------- |
| `chunk_size_violation_rate` | chunks outside [0.5×target, 2×target] / total | < 10%     | Bad chunk boundaries              |
| `heading_alignment_rate`    | chunks starting with heading / total          | > 80%     | Semantic chunker losing structure |
| `content_loss_rate`         | sum(len(output_chunks)) / sum(len(documents)) | > 95%     | Content dropped during chunking   |
| `embedding_dimension_match` | all embeddings match declared dim             | == 100%   | Model mismatch errors             |
| `duplicate_chunk_rate`      | exact duplicate content chunks / total        | < 1%      | Overlap creating redundant chunks |

### 2. Retrieval Metrics

| Metric        | Definition                                         | Range  | Interpretation               |
| ------------- | -------------------------------------------------- | ------ | ---------------------------- |
| `HitRate@K`   | Fraction of queries where ≥1 relevant doc in top-K | [0, 1] | Higher = better coverage     |
| `MRR`         | Mean of 1/rank_first_relevant across queries       | [0, 1] | Higher = better ranking      |
| `NDCG@K`      | Normalized Discounted Cumulative Gain at K         | [0, 1] | Higher = better ordering     |
| `Precision@K` | Relevant in top-K / K                              | [0, 1] | Higher = less noise          |
| `Recall@K`    | Relevant in top-K / total relevant                 | [0, 1] | Higher = more coverage       |
| `MAP@K`       | Mean Average Precision at K                        | [0, 1] | Combines precision + ranking |

**Default K values:** 1, 3, 5, 10, 20

### 3. End-to-End Metrics (Optional — requires LLM judge)

| Metric                | Method                                          | What it measures   |
| --------------------- | ----------------------------------------------- | ------------------ |
| `answer_faithfulness` | LLM-as-judge: does answer contradict context?   | Hallucination rate |
| `answer_relevance`    | LLM-as-judge: does answer address the question? | Helpfulness        |
| `context_sufficiency` | Is retrieved context enough to answer?          | Retrieval quality  |

**LLM-as-judge prompt pattern:**

```
Given the question and the retrieved context, evaluate:
1. Faithfulness (1-5): Does the answer stay strictly within the context?
2. Relevance (1-5): Does the answer directly address the question?
3. Completeness (1-5): Does the answer cover all aspects of the question?
```

### 4. Performance Metrics

| Metric                  | Unit         | Target                 |
| ----------------------- | ------------ | ---------------------- |
| `index_build_time`      | seconds      | < 300s for 4,500 files |
| `p50_chunk_latency`     | ms per doc   | < 50ms                 |
| `p99_chunk_latency`     | ms per doc   | < 200ms                |
| `p50_embed_latency`     | ms per chunk | < 100ms (API)          |
| `p99_embed_latency`     | ms per chunk | < 500ms (API)          |
| `p50_retrieval_latency` | ms per query | < 200ms                |
| `p99_retrieval_latency` | ms per query | < 1000ms               |
| `vector_store_size`     | MB           | < 500MB for 4,500 docs |
| `memory_peak`           | MB           | < 1024MB               |

---

## Parameter Sweep Matrix

The evaluation should test combinations:

| Parameter         | Values to Test                                            |
| ----------------- | --------------------------------------------------------- |
| `chunk_strategy`  | fixed, semantic, recursive                                |
| `chunk_size`      | 256, 512, 1024                                            |
| `chunk_overlap`   | 0, 64, 128                                                |
| `embedding_model` | openai (text-embedding-3-small), local (all-MiniLM-L6-v2) |
| `retriever_top_k` | 3, 5, 10                                                  |

That's 3 × 3 × 3 × 2 × 3 = **162 combinations** (54 per embedding model). Strategy: run full sweep nightly; run only best-known config during CI.

---

## Output Report Format

### Terminal Summary (human-readable)

```
╔══════════════════════════════════════════════════════════╗
║            RAG Evaluation Report — 2026-06-02            ║
╠══════════════════════════════════════════════════════════╣
║ Configuration:                                           ║
║   chunk_strategy: semantic  chunk_size: 512              ║
║   embedding: openai (1536d)  store: qdrant               ║
╠══════════════════════════════════════════════════════════╣
║ Indexing Quality                   Score    Threshold    ║
║   chunk_size_violation_rate        3.2%     < 10% (pass) ║
║   heading_alignment_rate          92.1%     > 80% (pass) ║
║   content_loss_rate               97.4%     > 95% (pass) ║
╠══════════════════════════════════════════════════════════╣
║ Retrieval (K=5)                    Score    vs Prev      ║
║   HitRate@5                       0.872    +0.023  ▲     ║
║   MRR                             0.814    -0.007  ▼     ║
║   NDCG@5                          0.791    +0.015  ▲     ║
║   Precision@5                     0.634    +0.042  ▲     ║
║   Recall@5                        0.723    +0.011  ▲     ║
╠══════════════════════════════════════════════════════════╣
║ Performance                          p50      p99        ║
║   chunk_latency                     12ms     45ms        ║
║   embed_latency                     67ms    312ms        ║
║   retrieval_latency                 45ms    189ms        ║
╠══════════════════════════════════════════════════════════╣
║ 100 questions evaluated.  3 thresholds failing.          ║
╚══════════════════════════════════════════════════════════╝
```

### JSON Report (machine-readable)

```json
{
  "run_id": "eval-20260602-153042",
  "config": { "chunk_strategy": "semantic", "chunk_size": 512, ... },
  "indexing_quality": { "chunk_size_violation_rate": 0.032, ... },
  "retrieval": {
    "aggregate": { "hit_rate@5": 0.872, "mrr": 0.814, ... },
    "per_question": [
      { "id": "q001", "hit@5": true, "rank_first": 1, "ndcg@5": 1.0 }
    ]
  },
  "performance": { "chunk_latency": { "p50": 12, "p99": 45 }, ... },
  "threshold_results": [
    { "metric": "hit_rate@5", "threshold": 0.80, "actual": 0.872, "passed": true }
  ],
  "regressions": [
    { "metric": "mrr", "previous": 0.821, "current": 0.814, "delta": -0.007, "regression": true }
  ]
}
```

---

## Project Structure — Additions for Evaluation

```
root/
├── cmd/
│   └── eval/                           # NEW: Evaluation CLI
│       └── main.go
│
├── internal/
│   ├── eval/                           # NEW: Evaluation package
│   │   ├── eval.go                     # Orchestrator: run all evaluators
│   │   ├── metrics.go                  # Core metric calculations (HitRate, MRR, NDCG)
│   │   ├── indexing.go                 # Indexing quality checks
│   │   ├── retrieval.go                # Retrieval evaluator
│   │   ├── performance.go              # Latency/throughput tracker
│   │   ├── report.go                   # Report generation (terminal + JSON)
│   │   ├── threshold.go                # Pass/fail threshold checking
│   │   └── diff.go                     # Regression comparison with previous run
│   │
│   ├── config/                         # Extended
│   │   └── config.go                   # NEW: eval fields (thresholds, dataset path, etc.)
│   │
│   ├── types/                          # Extended
│   │   └── eval.go                     # NEW: EvalQuestion, EvalRun, EvalReport, etc.
│   │
│   └── journal/                        # Reused
│       └── gob.go                      # .journal-eval/ for storing previous results
│
├── testdata/
│   └── eval/                           # NEW: evaluation data
│       ├── questions.json              # Ground truth Q&A
│       └── README.md                   # Guidelines for contributors
│
└── make.cmd                            # Extended: eval commands
```

---

## Domain Types

```go
// internal/types/eval.go

// EvalQuestion represents a single ground-truth question.
type EvalQuestion struct {
    ID                 string   `json:"id"`
    Category           string   `json:"category"`
    Difficulty         string   `json:"difficulty"` // easy, medium, hard
    Question           string   `json:"question"`
    Answer             string   `json:"answer"`
    SourcePaths        []string `json:"source_paths"`
    Keywords           []string `json:"keywords,omitempty"`
    ExpectedChunkTopics []string `json:"expected_chunk_topics,omitempty"`
}

// EvalConfig controls which evaluations to run.
type EvalConfig struct {
    DatasetPath       string  // path to questions.json
    TopK              []int   // [1, 3, 5, 10, 20]
    RunIndexingChecks bool    `json:"run_indexing_checks"`
    RunRetrievalChecks bool   `json:"run_retrieval_checks"`
    RunPerformanceChecks bool `json:"run_performance_checks"`
    Thresholds         map[string]float64 // metric -> min score
    ComparePrevious   bool    // diff against last journal entry
}

// RetrievalResult captures the outcome for a single question.
type RetrievalResult struct {
    QuestionID     string
    RetrievedPaths []string           // paths returned by retriever
    RelevantPaths  []string           // ground truth paths
    Scores         []float64          // similarity scores
    Hit            map[int]bool       // hit@K for each K
    RankFirst      int                // 1-based rank of first relevant
}

// EvalReport is the complete evaluation output.
type EvalReport struct {
    RunID         string
    Config        EvalConfig
    IndexingQuality map[string]float64
    Retrieval     struct {
        Aggregate map[string]float64        // "hit_rate@5": 0.872
        PerQuestion []RetrievalResult
    }
    Performance   map[string]map[string]float64 // metric -> {p50, p95, p99}
    Thresholds    []ThresholdResult
    Regressions   []RegressionResult
}

type ThresholdResult struct {
    Metric    string
    Threshold float64
    Actual    float64
    Passed    bool
}

type RegressionResult struct {
    Metric   string
    Previous float64
    Current  float64
    Delta    float64
    Severity string // none, minor, major, critical
}
```

---

## Metric Calculation Details

### HitRate@K

```go
func HitRate(results []RetrievalResult, k int) float64 {
    hits := 0
    for _, r := range results {
        for i := 0; i < min(k, len(r.RetrievedPaths)); i++ {
            if contains(r.RelevantPaths, r.RetrievedPaths[i]) {
                hits++
                break
            }
        }
    }
    return float64(hits) / float64(len(results))
}
```

### MRR

```go
func MRR(results []RetrievalResult) float64 {
    sum := 0.0
    for _, r := range results {
        if r.RankFirst > 0 {
            sum += 1.0 / float64(r.RankFirst)
        }
    }
    return sum / float64(len(results))
}
```

### NDCG@K

```go
func DCG(relevances []float64, k int) float64 {
    dcg := 0.0
    for i := 0; i < min(k, len(relevances)); i++ {
        num := math.Pow(2, relevances[i]) - 1
        den := math.Log2(float64(i + 2))
        dcg += num / den
    }
    return dcg
}

func NDCG(results []RetrievalResult, k int) float64 {
    // DCG / IdealDCG per query, averaged
}
```

---

## Threshold Pass/Fail System

Configured in config or via flags:

```json
{
  "thresholds": {
    "hit_rate@5": 0.8,
    "mrr": 0.7,
    "ndcg@5": 0.75,
    "precision@5": 0.5,
    "chunk_size_violation_rate": 0.1,
    "content_loss_rate": 0.95,
    "p99_retrieval_latency_ms": 1000
  }
}
```

`cmd/eval` exits non-zero if any threshold fails — CI pipeline blocks on regression.

---

## Comparison / Regression Detection

Store previous evaluation run in `.journal-eval/` (reuse GobFileJournal):

```go
type EvalJournal struct {
    PreviousReport *EvalReport
}
```

On each run:

1. Load previous report from journal
2. Run current evaluation
3. Compare each metric: delta = current - previous
4. Classify deltas: minor (<0.02), major (0.02–0.05), critical (>0.05)
5. Save current as new previous

---

## Parameter Sweep Runner

```go
type SweepConfig struct {
    ChunkStrategies []string
    ChunkSizes      []int
    ChunkOverlaps   []int
    EmbeddingModels []string
    TopKs           []int
}

func RunSweep(ctx context.Context, baseCfg *config.Config, sweep SweepConfig) ([]EvalReport, error) {
    results := []EvalReport{}
    for _, strategy := range sweep.ChunkStrategies {
        for _, size := range sweep.ChunkSizes {
            for _, overlap := range sweep.ChunkOverlaps {
                // Build index with these params
                // Run evaluation
                // Append report
            }
        }
    }
    return results, nil
}
```

**Output:** `eval-report-sweep.json` with all reports + a leaderboard ranking configurations by aggregate score.

---

## Implementation Phases

| Phase   | What                                                              | Depends On                  | Est. Effort |
| ------- | ----------------------------------------------------------------- | --------------------------- | ----------- |
| **P1**  | Types + Config (EvalQuestion, EvalReport, EvalConfig thresholds)  | —                           | Small       |
| **P2**  | Core metrics (metrics.go — HitRate, MRR, NDCG, Precision, Recall) | P1                          | Small       |
| **P3**  | Ground truth dataset (50+ questions)                              | —                           | Medium      |
| **P4**  | Retrieval evaluator (run retriever × questions, collect results)  | P1 + P2 + indexing pipeline | Large       |
| **P5**  | Report generation (terminal table + JSON output)                  | P2                          | Small       |
| **P6**  | Threshold system + regression detection                           | P2 + P5                     | Small       |
| **P7**  | Indexing quality checks                                           | P1                          | Small       |
| **P8**  | Performance tracking                                              | P1                          | Small       |
| **P9**  | `cmd/eval/main.go` — wire as CLI                                  | P4–P8                       | Small       |
| **P10** | Parameter sweep runner                                            | P9                          | Medium      |
| **P11** | LLM-as-judge E2E eval (optional)                                  | P4 + external LLM           | Medium      |
| **P12** | CI integration (make.cmd eval)                                    | P9                          | Tiny        |

**Dependency chain for the minimal viable eval (Phase 1–6):**

```
Indexing Pipeline (prerequisite) ──> Ground Truth Dataset
                                      │
Types + Config ──> Core Metrics ──> Retrieval Evaluator ──> Report Gen ──> CLI
                ──> Threshold System
```

---

## Key Design Decisions

| Decision              | Choice                           | Rationale                                                              |
| --------------------- | -------------------------------- | ---------------------------------------------------------------------- |
| Ground truth format   | Source paths (not chunk IDs)     | Chunk IDs change with chunking params; file paths are stable           |
| Relevance scoring     | Binary (relevant/not)            | Simplifies initial implementation; graded relevance can be added later |
| Metric implementation | Native Go (no external deps)     | Aligns with project's zero-dependency philosophy                       |
| Report format         | Terminal + JSON                  | Human + machine readable                                               |
| Regression detection  | Delta-based with severity levels | More actionable than simple pass/fail                                  |
| LLM-as-judge          | Optional, pluggable              | Core eval should work offline; E2E eval needs API key                  |
| Sweep vs single       | Both supported                   | Single for CI (fast), sweep for offline tuning                         |

---

## Future Extensions

- **A/B test framework**: compare two configs head-to-head with statistical significance
- **Synthetic question generation**: use LLM + handbook content to auto-generate eval questions
- **Failure analysis dashboard**: per-question drill-down showing which retrievals failed and why
- **Continuous eval**: GitHub Actions that runs eval on every PR and comments with diff
- **User feedback integration**: log real user queries + relevance feedback into eval dataset
