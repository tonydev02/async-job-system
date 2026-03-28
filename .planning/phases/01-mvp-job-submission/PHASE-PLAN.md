# PHASE-PLAN.md

## Phase
01 — MVP Job Submission and Background Processing

## Goal
Build the smallest end-to-end async job workflow that is still production-shaped.

## Why this phase matters
This phase proves the core architecture:
API → DB → Redis → Worker → DB → Polling API

## In scope
- job creation endpoint
- job status endpoint
- job persistence in Postgres
- queueing via Redis
- worker consumption
- status transitions
- basic error handling
- local development with Docker Compose

## Out of scope
- retries
- visibility timeout
- dead-letter queue
- multiple job types
- metrics dashboard

## Deliverables
- working local API service
- working local worker service
- schema migration for jobs table
- documented API contract
- demo flow verified end-to-end

## Acceptance criteria
- user can submit a job and receive a job ID
- submitted job is stored as `pending`
- worker eventually changes job to `processing`
- worker eventually changes job to `completed` or `failed`
- `GET /jobs/{id}` reflects the persisted state
- logs include job ID during processing

## Implementation notes
- Postgres is source of truth
- Redis only carries job references
- payload/result stored as JSONB
- worker uses a fake deterministic processor for MVP
