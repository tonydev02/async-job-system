# Roadmap

## Current Snapshot
- Current phase: `03-concurrency-and-worker-safety`
- Status: in progress (planning finalized; implementation pending)
- Next phase: `04-visibility-timeout-and-recovery`

## Phase Map
1. `01-mvp-job-submission`
Goal: prove end-to-end async flow works (API -> Postgres -> Redis -> worker -> Postgres -> polling API).
Status: done.

2. `02-retries-and-failure-handling`
Goal: implement retry policy, bounded attempts, and explicit terminal failure behavior.
Status: done.

3. `03-concurrency-and-worker-safety`
Goal: harden duplicate delivery handling and multi-worker race safety.
Status: in progress.

4. `04-visibility-timeout-and-recovery`
Goal: recover jobs stuck in `processing` after crashes/timeouts.
Status: planned.

5. `05-observability-and-ops`
Goal: improve structured logs, metrics-ready signals, and operational debugging workflows.
Status: planned.

6. `06-dashboard-or-admin-api`
Goal: add operator controls/inspection only after backend lifecycle reliability is solid.
Status: planned.

## Execution Rule Per Phase
For every active phase, maintain:
- `PHASE-PLAN.md`
- `PHASE-RESEARCH.md`
- `PHASE-SUMMARY.md`
- `PHASE-UAT.md`

And follow sequence:
1. confirm scope in plan
2. capture design decisions in research
3. implement in small steps
4. update docs
5. summarize outcomes
6. close with UAT evidence
