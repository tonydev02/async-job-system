# PHASE-RESEARCH.md

## Questions
1. Which operational signals are missing from the current API and worker?
2. How should logs and metrics remain useful without introducing high-cardinality or sensitive data?
3. What should liveness and readiness mean for each process?
4. Should Phase 05 add tracing, dashboards, and alerting infrastructure?
5. How should observability code remain testable without hiding lifecycle behavior?

## Current-state findings
- the worker uses `log/slog`, but production output is text and logger construction lives in `cmd/worker`
- worker logs often include `job_id`, but event fields and outcomes are not yet governed by a shared contract
- the API UAT command uses the standard `log` package, hardcoded connection settings, and `http.ListenAndServe`
- API handlers do not receive a logger and do not emit request or job lifecycle logs
- there is no request correlation middleware
- there are no liveness, readiness, or metrics endpoints
- there is no production-style API configuration or graceful API shutdown
- worker retry and recovery loops have useful logs but no counters, durations, or in-flight signal
- no observability library is currently present in `go.mod`

## Decisions

### Structured logging
Use `log/slog` JSON handlers for production API and worker commands.

Stable base fields:
- `service`: `api` or `worker`
- `component`: bounded subsystem name
- `request_id`: API request correlation value
- `job_id`: job identity in logs only
- `transition`, `decision`, `outcome`: bounded lifecycle values
- `duration_ms` or another single documented duration representation
- `error`: error value/string

Reason:
- keeps the implementation standard-library-first
- produces machine-readable output without a logging framework
- supports child loggers for service, component, request, worker slot, and job context

### Request correlation
Use `X-Request-ID` as the external correlation header.

Behavior:
- retain a syntactically valid incoming value within a conservative length bound
- otherwise generate a UUID
- always return the selected value in the response
- place the value and a request-scoped logger in context

Reason:
- enables correlation across access and handler logs
- avoids coupling to a tracing backend
- keeps the API contract simple and conventional

### Access-log route labeling
Log and measure route templates, not raw paths.

Examples:
- `/jobs`
- `/jobs/{id}`
- `/livez`
- `/readyz`
- `/metrics`
- `unmatched`

Reason:
- raw `/jobs/<uuid>` values create unbounded metric cardinality
- route templates make aggregate latency and error rates useful

### Metrics format
Expose Prometheus-compatible text metrics from each process.

Implementation direction:
- prefer a process-local registry injected into API and worker instrumentation
- avoid default global registries in tests
- use counters for event totals, histograms for latency, and gauges only for current state
- add a Prometheus client dependency if hand-rolled exposition would compromise histogram correctness or registry safety

Reason:
- Prometheus exposition is broadly understood and easy to inspect with `curl`
- a local registry keeps tests deterministic
- instrumentation can be useful before a monitoring stack is deployed

### Metric cardinality
Never use these as labels:
- `job_id`
- `request_id`
- payload values
- raw URL paths
- arbitrary error messages
- worker-slot numbers unless a concrete operational need is proven

Use bounded labels:
- method
- route template
- status class or bounded status code
- transition
- decision
- outcome
- service

Reason:
- identifiers and arbitrary strings can create one time series per event
- logs remain the correct place for individual job/request investigation

### API liveness
`/livez` is process-only and performs no dependency checks.

Reason:
- liveness should answer whether the process and HTTP server are running
- dependency outages should not cause restart loops

### API readiness
`/readyz` checks Postgres and Redis with a short timeout.

Reason:
- job submission requires both persistence and queue transport
- job reads require Postgres
- returning not-ready prevents routing new work to an API that cannot honor its contract

The response should use stable component status values and omit raw dependency errors.

### Worker liveness and readiness
The worker exposes a small operations HTTP server.

Liveness:
- succeeds while the process is serving the operations endpoint

Readiness:
- false during startup
- true after Postgres, Redis, repository, queue, processor, and worker runtime initialization
- false as soon as shutdown begins
- may include bounded dependency checks if they can complete within a short timeout

Reason:
- the worker has no business HTTP server but still needs orchestration and scraping endpoints
- readiness must reflect whether the worker should receive/continue operational traffic, not whether a single job succeeded

### API runtime
Add a production-style `cmd/api` command instead of expanding the hardcoded `cmd/api-uat` bootstrap.

Reason:
- API and worker should both be separately runnable production-style services
- environment config and graceful shutdown are prerequisites for meaningful health and metrics behavior
- retaining the UAT command as the documented API entrypoint would leave operational behavior ambiguous

### Instrumentation ownership
Record metrics at the layer that knows the semantic outcome.

Examples:
- HTTP middleware records method/route/status/duration
- job handler records validation, persistence, enqueue, and acceptance outcomes
- worker records claim, processing, transition, retry-dispatch, and recovery outcomes
- health handlers record health response behavior only if a concrete need emerges

Reason:
- generic wrappers cannot infer lifecycle decisions reliably
- instrumentation remains close to explicit state transitions
- repository correctness stays independent of metrics availability

### Failure behavior
Logging and metrics must be best-effort and must not change job outcomes.

Reason:
- observability is a reporting concern
- a metric registration or emission problem must be caught during construction/tests, not become a runtime job failure

### Scope boundary
Do not add OpenTelemetry, a Prometheus server deployment, Grafana, or alerting rules in Phase 05.

Reason:
- the codebase first needs stable local signals and operational contracts
- backend deployment choices are not yet established
- dashboards and alerts should be designed from proven metric names and actual operating behavior

## Proposed event vocabulary
Log events should use clear messages plus bounded fields rather than encoding all meaning in prose.

Core transitions:
- `pending_to_processing`
- `processing_to_completed`
- `processing_to_pending`
- `processing_to_failed`

Core outcomes:
- `applied`
- `skipped`
- `success`
- `error`
- `empty`
- `timeout`
- `retry_scheduled`
- `terminal_failed`
- `rescheduled`

The final vocabulary should be centralized or covered by tests so API and worker do not drift.

## Proposed test strategy
- logger tests decode JSON lines and assert fields rather than matching text formatting
- middleware tests use a response recorder and controlled handler to assert request ID, route, status, bytes, duration presence, and panic recovery
- metrics tests gather from an isolated registry and assert family names, labels, and values
- cardinality tests assert identifiers do not appear as label names or label values
- health tests use fake dependency checkers and bounded contexts
- command tests cover config/wiring through testable `run` functions rather than invoking `main`
- worker metric tests assert semantic outcomes and in-flight gauge cleanup on success, error, skip, and cancellation
- server lifecycle tests use ephemeral listeners to avoid fixed-port contention

## Risks and mitigations

### Risk: instrumentation makes worker code noisy
Mitigation:
- use a narrow worker metrics struct with explicit methods or collectors
- keep lifecycle decisions in existing control flow
- avoid a generic event bus

### Risk: metric cardinality grows accidentally
Mitigation:
- centralize label sets
- test gathered labels
- document forbidden labels

### Risk: readiness checks overload dependencies
Mitigation:
- use short timeouts
- keep checks lightweight
- avoid performing readiness checks inside job processing paths

### Risk: health endpoint logs create noise
Mitigation:
- use a specific access-log policy for health and metrics routes
- retain error logs when a readiness dependency fails

### Risk: API runtime work expands beyond observability
Mitigation:
- preserve existing job API behavior
- limit runtime work to config, dependency startup, HTTP timeouts, logging, and graceful shutdown

## Deferred decisions
- OpenTelemetry traces and trace propagation
- exemplars linking metrics to traces
- SLO targets and alert thresholds
- dashboard layout and deployment manifests
- profiling endpoints
- runtime log-level mutation
- admin/operator API
