# PHASE-SUMMARY.md

## Outcome
Phase 04 is implemented and validated. The worker now recovers stale `processing` jobs after a configurable visibility timeout using DB-authoritative retry-or-fail transitions.

## What is finalized
- stale `processing` recovery will run inside the worker process
- recovery will use retry-or-fail semantics
- recovery will remain DB-authoritative and concurrency-safe
- recovered retryable jobs will be scheduled through `next_run_at`, not directly enqueued

## What is implemented
- worker runtime config support for:
  - `PROCESSING_VISIBILITY_TIMEOUT` (`5m` default)
  - `PROCESSING_RECOVERY_INTERVAL` (`1m` default)
  - `PROCESSING_RECOVERY_BATCH_SIZE` (`10` default)
  - `PROCESSING_RECOVERY_RETRY_DELAY` (defaults to `RETRY_DELAY`)
- repository recovery contract:
  - `RecoverStaleProcessing`
  - stale eligibility based on `processing` status and `started_at` older than visibility timeout
  - concurrency-safe selection with `FOR UPDATE SKIP LOCKED`
- recovery lifecycle transitions:
  - attempts remaining: `processing -> pending`, set `next_run_at`
  - attempts exhausted: `processing -> failed`, set `completed_at`
  - recovery records error text and does not increment `attempt`
- worker recovery scanner:
  - starts from `Worker.Run`
  - runs immediately on startup and then by interval
  - logs scanner errors without stopping queue consumption
  - leaves Redis re-enqueue to the existing retry dispatcher
- recovery logging:
  - `job_id`
  - transition name and outcome
  - decision
  - attempt/max attempts
  - visibility timeout
- README, planning summary, and UAT evidence updated

## Validation evidence
- `go test ./internal/config ./internal/jobs/postgres ./internal/worker ./cmd/worker` passed on 2026-05-30.
- `go test ./...` passed on 2026-05-30.
- `go vet ./...` passed on 2026-05-30.

## Pairing mode
Implemented in small reviewable chunks:
1. recovery config and runtime wiring - complete
2. repository stale-processing recovery transition - complete
3. worker recovery scanner - complete
4. tests for repository, worker, and config behavior - complete
5. docs and UAT evidence capture - complete
