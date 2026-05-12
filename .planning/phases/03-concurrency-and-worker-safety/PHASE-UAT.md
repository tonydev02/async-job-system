# PHASE-UAT.md

## Objective
Validate concurrency and worker-safety behavior under duplicate delivery and multi-worker contention.

## Test cases

### 1. Duplicate delivery safety
- [x] add worker test coverage for duplicate messages with the same `job_id`
- [x] verify only one handler path applies `pending -> processing`
- [x] verify duplicate attempts are skipped without duplicate terminal transitions

### 2. Bounded in-process concurrency
- [x] run worker with explicit bounded worker count in runtime test configuration
- [x] verify active processing does not exceed configured bound
- [x] add explicit worker test coverage that active processing never exceeds configured concurrency
- [x] verify multiple jobs can be active concurrently while staying within the configured bound

### 3. Graceful shutdown drain
- [x] trigger shutdown during active processing
- [x] verify dequeue loop stops accepting new messages
- [x] add explicit worker test coverage that cancellation stops intake while allowing in-flight completion
- [x] verify in-flight jobs are allowed to finish up to timeout
- [x] verify timeout cancels context-aware in-flight work explicitly
- [x] verify timeout stops waiting when in-flight work does not exit on context cancellation

### 4. Repository contention safety
- [x] concurrent `MarkProcessing` calls on same job yield exactly one success
- [x] concurrent terminal/failure transition attempts apply at most once
- [x] concurrent `ClaimDueRetries` callers do not duplicate claimed job IDs

### 5. Logging traceability under concurrency
- [x] confirm job-related logs include `job_id`
- [x] confirm worker context fields (`worker_slot`, transition result) are present in concurrent paths

## Command validation
- [x] `go test ./internal/worker ./internal/config ./cmd/worker`
  - 2026-05-12 result: passed
  - packages: `internal/worker`, `internal/config`, `cmd/worker`
- [x] `go test ./internal/jobs/postgres`
  - 2026-05-12 result: passed
- [x] `go test ./...`
  - 2026-05-12 result: passed
  - covered packages include `internal/config`, `internal/httpapi`, `internal/jobs/postgres`, and `internal/worker`
- [x] `go vet ./...`
  - 2026-05-12 result: passed with no output

## Automated evidence captured
- [x] worker duplicate-delivery contention unit test: `TestRun_DuplicateDeliveryConcurrentOnlyOneProcessingClaimWins`
- [x] worker bounded-concurrency unit tests: `TestRun_UsesBoundedWorkerPoolConcurrency`, `TestRun_ActiveProcessingNeverExceedsConfiguredConcurrency`
- [x] worker graceful-shutdown drain tests: `TestRun_CancelStopsAcceptingNewWork`, `TestRun_CancelStopsIntakeWhileAllowingInFlightCompletion`, `TestRun_CancelDrainsInFlightJobsWithoutCancelingJobContext`, `TestRun_ShutdownTimeoutCancelsInFlightJobContext`, `TestRun_ShutdownTimeoutReturnsWhenInFlightJobIgnoresContext`
- [x] repository concurrent transition tests: `TestRepositoryConcurrentMarkProcessingSingleWinner`, `TestRepositoryConcurrentTerminalTransitionsAtMostOneApplies`
- [x] repository concurrent due-retry claim test: `TestRepositoryConcurrentClaimDueRetriesDoesNotDuplicateIDs`
- [x] config validation tests: `TestLoadWorkerConfig_RetryDefaults`, `TestLoadWorkerConfig_RetryOverrides`, `TestLoadWorkerConfig_InvalidWorkerConcurrency`, `TestLoadWorkerConfig_NonPositiveWorkerConcurrency`
- [x] worker claim win/skip transition log assertions: `TestHandleMessage_LogsClaimTransitionOutcomesWithWorkerSlot`

## Manual verification
- [x] reviewed `internal/worker/worker.go` shutdown path:
  - `Run` starts a fixed-size worker pool from configured concurrency
  - cancellation stops dequeue acceptance and closes the internal message channel
  - in-flight handlers use a drain context created with `context.WithoutCancel(ctx)`
  - `waitForInFlightJobs` cancels that drain context after `WORKER_SHUTDOWN_TIMEOUT`
- [x] reviewed `cmd/worker/main.go` runtime wiring:
  - `WORKER_CONCURRENCY` is passed through `SetConcurrency`
  - `WORKER_SHUTDOWN_TIMEOUT` is passed through `SetShutdownTimeout`
  - signal handling applies the same timeout as a bounded process shutdown wait
- [x] reviewed `internal/config/config.go`:
  - `WORKER_CONCURRENCY` default is `4`
  - non-positive and non-integer values fail config loading
- [x] reviewed log fields used in concurrent paths:
  - worker logs include `worker_slot`
  - job transition logs include `job_id`, `transition`, `transition_applied`, and `transition_outcome`

Manual local API + multiple worker process evidence was not captured in this phase because the repository does not include local Postgres/Redis orchestration. The safety guarantees above are validated by unit and repository integration tests using guarded DB transitions and concurrent callers.
