# PHASE-PLAN.md

## Phase
01 — MVP Job Submission and Background Processing

## Goal
Build the smallest end-to-end async job workflow that is still production-shaped.

## Why this phase matters
This phase proves the core architecture:
API → DB → Redis → Worker → DB → Polling API

## In scope
- job creation endpoint
- job status endpoint
- job persistence in Postgres
- queueing via Redis
- worker consumption
- status transitions
- basic error handling
- local development with Docker Compose

## Out of scope
- retries
- visibility timeout
- dead-letter queue
- multiple job types
- metrics dashboard

## Deliverables
- working local API service
- working local worker service
- schema migration for jobs table
- documented API contract
- demo flow verified end-to-end

## Acceptance criteria
- user can submit a job and receive a job ID
- submitted job is stored as `pending`
- worker eventually changes job to `processing`
- worker eventually changes job to `completed` or `failed`
- `GET /jobs/{id}` reflects the persisted state
- logs include job ID during processing

## Implementation notes
- Postgres is source of truth
- Redis only carries job references
- payload/result stored as JSONB
- worker uses a fake deterministic processor for MVP

## Step 4 scope
- add worker processing loop that continuously dequeues `job_id` from Redis
- apply explicit persisted transitions: `pending -> processing -> completed|failed`
- handle duplicate delivery safely by relying on guarded `MarkProcessing`
- stop gracefully on context cancellation

## Step 4 completion
- implemented `internal/worker` package with:
  - worker run loop (`Run`)
  - guarded claim step (`MarkProcessing`)
  - terminal transitions (`MarkCompleted` / `MarkFailed`)
  - deterministic MVP processor
- added worker unit tests for:
  - duplicate-safe skip path
  - success path
  - processor error path
  - empty dequeue + context cancellation path
  - deterministic processor cancellation behavior

## Step 5 scope
- add runnable worker service entrypoint at `cmd/worker/main.go`
- add service-level worker config loader for local runs (DB/Redis/runtime settings)
- wire dependency construction (Postgres repository, Redis queue adapter, deterministic processor, logger)
- add signal-aware graceful shutdown path for local process execution

## Step 5 completion
- implemented worker runtime entrypoint in `cmd/worker/main.go` with:
  - startup config load + validation
  - signal-aware process lifecycle (`SIGINT`, `SIGTERM`)
  - Postgres and Redis startup connectivity checks
  - explicit dependency wiring into `worker.NewWorker`
- implemented `internal/config` worker config loader with typed env parsing:
  - required `DATABASE_URL`
  - Redis settings (`REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_QUEUE_KEY`, `REDIS_BLOCK_TIMEOUT`)
  - runtime settings (`WORKER_SHUTDOWN_TIMEOUT`, `LOG_LEVEL`)
- extended Redis adapter with `NewRedisClient` startup constructor + `Ping` verification
