# PHASE-PLAN.md

## Phase
03 — Concurrency and Worker Safety

## Goal
Harden duplicate-delivery behavior and multi-worker race safety while preserving explicit, DB-backed lifecycle correctness.

## Why this phase matters
This phase validates that the system remains correct when several workers compete for the same jobs and when one process handles multiple jobs concurrently.

## In scope
- configurable worker concurrency inside one worker process (`WORKER_CONCURRENCY`)
- worker run-loop refactor to bounded goroutine worker-pool model
- graceful shutdown behavior for in-flight jobs (stop dequeue, allow in-flight completion up to timeout)
- explicit race-safety validation of DB lifecycle transitions under concurrent workers
- stronger structured logs for concurrent processing (`job_id`, `worker_instance`, transition result)

## Out of scope
- visibility-timeout crash recovery for stuck `processing` jobs (Phase 04)
- DLQ or backoff policy changes
- HTTP API contract changes
- distributed coordination beyond current DB-guarded transitions

## Deliverables
- worker runtime config support for `WORKER_CONCURRENCY` (`>0`, default `4`)
- concurrency-safe worker runtime implementation with bounded in-process parallelism
- graceful shutdown path that drains in-flight work within configured timeout
- new concurrency-focused tests in worker and repository layers
- Phase 03 planning/research/summary/UAT docs aligned with behavior

## Acceptance criteria
- worker process supports configurable bounded concurrency via `WORKER_CONCURRENCY`
- duplicate queue messages for the same `job_id` do not produce duplicate terminal transitions
- concurrent workers racing on the same job still apply at most one guarded state transition per stage
- worker shutdown stops dequeueing and waits for in-flight jobs up to timeout
- logs remain traceable under concurrency and include `job_id` and worker context
- local validation commands pass for changed code paths

## Public interfaces / contracts
- HTTP API: no changes
- Worker runtime config additions:
  - `WORKER_CONCURRENCY` (int, `>0`, default `4`)
- Internal lifecycle contract remains explicit and DB-guarded:
  - `pending -> processing`
  - `processing -> completed | pending(retry) | failed`

## Implementation notes
- keep Postgres as source of truth for lifecycle state
- keep Redis as transport only
- preserve existing guarded SQL transition patterns (`status` predicates + affected-row checks)
- avoid introducing framework-level abstractions; keep runtime flow explicit and testable
