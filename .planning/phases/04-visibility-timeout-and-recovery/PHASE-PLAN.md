# PHASE-PLAN.md

## Phase
04 - Visibility Timeout and Recovery

## Goal
Recover jobs that remain stuck in `processing` after worker crashes, forced shutdowns, or lost execution paths.

## Why this phase matters
Phase 03 made duplicate delivery and concurrent worker claims safe. Phase 04 closes the remaining reliability gap: once a worker wins `pending -> processing`, the system must not rely on that worker staying alive forever.

This phase keeps Postgres as the source of truth and makes stale `processing` recovery an explicit lifecycle transition.

## In scope
- configurable processing visibility timeout
- worker-owned periodic recovery scanner
- repository method for atomically recovering stale `processing` jobs
- retry-or-fail recovery semantics:
  - attempts remaining: `processing -> pending` with `next_run_at`
  - attempts exhausted: `processing -> failed`
- recovery logging with `job_id`, decision, attempts, and transition outcome
- repository and worker tests for stale processing recovery

## Out of scope
- dead-letter queue behavior
- exponential backoff or jitter
- HTTP API contract changes
- separate operator/admin recovery command
- frontend/admin UI

## Deliverables
- worker runtime config for:
  - `PROCESSING_VISIBILITY_TIMEOUT`
  - `PROCESSING_RECOVERY_INTERVAL`
  - `PROCESSING_RECOVERY_BATCH_SIZE`
  - optional `PROCESSING_RECOVERY_RETRY_DELAY`
- repository contract and Postgres implementation for stale `processing` recovery
- worker recovery loop started by `Worker.Run`
- tests covering repository recovery transitions, concurrent recovery safety, config validation, and worker loop behavior
- Phase 04 summary and UAT evidence updated after implementation

## Subtasks

### 1. Add recovery runtime configuration
Purpose: make recovery timing explicit and fail-fast on unsafe values.

Done when:
- visibility timeout, recovery interval, batch size, and retry delay are loaded through worker config
- invalid durations and non-positive batch sizes fail config loading
- `cmd/worker` wires recovery config into the worker runtime

### 2. Add repository recovery contract
Purpose: keep stale job recovery authoritative in Postgres.

Done when:
- repository exposes an explicit stale-processing recovery method
- stale eligibility is based on `status = processing` and `started_at <= now - visibility_timeout`
- rows are claimed with concurrency-safe SQL such as `FOR UPDATE SKIP LOCKED`
- recovery returns enough detail for worker logs and tests

### 3. Implement retry-or-fail recovery transition
Purpose: treat timeout recovery like a deliberate lifecycle decision, not a queue-only replay.

Done when:
- stale jobs with attempts remaining become `pending` and receive `next_run_at`
- stale jobs at max attempts become terminal `failed`
- recovery does not increment `attempt`
- fresh `processing` jobs and non-processing jobs are not changed

### 4. Add worker recovery scanner
Purpose: run recovery continuously without adding a separate service.

Done when:
- `Worker.Run` starts the recovery loop alongside the retry dispatcher
- the scanner runs once on startup and then on `PROCESSING_RECOVERY_INTERVAL`
- scanner errors are logged and do not stop queue consumption
- recovered pending jobs are not enqueued directly; retry dispatch remains responsible for due re-enqueue

### 5. Add tests and close docs
Purpose: prove correctness under stale jobs, concurrency, and runtime wiring.

Done when:
- repository tests cover retry recovery, terminal recovery, fresh-job exclusion, and concurrent recovery callers
- worker tests cover startup scan, interval scan, error logging, and cancellation
- config tests cover defaults, overrides, and invalid values
- summary and UAT files are updated with implementation details and validation evidence

## Acceptance criteria
- jobs stuck in `processing` beyond the configured visibility timeout are recovered from Postgres
- recovery is safe when multiple worker processes scan concurrently
- recovered jobs follow bounded-attempt semantics and do not exceed `max_attempts`
- logs make recovery decisions traceable by `job_id`
- `go test ./...` and `go vet ./...` pass after implementation

## Public interfaces / contracts
- HTTP API: no changes
- Worker runtime config additions:
  - `PROCESSING_VISIBILITY_TIMEOUT` duration, `>0`, default `5m`
  - `PROCESSING_RECOVERY_INTERVAL` duration, `>0`, default `1m`
  - `PROCESSING_RECOVERY_BATCH_SIZE` int, `>0`, default `10`
  - `PROCESSING_RECOVERY_RETRY_DELAY` duration, `>0`, default same as `RETRY_DELAY`
- Internal lifecycle contract expands to include:
  - stale `processing -> pending` for retry
  - stale `processing -> failed` when attempts are exhausted

## Implementation notes
- keep Redis as transport only
- do not enqueue directly from recovery; use `next_run_at` plus the existing retry dispatcher
- preserve guarded SQL transition patterns and affected-row/result checks
- prefer explicit structs over generic recovery abstractions
