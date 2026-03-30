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
- repository initialized
- planning structure created
- jobs table migration added (`up`/`down`)
- job domain model and status constants implemented
- Postgres jobs repository implemented (`Create`, `GetByID`, guarded transitions)
- migration smoke test and repository integration tests added
- HTTP jobs handlers implemented for `POST /jobs` and `GET /jobs/{id}`
- basic router wiring added for jobs endpoints with method guards
- HTTP handler tests added for `POST /jobs` success and `GET /jobs/{id}` not-found behavior
- queue contract added (`Queue` interface with `Enqueue`/`Dequeue` and `Message{job_id}`)
- Redis queue adapter added with blocking dequeue and UUID parsing

## Next milestone
First successful end-to-end job lifecycle in local Docker environment

## Risks / open questions
- Redis queue pattern for MVP
- duplicate processing protection approach
