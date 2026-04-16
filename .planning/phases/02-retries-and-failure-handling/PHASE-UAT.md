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

## Manual verification (non-gating smoke)
- Local Redis/Postgres services are not running in this environment; phase closure is based on automated acceptance coverage and validation commands above.
- [ ] run local API + worker + Postgres + Redis and capture retry cycle evidence
- [ ] capture terminal-failure evidence after final attempt
- [ ] capture dispatcher behavior evidence from worker logs
