# Async Job Processing System (Go)

A small async job system in Go with a focus on reliability and operational clarity.

Core goals:

- queue-based workflows
- explicit job lifecycle persistence
- bounded retries and terminal failure handling
- worker reliability under duplicate delivery
- bounded worker concurrency and graceful shutdown behavior
- operationally useful logs and testable design

## What This Project Focuses On

This is intentionally not a feature-heavy system. The emphasis is on correctness and clear behavior under failure:

- source of truth boundaries (`Postgres` vs `Redis`)
- idempotency and duplicate-delivery safety
- state-machine-like transitions instead of ad hoc flags
- recovery-oriented retry dispatch
- explicit, reviewable code over heavy abstractions

## Architecture

```text
Client -> HTTP API -> Postgres (create pending job)
                  -> Redis (enqueue job id)

Worker -> Redis (dequeue job id)
       -> Postgres (pending -> processing)
       -> Processor
       -> Postgres (processing -> completed) OR failure transition

Retry Dispatcher Loop (in worker runtime)
       -> Postgres ClaimDueRetries(next_run_at <= now)
       -> Redis re-enqueue
       -> on enqueue failure: Postgres RescheduleRetry
```

### Design Rules

- Postgres is the source of truth for lifecycle state.
- Redis is transport/buffering, not truth.
- Every state transition is persisted in DB.
- Worker behavior is explicit and traceable in logs (`job_id` included).
- Worker concurrency is bounded inside each worker process.

## Job Lifecycle Model

Statuses:

- `pending`
- `processing`
- `completed`
- `failed`

Failure transition from `processing` is atomic:

- if `attempt < max_attempts`: transition to `pending`, set `next_run_at`
- else: transition to terminal `failed`, set `completed_at`

Due retries are claimed with `FOR UPDATE SKIP LOCKED` semantics in the repository layer to support concurrent workers safely.

## Worker Concurrency And Shutdown

`WORKER_CONCURRENCY` controls how many jobs one worker process can process at the same time.

- Default: `4` when loaded through `internal/config`.
- Valid values: integers greater than `0`.
- Runtime behavior: the worker starts a fixed-size in-process worker pool and feeds dequeued messages into that pool. This avoids unbounded goroutine creation while still allowing multiple jobs to make progress concurrently.

Duplicate delivery remains safe because each dequeued message must first win the guarded Postgres transition from `pending` to `processing`. If another worker or worker slot already claimed the job, the transition is skipped and the duplicate message does not run the processor or apply another terminal transition.

Shutdown is drain-oriented:

- On cancellation or process signal, the worker stops accepting newly dequeued messages.
- The internal work channel is closed so idle worker slots exit.
- In-flight jobs keep running on a drain context so normal completion can persist final state.
- `WORKER_SHUTDOWN_TIMEOUT` bounds the drain wait. Its default is `10s`; when it expires, the drain context is canceled so context-aware processors and repository calls can stop.

## Current Phase Status

Phase 03 (Concurrency and Worker Safety) is complete.

Implemented:

- bounded retry/terminal failure transition (`HandleProcessingFailure`)
- due retry claiming + re-dispatch loop in worker
- immediate retry dispatch on worker startup
- enqueue-failure safety (`RescheduleRetry`) so retries are not dropped
- retry runtime config/env wiring:
  - `RETRY_DELAY`
  - `RETRY_DISPATCH_INTERVAL`
  - `RETRY_DISPATCH_BATCH_SIZE`
  - `RETRY_REENQUEUE_DELAY`
- configurable worker concurrency via `WORKER_CONCURRENCY`:
  - default `4`
  - fail-fast config validation for non-positive and non-integer values
- bounded in-process worker pool runtime
- graceful shutdown drain bounded by `WORKER_SHUTDOWN_TIMEOUT`
- duplicate-delivery and concurrent repository transition safety tests
- worker logs with `job_id`, `worker_slot`, and transition outcome fields

Next:

- Phase 04: visibility timeout and recovery for jobs stuck in `processing` after crashes/timeouts.

## Project Structure

```text
cmd/
  api-uat/        # local API bootstrap (UAT-focused)
  worker/         # worker process entrypoint
internal/
  httpapi/        # HTTP handlers and router
  jobs/           # domain + repository contracts
  jobs/postgres/  # Postgres repository implementation
  queue/          # queue abstraction
  queue/redis/    # Redis queue implementation
  worker/         # worker runtime + retry dispatcher + processor
migrations/       # SQL schema migrations
.planning/        # phase plans/research/summary/UAT tracking
```

## Quickstart

### 1) Prerequisites

- Go 1.24+
- PostgreSQL
- Redis
- `psql`

### 2) Create schema

```bash
psql "$DATABASE_URL" -f migrations/000001_create_jobs.up.sql
```

### 3) Run worker

```bash
export DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5432/async_jobs?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export REDIS_DB='0'
export REDIS_QUEUE_KEY='jobs:queue'
export REDIS_BLOCK_TIMEOUT='3s'
export WORKER_CONCURRENCY='4'
export WORKER_SHUTDOWN_TIMEOUT='10s'
export LOG_LEVEL='info'

go run ./cmd/worker
```

Optional failure injection for retry behavior demo:

```bash
export PROCESSOR_FAIL_JOB_ID='<job-uuid>'
```

### 4) Run API (UAT bootstrap)

```bash
go run ./cmd/api-uat
```

Note: `cmd/api-uat` currently uses hardcoded local connection settings intended for local UAT.

### 5) Create and inspect a job

```bash
curl -i -X POST http://localhost:8080/jobs \
  -H 'content-type: application/json' \
  -d '{"payload":{"task":"email","to":"user@example.com"}}'

curl -i http://localhost:8080/jobs/<job_id>
```

## Testing

Main validation commands:

```bash
go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi
go test ./...
go vet ./...
```

Worker tests include dispatcher coverage:

- claim + enqueue flow
- enqueue-failure reschedule
- claim error path
- immediate dispatch on startup
- bounded worker-pool concurrency
- duplicate-delivery contention
- graceful shutdown drain and timeout behavior
- transition logging fields for claim win/skip paths

Repository tests include contention coverage:

- concurrent `MarkProcessing` single-winner behavior
- concurrent terminal transition attempts applying at most once
- concurrent `ClaimDueRetries` callers avoiding duplicate claimed job IDs

## Notes

- API and worker are separately runnable.
- Worker runtime behavior is configurable via:
  - `WORKER_CONCURRENCY`
  - `WORKER_SHUTDOWN_TIMEOUT`
  - `RETRY_DELAY`
  - `RETRY_DISPATCH_INTERVAL`
  - `RETRY_DISPATCH_BATCH_SIZE`
  - `RETRY_REENQUEUE_DELAY`
