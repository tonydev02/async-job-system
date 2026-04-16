# STATE.md

## Project
Async Job Processing System

## Current phase
02 — Retries and Failure Handling

## Current status
Phase 02 complete; bounded retries, terminal failure transitions, retry dispatch loop, enqueue-failure reschedule safety, and retry runtime config wiring are implemented and validated by automated coverage.

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
- worker runtime retry config wiring added and validated:
  - `RETRY_DELAY`
  - `RETRY_DISPATCH_INTERVAL`
  - `RETRY_DISPATCH_BATCH_SIZE`
  - `RETRY_REENQUEUE_DELAY`
- phase-closure reliability hardening completed:
  - API enqueue failure path now re-schedules retry to avoid stranded pending jobs
  - retry scheduling SQL preserves sub-second delay precision
  - retry env config values are validated as positive during config load
- phase-closure acceptance tests added:
  - retry-before-max transition coverage
  - terminal-at-max transition coverage
  - due-retry claim clears `next_run_at` coverage
  - API retry metadata visibility coverage
- current local validation: `go test ./internal/jobs/postgres ./internal/worker ./internal/httpapi` passes
- full validation: `go test ./...` and `go vet ./...` pass
- manual end-to-end verification completed on 2026-04-16 (JST) with Docker `ajs-postgres` + `ajs-redis`; retry scheduling, dispatcher redispatch logs, and terminal failure behavior were captured

## Next milestone
start Phase 03 planning and scope definition

## Risks / open questions
- visibility-timeout and stuck-`processing` recovery remain deferred to Phase 04
- current due-retry SQL path should be backed by explicit integration tests for concurrent claim behavior
