# AGENTS.md

## Project mission
Build a production-style async job processing system in Go.

This project exists to demonstrate backend engineering skill in:
- asynchronous workflows
- queue-based architecture
- worker reliability
- idempotent job execution
- retry and failure handling
- observability and operational thinking

The goal is not just to make background jobs work.
The goal is to show production-style backend design decisions clearly.

---

## Working style
- Prefer small, reviewable changes.
- Before implementing non-trivial work, update the relevant files in `.planning/`.
- Keep docs, plans, and code aligned.
- Do not introduce abstractions before they are justified by real use.
- Favor explicit, readable Go over clever patterns.
- Preserve a working build at all times.

---

## Planning rules
- `.planning/STATE.md` is the current source of truth for overall project status.
- Each phase must have:
  - `PHASE-PLAN.md`
  - `PHASE-RESEARCH.md`
  - `PHASE-SUMMARY.md`
  - `PHASE-UAT.md`
- Before coding:
  1. confirm scope in `PHASE-PLAN.md`
  2. write design notes in `PHASE-RESEARCH.md` if needed
  3. implement in small steps
  4. update docs
  5. update `PHASE-SUMMARY.md`
  6. verify with `PHASE-UAT.md`

---

## Backend architecture rules
- API service and worker service must be separately runnable.
- PostgreSQL is the source of truth for job state.
- Redis is a transport/buffering mechanism, not the final source of truth.
- Every job lifecycle transition must be persisted in the database.
- Avoid hidden magic between layers.
- Keep job state transitions explicit and easy to trace.

---

## Reliability rules
- Design for duplicate delivery and retries.
- Treat idempotency as a first-class concern.
- Never assume a worker completes successfully once it dequeues a job.
- Handle crash, retry, and timeout cases deliberately.
- Prefer state-machine style status transitions over ad hoc flags.

---

## Observability rules
- Use structured logs.
- Include `job_id` in all job-related logs.
- Include request correlation where applicable.
- Keep logs useful for debugging worker crashes, duplicate processing, and stuck jobs.
- Add metrics-ready structure even if full metrics are not implemented yet.

---

## Go engineering rules
- Use the standard library first where reasonable.
- Keep packages small and cohesive.
- Prefer constructor-based dependency injection.
- Pass `context.Context` explicitly.
- Keep interfaces close to the consumer when useful.
- Write code that is easy to test without heavy mocking.

---

## API rules
- Keep HTTP handlers thin.
- Validate input at the boundary.
- Return stable response shapes.
- Do not leak internal queue details through the API.
- Model job states consistently across API, service, and database.

---

## Database rules
- Use migrations for schema changes.
- Keep schema simple and explicit.
- Job records must include status, payload, result, error, retry metadata, and timestamps.
- Avoid premature normalization unless it clearly improves correctness or operations.

---

## Validation rules
Before completing a task, always:
- run formatting
- run lint/vet if configured
- run tests relevant to the changed code
- confirm docs/plans are updated if behavior changed

If something cannot be validated locally, state that clearly.

---

## Anti-goals
- Do not build a giant framework.
- Do not add Kafka, Kubernetes, or multiple queues in the MVP.
- Do not optimize for scale before reliability and clarity are demonstrated.
- Do not add frontend/admin UI before the backend lifecycle is solid.