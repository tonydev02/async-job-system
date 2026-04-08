# STATE.md

## Project
Async Job Processing System

## Current phase
02 — Retries and Failure Handling

## Current status
in progress; repository-layer retry foundation is implemented and compiling, worker/runtime integration is pending

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
- current local validation: `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi` passes

## Next milestone
implement remaining Phase 02 runtime work:
- worker uses `HandleProcessingFailure` instead of `MarkFailed`
- add retry dispatcher loop and config wiring
- add targeted phase-02 behavior tests + UAT evidence

## Risks / open questions
- visibility-timeout and stuck-`processing` recovery remain deferred to Phase 04
- current due-retry SQL path should be backed by explicit integration tests for concurrent claim behavior
