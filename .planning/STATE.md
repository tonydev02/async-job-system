# STATE.md

## Project
Async Job Processing System

## Current phase
03 — Concurrency and Worker Safety

## Current status
Phase 03 implementation and documentation are complete; bounded worker-pool runtime, graceful shutdown-drain behavior, repository contention coverage, and UAT evidence are captured.

## Objective
Harden duplicate-delivery handling and multi-worker race safety while preserving explicit DB-backed lifecycle transitions.

## Non-goals for current phase
- visibility-timeout and stale `processing` recovery (Phase 04)
- dead-letter flow
- exponential backoff/jitter policy updates
- HTTP API contract expansion

## Done
- Phase 01 remains complete and validated (baseline API -> Postgres -> Redis -> worker flow)
- Phase 02 remains complete and validated:
  - bounded retries
  - terminal failure transitions
  - due-retry dispatch and enqueue-failure reschedule safety
  - retry runtime configuration wiring
- Phase 03 planning artifacts are now created:
  - `.planning/phases/03-concurrency-and-worker-safety/PHASE-PLAN.md`
  - `.planning/phases/03-concurrency-and-worker-safety/PHASE-RESEARCH.md`
  - `.planning/phases/03-concurrency-and-worker-safety/PHASE-SUMMARY.md`
  - `.planning/phases/03-concurrency-and-worker-safety/PHASE-UAT.md`
- Phase 03 scope and acceptance criteria are locked:
  - `WORKER_CONCURRENCY` runtime setting (`>0`, default `4`)
  - bounded in-process worker pool runtime model
  - graceful shutdown drain behavior target
  - explicit contention/race test coverage expectations
- Phase 03 config slice implemented:
  - `WORKER_CONCURRENCY` added to worker runtime config load/validation
  - default `4`, env override support, fail-fast on non-positive values
  - worker entrypoint now wires concurrency value into runtime logging context
- Phase 03 worker runtime slice implemented:
  - `internal/worker` `Run` now uses bounded in-process pool concurrency
  - retry dispatcher startup behavior preserved from `Run`
  - worker tests now cover bounded concurrency ceiling and dispatcher startup from `Run`
- Phase 03 worker shutdown-drain slice implemented:
  - cancellation stops dequeue acceptance and closes the internal work channel
  - in-flight jobs receive a drain context that is not canceled immediately by shutdown
  - configured shutdown timeout explicitly cancels in-flight work that has not drained and stops waiting
  - worker entrypoint wires both `WORKER_CONCURRENCY` and `WORKER_SHUTDOWN_TIMEOUT` into runtime behavior
- Phase 03 worker logging traceability slice implemented:
  - concurrent worker-pool handlers attach stable `worker_slot` context to per-job logs
  - guarded transition logs include `job_id`, transition name, applied flag, and outcome
- Phase 03 worker contention coverage slice implemented:
  - duplicate deliveries of the same `job_id` race for a processing claim with only one processing/completion path
  - active processing count is asserted not to exceed configured worker concurrency
  - cancellation is asserted to stop intake while allowing the in-flight job to complete
- Phase 03 repository contention coverage slice implemented:
  - concurrent `MarkProcessing` calls on the same job yield exactly one successful guarded transition
  - concurrent terminal transition attempts apply at most once
  - concurrent `ClaimDueRetries` calls do not return duplicate job IDs across callers
- Phase 03 documentation and UAT evidence captured:
  - phase summary reflects implemented worker concurrency, shutdown, logging, and contention behavior
  - UAT records passing command validation from 2026-05-12
  - README documents worker concurrency defaults and shutdown behavior

## Next milestone
begin Phase 04 planning for visibility timeout and stuck `processing` recovery

## Risks / open questions
- repository contention tests use explicit start barriers and persisted-state assertions to avoid flaky timing-only checks
- visibility-timeout recovery remains deferred, so crashes mid-processing are still handled in Phase 04
