# Async Job Processing System (Go)

Production-style async job system built to demonstrate backend engineering fundamentals:

- queue-based workflows
- explicit job lifecycle persistence
- bounded retries and terminal failure handling
- worker reliability under duplicate delivery
- operationally useful logs and testable design

## Why This Project

Many toy async systems stop at "enqueue + background worker." This project focuses on the harder parts interviewers care about:

- source of truth boundaries (`Postgres` vs `Redis`)
- idempotency and duplicate-delivery safety
- state-machine-like transitions instead of ad hoc flags
- recovery-oriented retry dispatch
- clear, reviewable architecture over framework-heavy abstractions

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

## Current Phase Status

Phase 02 (Retries and Failure Handling) is in progress.

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
- targeted worker tests for dispatcher behavior

Pending:

- manual phase UAT evidence capture (local end-to-end run artifacts)

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

Targeted packages:

```bash
go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi
```

Worker tests include dispatcher coverage:

- claim + enqueue flow
- enqueue-failure reschedule
- claim error path
- immediate dispatch on startup

## Interview Talking Points

- Explicitly separated truth (`Postgres`) from transport (`Redis`).
- Designed transitions for correctness first, scale later.
- Used small interfaces and constructor injection for testability.
- Added operational safeguards for real-world failures (duplicate delivery, transient queue errors, graceful shutdown).
- Kept implementation intentionally simple and evolvable (no premature orchestration complexity).
