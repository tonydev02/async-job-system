# STATE.md

## Project
Async Job Processing System

## Current phase
01 — MVP Job Submission and Background Processing

## Current status
in progress

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

## Next milestone
First successful end-to-end job lifecycle in local Docker environment

## Risks / open questions
- Redis queue pattern for MVP
- duplicate processing protection approach
