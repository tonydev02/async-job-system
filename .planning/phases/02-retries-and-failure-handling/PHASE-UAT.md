# PHASE-UAT.md

## Objective
Validate retry lifecycle behavior end-to-end for transient and terminal failures.

## Test cases

### 1. Retry scheduling on first failure
- [x] job enters `processing`
- [x] processor failure transitions to `pending`
- [x] `attempt` increments and `next_run_at` is populated
- [x] error text is persisted

### 2. Terminal failure at max attempts
- [x] job with exhausted attempts transitions to `failed`
- [x] `completed_at` is set
- [x] `next_run_at` is cleared

### 3. Due-retry dispatch
- [x] due retries (`next_run_at <= now`) are claimed from Postgres
- [x] claimed rows have `next_run_at` cleared during claim
- [x] claimed job IDs are enqueued back to Redis

### 4. Dispatch enqueue failure safety
- [x] enqueue failure during retry dispatch triggers DB reschedule
- [x] re-scheduled `next_run_at` is set with reenqueue delay

### 5. API visibility
- [x] `GET /jobs/{id}` exposes retry-relevant fields (`attempt`, `max_attempts`, `next_run_at`, `error`)

## Command validation
- [x] `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi`
- [x] `go test ./...`
- [x] `go vet ./...`

## Automated evidence captured
- [x] worker dispatcher claims and enqueues due retries (`TestDispatchDueRetries_ClaimsAndEnqueues`)
- [x] worker dispatcher reschedules on enqueue failure (`TestDispatchDueRetries_EnqueueFailure_Reschedules`)
- [x] worker dispatcher returns claim errors without enqueue attempts (`TestDispatchDueRetries_ClaimDueRetriesError_ReturnsErrorAndSkipsEnqueue`)
- [x] worker dispatcher performs immediate dispatch on startup (`TestRunRetryDispatcher_DispatchesImmediatelyOnStart`)
- [x] worker retry runtime env parsing is covered (`internal/config/config_test.go`)
- [x] worker retry runtime config validation/application is covered (`TestSetRetryRuntimeConfig_*`)
- [x] retry transition before max attempts is covered (`TestRepositoryHandleProcessingFailure_SchedulesRetryBeforeMaxAttempts`)
- [x] terminal transition at max attempts is covered (`TestRepositoryHandleProcessingFailure_MarksTerminalAtMaxAttempts`)
- [x] due-retry claim clears `next_run_at` is covered (`TestRepositoryClaimDueRetries_ClearsNextRunAtOnClaim`)
- [x] API retry metadata visibility is covered (`TestGetJobByID_IncludesRetryMetadataFields`)

## Manual verification
- [x] run local API + worker + Postgres + Redis and capture retry cycle evidence
- [x] capture terminal-failure evidence after final attempt
- [x] capture dispatcher behavior evidence from worker logs

### Manual evidence (2026-04-16 JST)
- Infra used: Docker containers `ajs-postgres` (`127.0.0.1:55432`) and `ajs-redis` (`127.0.0.1:6379`), API `cmd/api-uat`, worker `cmd/worker`.
- Retry scheduling evidence:
  - Created job `4ed58245-c6a1-473e-86ab-995494fbba66`.
  - Worker log: `job failure transitioned to retry ... attempt=1 max_attempts=3 next_run_at=2026-04-16T23:10:19...`.
  - API `GET /jobs/{id}` showed `status=pending`, `attempt=1`, `next_run_at` populated, `error="injected processor failure for UAT"`.
- Terminal failure evidence:
  - Created job `9064dd9f-1061-4b72-8019-a7f2b7663893`, set `max_attempts=1`, ran worker with `PROCESSOR_FAIL_JOB_ID` for that job.
  - Worker log: `job failure transitioned to terminal failed ... attempt=1 max_attempts=1`.
  - API `GET /jobs/{id}` showed `status=failed`, `completed_at` set, `next_run_at=null`.
- Dispatcher log evidence:
  - Worker log included `dispatched job for retry job_id=4ed58245-c6a1-473e-86ab-995494fbba66`.
