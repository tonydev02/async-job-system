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

## Next milestone
First successful end-to-end job lifecycle in local Docker environment

## Risks / open questions
- Redis queue pattern for MVP
- job status transition rules
- duplicate processing protection approach