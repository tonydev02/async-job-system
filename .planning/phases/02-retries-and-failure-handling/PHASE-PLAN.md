# PHASE-PLAN.md

## Phase
02 — Retries and Failure Handling

## Goal
Add bounded retry behavior with explicit terminal failure while keeping Postgres as the source of truth for lifecycle transitions.

## Why this phase matters
This phase turns the MVP into a reliability-focused system that can survive transient processing failures without losing lifecycle correctness.

## In scope
- fixed-delay retry policy
- bounded attempts (`attempt` + `max_attempts`)
- atomic failure transition (`processing -> pending` for retry, `processing -> failed` for terminal)
- Postgres-driven due-retry claim + Redis re-dispatch
- retry re-schedule on dispatcher enqueue failure
- worker retry decision logging with `job_id`
- worker runtime retry configuration

## Out of scope
- visibility-timeout crash recovery
- exponential backoff/jitter
- dead-letter queue
- public API expansion for retry controls

## Deliverables
- repository methods for retry transition, due-retry claiming, and re-scheduling
- worker retry transition behavior and retry dispatcher loop
- config support for retry timings and dispatcher batch size
- updated tests across repository, worker, and API behavior visibility

## Acceptance criteria
- processor failure before max attempts schedules retry (`status=pending`, `next_run_at` set)
- processor failure at max attempts becomes terminal (`status=failed`, `completed_at` set)
- due retry jobs are claimed from Postgres and re-enqueued to Redis
- enqueue failure during retry dispatch re-schedules `next_run_at`
- `GET /jobs/{id}` continues exposing retry metadata (`attempt`, `max_attempts`, `next_run_at`, `error`)
- tests for updated behavior pass locally
