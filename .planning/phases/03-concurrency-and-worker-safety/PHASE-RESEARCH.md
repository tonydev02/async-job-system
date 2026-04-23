# PHASE-RESEARCH.md

## Questions
1. How should worker concurrency be introduced without hiding lifecycle transitions?
2. How should shutdown coordinate dequeue stop vs in-flight job drain?
3. Which race scenarios must be covered to prove correctness under contention?

## Decisions
### Concurrency model
Use a bounded in-process worker pool controlled by `WORKER_CONCURRENCY`.
Reason:
- keeps behavior explicit and reviewable
- avoids unbounded goroutine growth
- aligns with production-style runtime controls

### Transition safety model
Keep transition authority in Postgres using guarded updates and affected-row checks.
Reason:
- preserves source-of-truth rule
- naturally handles duplicate delivery and worker races
- keeps correctness anchored in DB state machine transitions

### Shutdown behavior
On shutdown signal, stop dequeueing new messages and wait for in-flight handlers until shutdown timeout.
Reason:
- avoids starting new work while draining
- minimizes abandoned in-flight jobs
- keeps runtime behavior deterministic and testable

### Logging contract under concurrency
Include `job_id` and worker context (`worker_instance`, transition outcome) in concurrent paths.
Reason:
- improves debuggability for duplicate delivery and race outcomes
- preserves operational clarity as concurrency increases

## Test strategy decisions
- add worker runtime tests for bounded concurrency, duplicate message contention, and shutdown drain behavior
- add repository contention tests for concurrent guarded transitions:
  - `MarkProcessing` single-winner semantics
  - terminal transition single-application semantics
  - `ClaimDueRetries` no duplicate claim across concurrent callers

## Deferred decisions
- visibility timeout and stale `processing` recovery (Phase 04)
- dead-letter queue behavior
- backoff policy evolution (exponential/jitter)
