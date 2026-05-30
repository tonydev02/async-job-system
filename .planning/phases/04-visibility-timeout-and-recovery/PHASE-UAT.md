# PHASE-UAT.md

## Objective
Validate that jobs stuck in `processing` after worker crashes or forced shutdowns are recovered safely and consistently.

## Test cases

### 1. Retryable stale processing recovery
- [x] create or arrange a `processing` job older than `PROCESSING_VISIBILITY_TIMEOUT`
- [x] verify recovery transitions it to `pending`
- [x] verify `next_run_at` is set
- [x] verify `attempt` is not incremented by recovery
- [x] verify recovery error text is recorded

### 2. Terminal stale processing recovery
- [x] create or arrange a stale `processing` job with `attempt >= max_attempts`
- [x] verify recovery transitions it to `failed`
- [x] verify `completed_at` is set
- [x] verify `next_run_at` is cleared

### 3. Fresh and non-processing exclusion
- [x] verify a fresh `processing` job is not recovered
- [x] verify `pending`, `completed`, and `failed` jobs are not recovered

### 4. Concurrent recovery safety
- [x] run concurrent recovery callers
- [x] verify no job ID is recovered by more than one caller
- [x] verify persisted states match exactly one recovery decision per job

### 5. Worker recovery loop
- [x] verify recovery scan runs once on worker startup
- [x] verify recovery scan repeats on configured interval
- [x] verify scanner errors are logged and do not stop worker dequeue processing
- [x] verify cancellation stops the recovery scanner

### 6. Retry dispatcher handoff
- [x] verify recovered retryable jobs are not enqueued directly by recovery
- [x] verify due retry dispatcher later claims recovered jobs through `next_run_at`
- [x] verify enqueue failure still uses existing retry reschedule behavior

### 7. Logging and configuration
- [x] verify recovery logs include `job_id`
- [x] verify recovery logs include decision, attempt, max attempts, timeout, and transition outcome
- [x] verify config defaults are applied
- [x] verify invalid config values fail fast

## Command validation
- [x] `go test ./internal/config ./internal/jobs/postgres ./internal/worker ./cmd/worker`
  - 2026-05-30 result: passed
- [x] `go test ./...`
  - 2026-05-30 result: passed
- [x] `go vet ./...`
  - 2026-05-30 result: passed with no output

## Automated evidence to capture
- [x] repository retryable stale recovery test: `TestRepositoryRecoverStaleProcessing_SchedulesRetryBeforeMaxAttempts`
- [x] repository terminal stale recovery test: `TestRepositoryRecoverStaleProcessing_MarksTerminalAtMaxAttempts`
- [x] repository fresh/non-processing exclusion tests: `TestRepositoryRecoverStaleProcessing_SkipsFreshAndNonProcessingJobs`
- [x] repository concurrent recovery no-duplicate test: `TestRepositoryConcurrentRecoverStaleProcessingDoesNotDuplicateIDs`
- [x] worker startup recovery scan test: `TestRun_StartsProcessingRecoveryScanner`
- [x] worker interval recovery scan test: `TestRunProcessingRecoveryScanner_DispatchesImmediatelyAndOnInterval`
- [x] worker recovery error logging test: `TestRunProcessingRecoveryScanner_LogsErrorsAndStopsOnCancel`
- [x] config default/override/invalid recovery setting tests: `TestLoadWorkerConfig_RetryDefaults`, `TestLoadWorkerConfig_RetryOverrides`, `TestLoadWorkerConfig_InvalidProcessingRecoveryValues`, `TestLoadWorkerConfig_NonPositiveRetryValues`

## Manual verification
- [x] review repository SQL for guarded `processing` predicates and `FOR UPDATE SKIP LOCKED`
- [x] review worker runtime startup to confirm retry dispatcher and recovery scanner both start from `Run`
- [x] review logs for recovery traceability fields
- [x] review README updates for recovery config and lifecycle behavior
