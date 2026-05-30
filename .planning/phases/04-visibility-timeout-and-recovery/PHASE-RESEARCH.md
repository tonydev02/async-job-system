# PHASE-RESEARCH.md

## Questions
1. Where should stale `processing` recovery run?
2. What transition should recovery apply after the visibility timeout expires?
3. How should recovery remain safe with multiple worker processes?

## Decisions

### Recovery runtime
Run stale `processing` recovery inside the worker process as a periodic scanner.

Reason:
- follows the existing worker-owned retry dispatcher pattern
- avoids introducing a second service for the MVP
- keeps operational behavior explicit in the worker runtime
- lets every worker process contribute to recovery safely when backed by DB locking

### Recovery transition semantics
Use retry-or-fail semantics for stale `processing` jobs.

Behavior:
- if `attempt < max_attempts`, transition `processing -> pending`
- set `next_run_at` to `now + recovery_retry_delay`
- record a recovery-oriented error message
- if `attempt >= max_attempts`, transition `processing -> failed`
- set `completed_at` for terminal failure

Reason:
- preserves bounded retry guarantees
- prevents infinite timeout recovery loops
- keeps stuck-job recovery aligned with normal processor failure handling

### Attempt counting
Do not increment `attempt` during recovery.

Reason:
- attempts are already incremented by `pending -> processing`
- a stale processing timeout represents the outcome of the current attempt
- the next attempt should be counted only when the job is claimed again

### Queue interaction
Recovery should not enqueue jobs directly.

Recovered retryable jobs become `pending` with `next_run_at`, and the existing retry dispatcher later claims and enqueues them.

Reason:
- Postgres remains the source of truth
- recovery and transport dispatch stay separated
- enqueue failures continue to use existing retry reschedule behavior

### Concurrency safety
Use Postgres row locking for recovery claims.

Implementation direction:
- select stale `processing` rows ordered by `started_at`
- use `FOR UPDATE SKIP LOCKED`
- update only selected rows in one repository operation
- return recovered job IDs and decisions for logging

Reason:
- supports multiple worker processes scanning at the same time
- prevents duplicate recovery decisions for the same job
- matches the existing due-retry claim pattern

## Test strategy decisions
- repository tests should assert persisted state after recovery, not just returned results
- concurrent recovery tests should run multiple callers and assert no duplicate recovered IDs
- worker tests should use fake repository hooks to prove startup scan and interval behavior
- config tests should cover both defaults and invalid runtime values

## Deferred decisions
- dead-letter queue or DLQ table
- operator-triggered manual recovery endpoint/command
- exponential backoff and jitter
- metrics export beyond structured, metrics-ready log fields
