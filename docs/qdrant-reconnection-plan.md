# Qdrant gRPC Reconnection — Implementation Plan

3 phases. Addresses issue #4 from `known-issues-and-fixes.md`.

## Background

`QdrantStore` creates a single persistent gRPC client in `Connect()`. If Qdrant restarts or the network drops, all subsequent `Store`, `Search`, and `EnsureCollection` calls fail permanently. The gRPC library may or may not auto-reconnect depending on the error; we need explicit retry logic.

**Callers** that go through the `VectorStore` interface:
- `index_worker.go` — `Store` (embedding batches), `EnsureCollection` (once)
- `retriever.go` — `Search` (query)
- `eval/pipeline.go` — `Search` (evaluation queries)

---

## Phase 0: Track `lastDSN` + extract inner methods

**Files:**
- `internal/store/qdrant.go`

**What changes:**

Add `lastDSN` field to `QdrantStore` and save it during `Connect`:

```go
type QdrantStore struct {
    client  *qdrant.GrpcClient
    apiKey  string
    lastDSN string
}

func (s *QdrantStore) Connect(ctx context.Context, dsn string) error {
    s.lastDSN = dsn
    // ... existing code ...
}
```

Extract the inner logic of each public method into a private `*Once` variant (no retry). The public method becomes a thin wrapper that calls the inner variant with retry. This keeps the retry logic in one place.

```go
func (s *QdrantStore) storeOnce(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
    // existing Store body — no retry
}

func (s *QdrantStore) searchOnce(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error) {
    // existing Search body — no retry
}

func (s *QdrantStore) ensureCollectionOnce(ctx context.Context, name string, vectorSize int, distance string) error {
    // existing EnsureCollection body — no retry
}
```

**Tests:**

```
TestQdrantStore_LastDSN
    Connect with DSN "http://qdrant:6334"
    Verify s.lastDSN == "http://qdrant:6334"

TestQdrantStore_StoreOnce_NotConnected
    Call storeOnce with nil client
    Verify returns "not connected" error
```

**Verify:** `go test ./internal/store/`

---

## Phase 1: `reconnect()` helper + `isConnError()` detector

**Files:**
- `internal/store/qdrant.go`

**What changes:**

Add a `reconnect` method and a connection-error detector:

```go
func (s *QdrantStore) reconnect(ctx context.Context) error {
    if s.client != nil {
        s.client.Close()
        s.client = nil
    }
    if s.lastDSN == "" {
        return fmt.Errorf("no last DSN — Connect was never called")
    }
    return s.Connect(ctx, s.lastDSN)
}
```

Connection error detection needs to match gRPC status codes. The qdrant client uses standard gRPC. We can detect via `google.golang.org/grpc/status`:

```go
import (
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

func isConnError(err error) bool {
    st, ok := status.FromError(err)
    if !ok {
        return false
    }
    switch st.Code() {
    case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
        return true
    }
    return false
}
```

If the `go.mod` doesn't already have `google.golang.org/grpc`, add it via `go get google.golang.org/grpc` (it's likely a transitive dependency of the qdrant client).

**Tests:**

```
TestQdrantStore_Reconnect
    Connect, close client manually, call reconnect
    Verify s.client is non-nil after reconnect

TestQdrantStore_Reconnect_NoDSN
    Call reconnect on a store that was never connected
    Verify error
```

**Verify:** `go test ./internal/store/`

---

## Phase 2: Retry wrapper on `Store`, `Search`, `EnsureCollection`

**Files:**
- `internal/store/qdrant.go`

**What changes:**

Wrap each public method with a retry loop. Pattern:

```go
func (s *QdrantStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
    for attempt := 0; attempt < 3; attempt++ {
        if attempt > 0 {
            if err := s.reconnect(ctx); err != nil {
                return fmt.Errorf("reconnect failed: %w", err)
            }
        }
        err := s.storeOnce(ctx, collectionName, chunks)
        if err == nil {
            return nil
        }
        if isConnError(err) {
            continue
        }
        return err
    }
    return fmt.Errorf("store failed after 3 attempts")
}
```

Same pattern for `Search` and `EnsureCollection`. The `EnsureCollection` method is called once per collection lifecycle (not per-batch), so retries matter less but are still applied for consistency.

**Tests:**

A full integration test against a real Qdrant instance would require:
1. Connecting to Qdrant
2. Using `StoreOnce` to insert data
3. Restarting Qdrant (or simulating a connection drop)
4. Calling `Store` and verifying it reconnects and succeeds

Since we can't restart Qdrant in unit tests, use a **mock gRPC server** or test the error-handling paths:
- Create a `QdrantStore`, don't connect
- Call `Store` → should retry, `reconnect` will fail (no lastDSN), returns error
- Verify the error message mentions reconnection failure

```go
TestQdrantStore_Retry_ReconnectFails
    Create QdrantStore with no connection
    Call Store
    Verify error contains "reconnect failed"

TestQdrantStore_Retry_NonConnError
    Manually inject a non-connection error
    Verify no retry (immediate return)
```

**Verify:** `go test ./internal/store/`

---

## Dependency graph

```
Phase 0 (lastDSN + once methods) ──► Phase 1 (reconnect + isConnError)
                                            │
                                            └──► Phase 2 (retry wrappers)
```

Phases build sequentially: Phase 1 needs Phase 0, Phase 2 needs both.

---

## Testing strategy summary

| Phase | What's tested | How | Key edge cases |
|-------|--------------|-----|----------------|
| 0 | `lastDSN` saved, `*Once` methods extracted | Direct struct field check, call with nil client | Never connected, already connected |
| 1 | `reconnect()` cleans up + reconnects, `isConnError` detects gRPC status codes | Connect + close + reconnect; test with gRPC status errors | No lastDSN, transport vs non-transport errors |
| 2 | Public methods retry on conn errors, skip on non-conn errors | Mock error injection | 3 attempts exhausted, reconnect succeeds mid-retry, mixed error types |
