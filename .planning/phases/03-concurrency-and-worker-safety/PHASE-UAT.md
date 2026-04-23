# PHASE-UAT.md

## Objective
Validate concurrency and worker-safety behavior under duplicate delivery and multi-worker contention.

## Test cases

### 1. Duplicate delivery safety
- [ ] enqueue duplicate messages with the same `job_id`
- [ ] verify only one handler path applies `pending -> processing`
- [ ] verify duplicate attempts are skipped without duplicate terminal transitions

### 2. Bounded in-process concurrency
- [ ] run worker with explicit `WORKER_CONCURRENCY`
- [ ] verify active processing does not exceed configured bound
- [ ] verify throughput increases when concurrency is raised (sanity check)

### 3. Graceful shutdown drain
- [ ] trigger shutdown during active processing
- [ ] verify dequeue loop stops accepting new messages
- [ ] verify in-flight jobs are allowed to finish up to timeout

### 4. Repository contention safety
- [ ] concurrent `MarkProcessing` calls on same job yield exactly one success
- [ ] concurrent terminal/failure transition attempts apply at most once
- [ ] concurrent `ClaimDueRetries` callers do not duplicate claimed job IDs

### 5. Logging traceability under concurrency
- [ ] confirm job-related logs include `job_id`
- [ ] confirm worker context fields (`worker_instance`, transition result) are present in concurrent paths

## Command validation
- [ ] `go test ./internal/worker ./internal/jobs/postgres ./internal/config`
- [ ] `go test ./...`
- [ ] `go vet ./...`

## Automated evidence captured
- [ ] worker duplicate-delivery contention unit tests
- [ ] worker bounded-concurrency unit tests
- [ ] worker graceful-shutdown drain tests
- [ ] repository concurrent transition tests
- [ ] repository concurrent due-retry claim tests

## Manual verification
- [ ] run local API + multiple worker processes against same Postgres/Redis
- [ ] capture no-duplicate-terminal-transition evidence from DB + logs
- [ ] capture shutdown drain behavior evidence
