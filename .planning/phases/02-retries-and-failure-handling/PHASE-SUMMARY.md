# PHASE-SUMMARY.md

## Outcome
Phase 02 is complete.
Current status: bounded retries, terminal failure handling, retry dispatcher flow, enqueue-failure safety, and retry runtime configuration are implemented and validated by automated tests.

## What is finalized
- phase scope and acceptance criteria
- architecture decisions (Postgres-driven retries, bounded attempts, fixed delay)
- implementation plan and test plan

## What is implemented
- repository contract updated with:
  - `HandleProcessingFailure`
  - `ClaimDueRetries`
  - `RescheduleRetry`
- Postgres repository implementation added for:
  - atomic processing failure transition (`retry` vs `terminal_failed`)
  - due-retry claiming path with `FOR UPDATE SKIP LOCKED`
  - retry reschedule path
- retry decision enum standardized to `retry` and `terminal_failed`
- worker processing path migrated from `MarkFailed` to `HandleProcessingFailure`
- worker logs now include explicit retry/terminal failure decision context (`job_id`, decision, attempts, and `next_run_at` for retries)
- worker retry dispatcher runtime added:
  - immediate dispatch on worker startup
  - periodic ticker dispatch for due retries
  - due-retry claim from Postgres + Redis re-enqueue
  - enqueue-failure fallback via `RescheduleRetry`
- test doubles updated to support the new repository interface in:
  - `internal/worker/worker_test.go`
  - `internal/httpapi/jobs_handler_test.go`
- dispatcher-focused worker tests added:
  - `TestDispatchDueRetries_ClaimsAndEnqueues`
  - `TestDispatchDueRetries_EnqueueFailure_Reschedules`
  - `TestDispatchDueRetries_ClaimDueRetriesError_ReturnsErrorAndSkipsEnqueue`
  - `TestRunRetryDispatcher_DispatchesImmediatelyOnStart`
- worker runtime retry config wiring added:
  - `RETRY_DELAY`
  - `RETRY_DISPATCH_INTERVAL`
  - `RETRY_DISPATCH_BATCH_SIZE`
  - `RETRY_REENQUEUE_DELAY`
  - applied in `cmd/worker` via `SetRetryRuntimeConfig`
  - config parsing tests added in `internal/config/config_test.go`
  - worker runtime config validation tests added in `internal/worker/worker_test.go`
- reliability hardening updates:
  - API enqueue failure now triggers retry reschedule to avoid stranded pending rows
  - retry delay SQL now preserves sub-second precision
  - config loader now validates retry env values are positive
- additional phase-closure tests:
  - `TestRepositoryHandleProcessingFailure_SchedulesRetryBeforeMaxAttempts`
  - `TestRepositoryHandleProcessingFailure_MarksTerminalAtMaxAttempts`
  - `TestRepositoryClaimDueRetries_ClearsNextRunAtOnClaim`
  - `TestGetJobByID_IncludesRetryMetadataFields`

## Validation run
- `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi` passed
- `go test ./internal/config ./internal/worker` passed
- `go test ./...` passed
- `go vet ./...` passed
- manual end-to-end UAT smoke (2026-04-16 JST) passed using Docker `ajs-postgres` + `ajs-redis` with live API/worker runtime evidence captured in `PHASE-UAT.md`

## What is pending implementation
- none for Phase 02 scope

## Residual follow-up (non-gating)
- none for Phase 02

## Pairing mode
Implementation will proceed in guided subtasks:
1. define subtask purpose
2. list files/functions to edit
3. user implements
4. review feedback and next subtask
