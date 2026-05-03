# STATE.md

## Project
Async Job Processing System

## Current phase
03 — Concurrency and Worker Safety

## Current status
Phase 03 implementation in progress; bounded worker-pool runtime slice is complete, shutdown-drain and repository contention slices remain.

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

## Next milestone
implement Phase 03 runtime and test changes in small reviewable steps

## Risks / open questions
- contention tests require careful deterministic orchestration to avoid flaky timing-based assertions
- visibility-timeout recovery remains deferred, so crashes mid-processing are still handled in Phase 04
