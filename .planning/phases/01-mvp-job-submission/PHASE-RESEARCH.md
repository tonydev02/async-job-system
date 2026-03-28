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
