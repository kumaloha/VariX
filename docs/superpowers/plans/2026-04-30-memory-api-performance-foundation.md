# Memory API Performance Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the storage and API foundation for eventually consistent memory reads, projection refresh scheduling, and LLM stage caching.

**Architecture:** Keep Facet-facing reads on precomputed read models and move slow work behind durable cache/dirty markers. The first implementation stays in SQLite and standard-library HTTP so the system can run locally without Redis, Kafka, or a new web framework.

**Tech Stack:** Go, SQLite via `modernc.org/sqlite`, standard library `net/http`, existing VariX contentstore and CLI patterns.

---

### Task 1: LLM Stage Cache Storage

**Files:**
- Modify: `varix/storage/contentstore/sqlite_store.go`
- Create: `varix/storage/contentstore/sqlite_llm_cache.go`
- Test: `varix/storage/contentstore/sqlite_llm_cache_test.go`

- [ ] Add an `llm_cache_entries` table keyed by `cache_key`.
- [ ] Add `LLMCacheMode` values `read-through`, `refresh`, and `off` for caller policy.
- [ ] Add `GetLLMCacheEntry` and `UpsertLLMCacheEntry` methods.
- [ ] Test exact-key hit, miss, refresh overwrite, and invalid keys.

### Task 2: Projection Dirty Marker Storage

**Files:**
- Modify: `varix/storage/contentstore/sqlite_store.go`
- Create: `varix/storage/contentstore/sqlite_projection_dirty.go`
- Test: `varix/storage/contentstore/sqlite_projection_dirty_test.go`

- [ ] Add a `projection_dirty_marks` table keyed by `user_id, layer, subject, ticker, horizon`.
- [ ] Add `MarkProjectionDirty`, `ListProjectionDirtyMarks`, and `ClearProjectionDirtyMark`.
- [ ] Test coalescing many marks into one pending row and clearing only the requested row.

### Task 3: Deferred Content Graph Persistence

**Files:**
- Modify: `varix/storage/contentstore/sqlite_event_input.go`
- Test: `varix/storage/contentstore/sqlite_memory_content_graph_test.go`

- [ ] Add `PersistMemoryContentGraphDeferred` that writes the content graph and marks projection dirty without synchronously refreshing event/paradigm layers.
- [ ] Keep existing `PersistMemoryContentGraph` synchronous for compatibility with current tests and operator commands.
- [ ] Test that deferred persistence writes the graph, marks dirty, and does not create event graphs immediately.

### Task 4: Facet-Oriented Subject Memory API

**Files:**
- Create: `varix/api/server.go`
- Create: `varix/api/server_test.go`
- Modify: `varix/cmd/cli/main.go`
- Modify: `varix/cmd/cli/command_groups.go`
- Create: `varix/cmd/cli/serve_commands.go`
- Test: `varix/cmd/cli/main_test.go`

- [ ] Add standard-library HTTP handlers for `/healthz`, `/memory/subjects/{subject}/timeline`, `/memory/subjects/{subject}/horizons`, and `/memory/subjects/{subject}/experience`.
- [ ] Include `freshness` metadata in subject horizon and experience responses.
- [ ] Add `varix serve --addr :8000` to run the API locally.
- [ ] Test JSON shape and argument validation.

### Task 5: Verification

**Files:**
- All touched files.

- [ ] Run `go test -count=1 ./storage/contentstore`.
- [ ] Run `go test -count=1 ./api`.
- [ ] Run `go test -count=1 ./cmd/cli`.
- [ ] Run `go test -count=1 ./...`.
- [ ] Run `git diff --check`.
