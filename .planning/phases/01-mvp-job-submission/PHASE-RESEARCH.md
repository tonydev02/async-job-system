# PHASE-RESEARCH.md

## Questions
1. Should the queue store full payload or only job ID?
2. What status transitions are allowed?
3. How should duplicate processing be prevented in MVP?
4. What is the simplest worker loop that is still clean?

## Decisions
### Queue payload
Use `job_id` only.
Reason:
- Postgres remains source of truth
- avoids queue/data divergence
- easier to retry and replay

### Status model
pending -> processing -> completed
pending -> processing -> failed

### Duplicate protection
Worker must re-check persisted job state before processing.
Only jobs still eligible for processing should continue.

## Deferred decisions
- lease/visibility timeout design
- retry backoff strategy
- dead-letter handling

## Step 4 worker loop notes
### Core lifecycle
Worker loop for each dequeued message:
1. `Dequeue` `job_id` from Redis
2. `MarkProcessing(job_id)` in Postgres
3. if transition applied, run processor
4. on processor success, `MarkCompleted(job_id, result)`
5. on processor failure, `MarkFailed(job_id, err)`

This keeps Postgres as the source of truth for every lifecycle transition.

### Duplicate delivery handling
Redis delivery may be duplicated; worker must treat dequeue as "attempt to claim work", not guaranteed ownership.
`MarkProcessing` is the guard rail:
- if it returns `true`, this worker instance claimed the job
- if it returns `false`, job was already claimed or moved; skip processing

### Context cancellation behavior
Worker loop should run until `context.Context` is canceled.
On cancellation:
- stop dequeuing new messages
- return cleanly from the run loop
- rely on persisted transitions so restart is safe

### Validation notes from implementation
- `queue.ErrEmpty` is treated as a non-fatal condition in the run loop
- `MarkCompleted` / `MarkFailed` boolean transition outcomes are checked explicitly
- job processing logs use `job_id` structured field consistently

## Step 5 runtime wiring notes
### Local worker env vars
Worker process should read explicit environment configuration for local run:
- `DATABASE_URL` for Postgres connection string
- `REDIS_ADDR` for Redis host:port
- `REDIS_PASSWORD` and `REDIS_DB` for optional auth/database selection
- `REDIS_QUEUE_KEY` for job queue key name
- `REDIS_BLOCK_TIMEOUT` for blocking dequeue timeout duration
- `WORKER_SHUTDOWN_TIMEOUT` for bounded shutdown/cleanup
- `LOG_LEVEL` for structured logging level

Use safe defaults where possible (for local development) and fail fast for required values (especially `DATABASE_URL`).

### Graceful shutdown expectations
Worker service should be signal aware (`SIGINT`, `SIGTERM`) and cancel root context on signal.
Shutdown flow:
1. stop accepting new dequeue work by canceling run context
2. allow in-flight operation to observe cancellation and return
3. close Redis and Postgres clients cleanly
4. exit with non-zero status on startup/wiring failures

This keeps local behavior predictable and production-shaped while preserving clear lifecycle ownership.

### Step 5 implementation notes
- worker process now runs from a dedicated executable (`cmd/worker/main.go`) rather than package-only wiring
- startup dependencies are validated eagerly (Postgres ping + Redis ping) to fail fast on local misconfiguration
- shutdown timeout is applied as a bounded wait during signal-driven termination, avoiding fixed-lifetime worker behavior
