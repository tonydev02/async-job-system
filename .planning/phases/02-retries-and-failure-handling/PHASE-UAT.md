# PHASE-UAT.md

## Objective
Validate retry lifecycle behavior end-to-end for transient and terminal failures.

## Test cases

### 1. Retry scheduling on first failure
- [ ] job enters `processing`
- [ ] processor failure transitions to `pending`
- [ ] `attempt` increments and `next_run_at` is populated
- [ ] error text is persisted

### 2. Terminal failure at max attempts
- [ ] job with exhausted attempts transitions to `failed`
- [ ] `completed_at` is set
- [ ] `next_run_at` is cleared

### 3. Due-retry dispatch
- [ ] due retries (`next_run_at <= now`) are claimed from Postgres
- [ ] claimed rows have `next_run_at` cleared during claim
- [ ] claimed job IDs are enqueued back to Redis

### 4. Dispatch enqueue failure safety
- [ ] enqueue failure during retry dispatch triggers DB reschedule
- [ ] re-scheduled `next_run_at` is set with reenqueue delay

### 5. API visibility
- [ ] `GET /jobs/{id}` exposes retry-relevant fields (`attempt`, `max_attempts`, `next_run_at`, `error`)

## Command validation
- [x] `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi`
- [ ] `go test ./...`
- [ ] `go vet ./...`

## Manual verification
- [ ] run local API + worker + Postgres + Redis and capture retry cycle evidence
- [ ] capture terminal-failure evidence after final attempt
- [ ] capture dispatcher behavior evidence from worker logs
