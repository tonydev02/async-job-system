# STATE.md

## Project
Async Job Processing System

## Current phase
01 — MVP Job Submission and Background Processing

## Current status
implementation complete; manual end-to-end local UAT captured (success + failure + signal shutdown verified)

## Objective
Deliver a minimal end-to-end async workflow:
- submit job via API
- persist job in Postgres
- enqueue job ID in Redis
- worker processes job
- status can be polled via API

## Non-goals for current phase
- retries
- dead-letter flow
- visibility timeout
- multiple job types
- dashboard UI

## Done
- foundation: repository initialized and planning structure established
- schema/domain: `jobs` migration (`up`/`down`) and job status model added
- persistence: Postgres repository implemented with guarded transitions (`Create`, `GetByID`, `MarkProcessing`, `MarkCompleted`, `MarkFailed`)
- persistence validation: migration smoke and repository integration tests added
- HTTP API: `POST /jobs` + `GET /jobs/{id}` handlers and router method guards implemented
- Step 3 completion: `POST /jobs` now persists in Postgres, then enqueues `job_id` through `queue.Queue`
- queueing: queue contract and Redis adapter added (blocking dequeue + UUID parsing)
- API validation: handler tests cover submit success, enqueue failure (`503`), and not-found mapping
- safety: constructor dependency guards added for HTTP handler and Postgres repository
- Step 4 completion: worker package implemented with dequeue loop, claim-then-process flow, and explicit terminal transitions
- worker validation: unit tests added for duplicate-safe skip, success completion, processor failure, queue-empty cancellation, and processor context cancellation
- Step 5 completion: runnable worker entrypoint added at `cmd/worker/main.go` with service-level config loading, dependency wiring, startup connectivity checks, and signal-aware shutdown handling
- Step 5 validation: worker entrypoint builds through `go test ./...`; Redis client bootstrap moved to explicit constructor with startup ping

## Next milestone
phase close-out and next-phase planning (retries / visibility-timeout / dead-letter)

## Risks / open questions
- no critical open items for phase 01 UAT; next major risk moves to retry/lease semantics in later phases
