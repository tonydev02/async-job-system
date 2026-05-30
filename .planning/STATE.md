# STATE.md

## Project
Async Job Processing System

## Current phase
04 - Visibility Timeout and Recovery

## Current status
Phase 04 implementation and documentation are complete; stale `processing` recovery, worker-owned recovery scanning, runtime configuration, concurrency-safe repository recovery, and UAT evidence are captured.

## Objective
Recover jobs stuck in `processing` after crashes, forced shutdowns, or lost worker execution paths while preserving explicit DB-backed lifecycle transitions.

## Non-goals for current phase
- dead-letter queue behavior
- exponential backoff/jitter policy updates
- HTTP API contract expansion
- separate operator/admin recovery command
- frontend/admin UI

## Done
- Phase 01 remains complete and validated (baseline API -> Postgres -> Redis -> worker flow)
- Phase 02 remains complete and validated:
  - bounded retries
  - terminal failure transitions
  - due-retry dispatch and enqueue-failure reschedule safety
  - retry runtime configuration wiring
- Phase 03 remains complete and validated:
  - configurable bounded worker concurrency
  - graceful shutdown drain behavior
  - worker duplicate-delivery safety
  - repository contention coverage
- Phase 04 planning artifacts are created and synced:
  - `.planning/phases/04-visibility-timeout-and-recovery/PHASE-PLAN.md`
  - `.planning/phases/04-visibility-timeout-and-recovery/PHASE-RESEARCH.md`
  - `.planning/phases/04-visibility-timeout-and-recovery/PHASE-SUMMARY.md`
  - `.planning/phases/04-visibility-timeout-and-recovery/PHASE-UAT.md`
- Phase 04 config slice implemented:
  - `PROCESSING_VISIBILITY_TIMEOUT` default `5m`
  - `PROCESSING_RECOVERY_INTERVAL` default `1m`
  - `PROCESSING_RECOVERY_BATCH_SIZE` default `10`
  - `PROCESSING_RECOVERY_RETRY_DELAY` defaults to `RETRY_DELAY`
  - non-positive and malformed values fail config loading
- Phase 04 repository recovery slice implemented:
  - `RecoverStaleProcessing` recovers `processing` rows older than visibility timeout
  - retryable stale jobs transition to `pending` with `next_run_at`
  - exhausted stale jobs transition to terminal `failed`
  - recovery does not increment `attempt`
  - concurrent callers use `FOR UPDATE SKIP LOCKED` to avoid duplicate recovery
- Phase 04 worker runtime slice implemented:
  - recovery scanner starts from `Worker.Run` alongside the retry dispatcher
  - scanner runs once immediately and then on configured interval
  - scanner errors are logged and do not stop queue consumption
  - recovered retryable jobs are not directly enqueued by recovery; due-retry dispatch remains responsible for Redis transport
- Phase 04 logging and validation slice implemented:
  - recovery logs include `job_id`, transition, outcome, decision, attempt, max attempts, and visibility timeout
  - config, worker, repository, and HTTP fake tests are updated for the expanded repository contract
  - README documents recovery lifecycle and runtime configuration
  - UAT records passing command validation from 2026-05-30

## Next milestone
begin Phase 05 planning for observability and ops

## Risks / open questions
- Postgres integration tests require `TEST_DATABASE_URL` for real database execution; without it, package tests skip database-backed cases per existing test behavior
- dead-letter flow remains deferred, so exhausted timeout recovery currently uses the existing terminal `failed` state
