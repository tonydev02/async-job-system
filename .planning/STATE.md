# STATE.md

## Project
Async Job Processing System

## Current phase
02 — Retries and Failure Handling

## Current status
in progress; repository-layer retry foundation, worker failure-transition migration, and worker retry dispatcher loop are implemented; remaining work is config wiring + full phase validation/UAT evidence

## Objective
Implement bounded retries and explicit terminal failure handling while preserving Postgres as the source of truth for lifecycle transitions.

## Non-goals for current phase
- dead-letter flow
- visibility timeout
- exponential backoff/jitter
- multiple job types

## Done
- Phase 01 remains complete and validated (API -> Postgres -> Redis -> worker baseline)
- Phase 02 planning artifacts are created (`PHASE-PLAN`, `PHASE-RESEARCH`, `PHASE-SUMMARY`, `PHASE-UAT`)
- repository contract extended with retry-oriented methods:
  - `HandleProcessingFailure`
  - `ClaimDueRetries`
  - `RescheduleRetry`
- Postgres repository implementation added for retry transition, due-retry claiming, and rescheduling paths
- retry decision enum now explicit (`retry`, `terminal_failed`)
- test doubles updated to satisfy new repository interface in worker and HTTP handler tests
- worker processing failure path now uses `HandleProcessingFailure` (replacing `MarkFailed`) with retry/terminal decision logging
- worker retry dispatcher loop added:
  - immediate due-retry dispatch on worker start
  - periodic due-retry dispatch ticker
  - DB claim (`ClaimDueRetries`) + queue re-enqueue path
  - enqueue-failure safety via `RescheduleRetry`
- worker dispatcher behavior tests added for:
  - claim-and-enqueue flow
  - enqueue-failure reschedule safety
  - claim error handling
  - immediate dispatch on startup
- current local validation: `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi` passes

## Next milestone
implement remaining Phase 02 runtime work:
- add worker runtime config wiring for retry timings/dispatcher batch size
- complete phase-02 UAT evidence and full validation commands (`go test ./...`, `go vet ./...`)

## Risks / open questions
- visibility-timeout and stuck-`processing` recovery remain deferred to Phase 04
- current due-retry SQL path should be backed by explicit integration tests for concurrent claim behavior
