# Scale Test Plan

## Goals

1. Determine the maximum throughput and document count each pipeline stage can handle before failing or degrading unacceptably.
2. Identify the specific bottleneck at each scale level (1x, 10x, 100x).
3. Establish pass/fail criteria for production-readiness.
4. Guide the order and priority of hardening fixes.

## Scale Levels

| Level    | Documents | Approx. size     | Notes                                       |
| -------- | --------- | ---------------- | ------------------------------------------- |
| **1x**   | ~2,000    | ~50 MB markdown  | Handbook corpus today — baseline            |
| **10x**  | ~20,000   | ~500 MB markdown | 10 handbook copies or synthetic files       |
| **100x** | ~200,000  | ~5 GB markdown   | Large-scale test, may require orchestration |

## Pass / Fail Criteria

A stage "passes" at a given scale if ALL of:

- **Zero data loss**: Every input document is represented in the output
- **Zero silent failures**: No errors swallowed without being logged
- **No OOM / panic**: Process completes without crash or `runtime.NNnn: out of memory`
- **Reasonable duration**: Completes within 2x of linearly projected time from 1x
- **Correct output**: Verification checks pass (file count, directory structure, content checksums)

A stage "fails" if ANY of:

- Process OOMs or panics
- Document count mismatch between input and output
- Embedding/generation rate-limit retries exceed configured max and data is lost
- Indexer abandons remaining files due to a single failure
- gRPC connection drops and is not recovered, causing permanent failure

## Metrics to Capture

### Per run (aggregate)

| Metric                        | Source                          | Notes |
| ----------------------------- | ------------------------------- | ----- |
| Total wall-clock time         | `time` command                  |       |
| Peak RSS memory               | `Get-Process` or Docker stats   |       |
| Total documents processed     | Pipeline output                 |       |
| Total chunks created          | Pipeline output                 |       |
| Documents failed / skipped    | Pipeline logs                   |       |
| API call count (embed)        | Embedder logs                   |       |
| API call count (generate)     | Generator logs                  |       |
| Rate-limit 429 count          | Embedder + generator error logs |       |
| Rate-limit backoff total time | Derived from log timestamps     |       |
| gRPC connection errors        | Qdrant store logs               |       |

### Per-stage breakdown

| Stage           | Metric                          | How to measure      |
| --------------- | ------------------------------- | ------------------- |
| Clone           | Clone/pull time                 | Stage duration      |
| Preprocess      | Files processed / sec           | Count / time        |
| Preprocess      | Memory per file walk            | Monitoring          |
| Verify          | Walk duration + report size     | Stage duration      |
| Parse           | Parse time per file             | Add per-file timing |
| Chunk           | Chunks per file, total tokens   | Chunker output      |
| Embed           | Batches / 429s / batch latency  | Embedder logs       |
| Store           | Upsert batches / batch duration | Qdrant store logs   |
| Eval (search)   | Search latency per query        | Eval pipeline       |
| Eval (generate) | Generate latency per query      | Generator logs      |
| Eval (judge)    | Judge latency per query         | Judge logs          |

## Test Data Generation

### Preprocessing scale tests

Synthetic data avoids network dependency on GitLab.

```powershell
# scripts/gen-scale-test-data.ps1
param(
    [int]$DocumentCount = 2000,
    [string]$OutputDir = "testdata/scale"
)

$template = @"
# Section {0}

This is a synthetic markdown document for scale testing.

{{% include "includes/common.md" %}}

## Content Block {1}

{2}

{{< ref "related-doc-{3}" >}}

<div class="details">
<summary>Details</summary>
Content inside HTML.
</div>
"@

$lorem = @"
Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.
"@ * 10

New-Item -ItemType Directory -Path "$OutputDir/content" -Force | Out-Null
New-Item -ItemType Directory -Path "$OutputDir/includes" -Force | Out-Null

# Create shared include
Set-Content -Path "$OutputDir/includes/common.md" -Value "Shared include content for scale testing."

for ($i = 0; $i -lt $DocumentCount; $i++) {
    $content = $template -f $i, ($i % 10), $lorem, (($i + 1) % $DocumentCount)
    Set-Content -Path "$OutputDir/content/doc-$i.md" -Value $content
}

Write-Host "Generated $DocumentCount documents in $OutputDir"
```

### Indexing scale tests

Preprocess the synthetic data to produce the output directory, then point the indexer at it.

### Eval scale tests

Generate synthetic eval datasets:

```powershell
# scripts/gen-scale-eval-data.ps1
param(
    [int]$QuestionCount = 100,
    [string]$OutputPath = "testdata/eval/scale-100.json"
)

$questions = @()
for ($i = 0; $i -lt $QuestionCount; $i++) {
    $questions += @{
        id = "scale-q-$i"
        question = "Sample question $i about the handbook?"
        expected_answer = "Expected answer $i."
        category = "scale-test"
        difficulty = "easy"
        relevance = @(
            @{
                document_path = "content/doc-$i.md"
                grade = 2
            }
        )
    }
}

$data = @{ questions = $questions }
$data | ConvertTo-Json -Depth 4 | Set-Content -Path $OutputPath
Write-Host "Generated $QuestionCount eval questions in $OutputPath"
```

## Test Scenarios

### Scenario A: Preprocessing Throughput

**What it tests**: Raw throughput of `preprocess` stage (clone → preprocess → verify) without real GitLab dependency.

**Procedure**:

1. Generate synthetic data at 1x scale (2k files, ~50 MB)
2. Copy data to `artifacts/preprocessing/scale-test-1x/repo/content/`
3. Run: `go run ./cmd/preprocess --tag scale-test-1x --repo-url file:///dev/null --include-dirs content/`
4. Record all metrics
5. Repeat for 10x, 100x
6. If 100x fails, binary-search to find breaking point

**Expected outcomes**:

- 1x: Complete in < 5 min, < 500 MB RSS
- 10x: Complete in < 30 min, < 1 GB RSS
- 100x: Likely OOM or excessive duration; threshold to document

### Scenario B: Indexing Throughput

**What it tests**: Parse → chunk → embed → store path under load.

**Precondition**: Preprocessed output from Scenario A at each scale level.

**Procedure**:

1. Use preprocessed output from Scenario A
2. Run: `go run ./cmd/index --input-tag scale-test-Nx --tag idx-scale-test-Nx --embedding-model text-embedding-3-small --batch-size 20`
3. Record all metrics
4. Run at each scale; note the point where rate-limit retries cause 50%+ overhead

**Expected outcomes**:

- 1x: Complete embedding within API rate limits
- 10x: Rate-limit retries add < 50% overhead
- 100x: Rate-limit retries dominate > 80% of wall time

### Scenario C: Failure Isolation

**What it tests**: Graceful degradation when individual files or operations fail.

**Procedure**:

1. Inject corrupt files into a 1x dataset (binary content, missing includes, malformed markdown)
2. Inject a file whose embedding consistently 400s (e.g., absurdly long content)
3. Verify that errors are logged, skipped files are counted, remaining files are processed
4. Verify that a single failing file does NOT abort the entire pipeline

**Expected outcomes**:

- Parse/chunk errors: skipped with warning, pipeline continues
- Embed errors: skipped with warning, pipeline continues (currently FAILS — fix needed)
- Store errors: retried (not currently implemented)

### Scenario D: Connection Resilience

**What it tests**: Pipeline behavior when backing services (Qdrant, Postgres) are interrupted.

**Procedure**:

1. Start a 1x indexing run
2. Kill the Qdrant container mid-run
3. Wait 10 seconds, restart Qdrant
4. Observe whether the pipeline recovers or permanently fails

**Expected outcomes**:

- gRPC disconnects should be detected and retried with backoff
- Current behavior: permanent failure (fix needed)

### Scenario E: Resource Limits

**What it tests**: Memory ceiling, goroutine leaks, file descriptor leaks.

**Procedure**:

1. Run Scenario B at 10x scale with `$env:GOMEMLIMIT = "1GiB"`
2. Run with `-race` detector (`go run -race ./cmd/index ...`)
3. Monitor open file handles with `Get-Process`
4. Verify no FD leaks after large walks

**Expected outcomes**:

- No race conditions
- No goroutine leak (steady-state goroutine count)
- File handles released after file walks

### Scenario F: Eval Throughput

**What it tests**: End-to-end eval pipeline with varying question counts and LLM providers.

**Precondition**: An indexed collection at 1x scale (`idx-eval-scale-1x`).

**Procedure**:

1. Generate eval datasets at 10, 100, and 1000 questions
2. Run for each:
   ```
   go run ./cmd/eval --index-tag idx-eval-scale-1x --query-strategy naive-search --dataset-dir testdata/eval/ --llm-provider openai
   ```
3. Measure: time per question, 429 counts, judge reliability

**Expected outcomes**:

- 10 questions: < 1 min
- 100 questions: < 10 min
- 1000 questions: will take hours due to sequential design (document the ceiling)

### Scenario G: Workerd Concurrency Ceiling

**What it tests**: Max parallel throughput of River workers with increasing `MaxWorkers`.

**Procedure**:

1. Submit 50 preprocess workflows at once
2. Run workerd with `MaxWorkers: 1`, `MaxWorkers: 5`, `MaxWorkers: 20`
3. Measure time to drain all workflows

## Infrastructure

### Docker resource limits

When testing at 10x and 100x, constrain Docker containers to prevent host OOM:

```yaml
services:
  workerd:
    deploy:
      resources:
        limits:
          memory: 4G
```

Run with:

```powershell
docker compose run -e GOMEMLIMIT=3GiB workerd
```

### Monitoring commands

```powershell
# Memory during run — run in separate terminal
$process = Get-Process -Name workerd; while ($true) { $mem = [math]::Round($process.WorkingSet64 / 1MB); Write-Host "$(Get-Date -Format 'HH:mm:ss') MEM=${mem}MB"; Start-Sleep -Seconds 2 }

# Count Qdrant points during index
docker compose exec qdrant curl -s http://localhost:6333/collections/idx-scale-test-1x | ConvertFrom-Json | Select-Object -ExpandProperty result | Select-Object -ExpandProperty points_count
```

## Test Execution Matrix

Run each scenario at each scale level and record results.

| Scenario                 | 1x (2k)     | 10x (20k)      | 100x (200k)    | Breaking point |
| ------------------------ | ----------- | -------------- | -------------- | -------------- |
| A: Preprocessing         | ✅ Baseline | 🔴 Find issues | 🔴 Find OOM    | Document       |
| B: Indexing              | ✅ Baseline | 🔴 Find issues | 🔴 Find issues | Document       |
| C: Failure isolation     | ✅ Baseline | N/A            | N/A            | N/A            |
| D: Connection resilience | ✅ Baseline | N/A            | N/A            | N/A            |
| E: Resource limits       | ✅ Baseline | ✅ Baseline    | 🔴 Find issues | Document       |
| F: Eval throughput       | ✅ Baseline | ✅ Baseline    | 🔴 Document    | Document       |

✅ = Should pass with current code
🔴 = Expected to reveal issues

## Priority Issues to Fix Before/During Scale Tests

Fix these before running the test matrix (they distort results if not fixed):

1. **Indexer single-failure abandons all** — wrap embed/store in graceful skip
2. **Embedder per-file timeout** — make configurable or scale with input size
3. **Workerd `MaxWorkers: 5`** — increase default or make configurable
4. **Add `GOMEMLIMIT`** — set in Dockerfile or workerd startup

Measure during tests and fix afterwards:

5. Proactive rate limiting for embedder/generator
6. Qdrant gRPC reconnection
7. Composite DB indexes
8. Streaming file processing (post-MVP)
