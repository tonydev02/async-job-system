# PHASE-PLAN.md

## Phase
03 — Concurrency and Worker Safety

## Goal
Harden duplicate-delivery behavior and multi-worker race safety while preserving explicit, DB-backed lifecycle correctness.

## Why this phase matters
This phase validates that the system remains correct when several workers compete for the same jobs and when one process handles multiple jobs concurrently.

## In scope
- configurable worker concurrency inside one worker process (`WORKER_CONCURRENCY`)
- worker run-loop refactor to bounded goroutine worker-pool model
- graceful shutdown behavior for in-flight jobs (stop dequeue, allow in-flight completion up to timeout)
- explicit race-safety validation of DB lifecycle transitions under concurrent workers
- stronger structured logs for concurrent processing (`job_id`, `worker_instance`, transition result)

## Out of scope
- visibility-timeout crash recovery for stuck `processing` jobs (Phase 04)
- DLQ or backoff policy changes
- HTTP API contract changes
- distributed coordination beyond current DB-guarded transitions

## Deliverables
- worker runtime config support for `WORKER_CONCURRENCY` (`>0`, default `4`)
- concurrency-safe worker runtime implementation with bounded in-process parallelism
- graceful shutdown path that drains in-flight work within configured timeout
- new concurrency-focused tests in worker and repository layers
- Phase 03 planning/research/summary/UAT docs aligned with behavior

## Subtasks

### 1. Add worker concurrency configuration
Purpose: make in-process worker concurrency explicit, configurable, and validated before runtime starts.

Files/functions:
- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/worker/main.go`

Done when:
- `WORKER_CONCURRENCY` is loaded with default `4`
- non-integer and non-positive values fail config loading
- worker startup logs include the configured concurrency value

### 2. Refactor worker runtime to bounded concurrency
Purpose: replace unbounded per-message processing with a fixed-size worker pool while preserving existing retry-dispatch startup behavior.

Files/functions:
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`

Done when:
- `Run` starts exactly the configured number of worker slots
- dequeued messages are passed through a bounded internal work channel
- active processing never exceeds configured concurrency
- retry dispatcher startup from `Run` remains covered by tests

### 3. Preserve duplicate-delivery safety under concurrency
Purpose: prove duplicate Redis deliveries cannot create duplicate processing or terminal state transitions.

Files/functions:
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`
- `internal/jobs/postgres/repository.go`
- `internal/jobs/postgres/repository_test.go`

Done when:
- concurrent duplicate messages for one `job_id` race on `pending -> processing`
- exactly one claim winner proceeds to processing
- skipped duplicate claims are logged and do not apply terminal transitions
- tests assert the persisted lifecycle state, not just timing behavior

### 4. Implement graceful shutdown drain behavior
Purpose: stop accepting new work on shutdown while giving already in-flight jobs a bounded chance to finish.

Files/functions:
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`
- `cmd/worker/main.go`

Done when:
- cancellation stops dequeue acceptance
- in-flight jobs continue on a drain context
- `WORKER_SHUTDOWN_TIMEOUT` cancels remaining in-flight work after the timeout
- `Run` returns after the drain completes or the timeout expires

### 5. Add repository contention coverage
Purpose: validate that Postgres guarded transitions remain single-winner under concurrent callers.

Files/functions:
- `internal/jobs/postgres/repository_test.go`

Done when:
- concurrent `MarkProcessing` attempts produce exactly one successful transition
- concurrent terminal transition attempts apply at most once
- concurrent `ClaimDueRetries` calls do not return duplicate job IDs
- tests use start barriers/wait groups and persisted-state assertions

### 6. Strengthen concurrent logging traceability
Purpose: make worker behavior debuggable when several worker slots process jobs at the same time.

Files/functions:
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`

Done when:
- job-related logs include `job_id`
- worker-pool logs include stable worker context such as `worker_slot`
- guarded transition logs include transition name, applied flag, and outcome
- log assertions cover claim win/skip paths

### 7. Close docs and validation evidence
Purpose: keep phase docs aligned with implemented behavior and provide reproducible validation notes.

Files/functions:
- `.planning/STATE.md`
- `.planning/phases/03-concurrency-and-worker-safety/PHASE-SUMMARY.md`
- `.planning/phases/03-concurrency-and-worker-safety/PHASE-UAT.md`
- `README.md`

Done when:
- summary lists implemented concurrency, shutdown, logging, and contention behavior
- UAT records test commands and results
- README documents worker concurrency and shutdown behavior
- `go test ./...` and `go vet ./...` pass locally

## Acceptance criteria
- worker process supports configurable bounded concurrency via `WORKER_CONCURRENCY`
- duplicate queue messages for the same `job_id` do not produce duplicate terminal transitions
- concurrent workers racing on the same job still apply at most one guarded state transition per stage
- worker shutdown stops dequeueing and waits for in-flight jobs up to timeout
- logs remain traceable under concurrency and include `job_id` and worker context
- local validation commands pass for changed code paths

## Public interfaces / contracts
- HTTP API: no changes
- Worker runtime config additions:
  - `WORKER_CONCURRENCY` (int, `>0`, default `4`)
- Internal lifecycle contract remains explicit and DB-guarded:
  - `pending -> processing`
  - `processing -> completed | pending(retry) | failed`

## Implementation notes
- keep Postgres as source of truth for lifecycle state
- keep Redis as transport only
- preserve existing guarded SQL transition patterns (`status` predicates + affected-row checks)
- avoid introducing framework-level abstractions; keep runtime flow explicit and testable
