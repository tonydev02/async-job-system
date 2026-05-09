# PHASE-UAT.md

## Objective
Validate concurrency and worker-safety behavior under duplicate delivery and multi-worker contention.

## Test cases

### 1. Duplicate delivery safety
- [ ] enqueue duplicate messages with the same `job_id`
- [ ] verify only one handler path applies `pending -> processing`
- [ ] verify duplicate attempts are skipped without duplicate terminal transitions

### 2. Bounded in-process concurrency
- [x] run worker with explicit bounded worker count in runtime test configuration
- [x] verify active processing does not exceed configured bound
- [ ] verify throughput increases when concurrency is raised (sanity check)

### 3. Graceful shutdown drain
- [x] trigger shutdown during active processing
- [x] verify dequeue loop stops accepting new messages
- [x] verify in-flight jobs are allowed to finish up to timeout
- [x] verify timeout cancels context-aware in-flight work explicitly
- [x] verify timeout stops waiting when in-flight work does not exit on context cancellation

### 4. Repository contention safety
- [ ] concurrent `MarkProcessing` calls on same job yield exactly one success
- [ ] concurrent terminal/failure transition attempts apply at most once
- [ ] concurrent `ClaimDueRetries` callers do not duplicate claimed job IDs

### 5. Logging traceability under concurrency
- [x] confirm job-related logs include `job_id`
- [x] confirm worker context fields (`worker_slot`, transition result) are present in concurrent paths

## Command validation
- [x] `go test ./internal/worker ./internal/config ./cmd/worker`
- [x] `go test ./...`
- [x] `go vet ./...`

## Automated evidence captured
- [ ] worker duplicate-delivery contention unit tests
- [x] worker bounded-concurrency unit tests
- [x] worker graceful-shutdown drain tests
- [ ] repository concurrent transition tests
- [ ] repository concurrent due-retry claim tests
- [x] worker claim win/skip transition log assertions

## Manual verification
- [ ] run local API + multiple worker processes against same Postgres/Redis
- [ ] capture no-duplicate-terminal-transition evidence from DB + logs
- [ ] capture shutdown drain behavior evidence
