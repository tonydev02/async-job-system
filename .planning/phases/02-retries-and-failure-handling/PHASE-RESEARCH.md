# PHASE-RESEARCH.md

## Questions
1. Where should retry scheduling authority live?
2. How should failure transition decide retry vs terminal failure?
3. How do we avoid losing due retries when enqueue fails?

## Decisions
### Retry authority
Use Postgres-driven scheduling (`next_run_at`) and dispatch due retries from DB.
Reason:
- aligns with "Postgres is source of truth"
- resilient to worker restarts
- deterministic lifecycle visibility in one store

### Failure transition model
Use one atomic repository transition from `processing`:
- if `attempt < max_attempts`: set `status=pending`, set `next_run_at=now()+retry_delay`, clear terminal fields
- else: set `status=failed`, set `completed_at`, clear `next_run_at`

Reason:
- single-write decision avoids split-brain lifecycle updates
- explicit status transition remains easy to reason about in interviews

### Due retry dispatch
Claim due rows with `FOR UPDATE SKIP LOCKED`, clear `next_run_at` on claim, enqueue each job ID.
On enqueue failure, re-schedule in Postgres with a short reenqueue delay.
Run this via a worker background ticker loop so dispatch happens even when no new queue messages arrive.
Keep dispatcher behavior explicit in worker runtime (claim -> enqueue -> reschedule-on-failure) for traceable ops logs.

Reason:
- multi-worker safe claiming
- avoids duplicated re-dispatch under concurrent dispatchers
- preserves recoverability on transport failure

## Deferred decisions
- visibility timeout and stale `processing` recovery (Phase 04)
- dead-letter policy (later phase)
- exponential backoff/jitter policy (future enhancement)
