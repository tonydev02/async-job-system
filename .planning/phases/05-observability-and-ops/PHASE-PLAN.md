# PHASE-PLAN.md

## Phase
05 - Observability and Ops

## Goal
Make the API and worker operationally inspectable through consistent structured logs, bounded-cardinality metrics, health/readiness endpoints, and documented debugging workflows.

## Why this phase matters
Phases 01-04 established a DB-backed lifecycle that remains correct through retries, duplicate delivery, concurrency, shutdown, and stale-processing recovery. The next production gap is knowing what the system is doing without reading database rows or reconstructing behavior from inconsistent text logs.

Phase 05 adds observability at the existing lifecycle boundaries. It does not change job-state semantics: logs and metrics report decisions already made by the API, worker, queue, and repository layers.

## In scope
- shared structured JSON logger construction for API and worker
- stable service-level log fields:
  - `service`
  - `component`
  - `request_id` where applicable
  - `job_id` for job-related events
  - transition, outcome, attempt, and duration fields where applicable
- API request correlation and access logging
- Prometheus-compatible metrics for API traffic and worker lifecycle behavior
- bounded metric labels with explicit cardinality rules
- API liveness/readiness endpoints
- worker operations HTTP server with liveness/readiness and metrics endpoints
- production-style API command/configuration and graceful HTTP shutdown
- tests for logs, metrics, middleware, health checks, configuration, and server shutdown
- README operational runbook and troubleshooting queries

## Out of scope
- distributed tracing or OpenTelemetry export
- log aggregation backend
- Prometheus server, Grafana dashboards, or alert-manager deployment
- paging thresholds or SLO policy
- per-job metric labels
- profiling endpoints
- dead-letter queue behavior
- operator mutation endpoints, manual retries, or admin UI
- changes to job lifecycle states or retry policy

## Deliverables
- reusable observability package for logger and metric registry construction
- API middleware for request IDs, panic-safe access logging, status capture, and duration measurement
- API metrics covering requests, job submissions, and enqueue failures
- worker metrics covering dequeue, processing, transitions, retry dispatch, recovery, in-flight work, and shutdown
- `/livez`, `/readyz`, and `/metrics` on the API server
- worker operations listener exposing `/livez`, `/readyz`, and `/metrics`
- environment-backed API and worker operations configuration
- graceful startup/shutdown behavior for both HTTP servers
- focused automated tests plus full `go test ./...` and `go vet ./...` validation
- Phase 05 summary, UAT evidence, README, state, and roadmap updates

## Subtasks

### 1. Establish observability contracts
Purpose: define stable event names, field names, metric names, and label bounds before instrumenting code.

Files/functions:
- `internal/observability/`
- `.planning/phases/05-observability-and-ops/PHASE-RESEARCH.md`

Done when:
- shared logger construction supports JSON output and validated log levels
- service loggers always carry `service`
- component loggers add a stable `component`
- durations use a consistent numeric unit or structured duration convention
- error values use the `error` field
- outcome values come from small documented sets
- metric names, types, help text, and allowed labels are centralized
- `job_id`, `request_id`, raw URL paths, error strings, and payload values are prohibited as metric labels

### 2. Add production API runtime configuration
Purpose: replace the hardcoded UAT bootstrap with a separately runnable, configurable API service.

Files/functions:
- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/api/main.go`
- `cmd/api/main_test.go`

Done when:
- API config loads database, Redis, queue, HTTP, shutdown, and log settings from environment
- required and malformed values fail before serving traffic
- server timeouts are explicit and positive
- the API verifies required dependencies during startup
- signal cancellation triggers graceful HTTP shutdown within the configured timeout
- `cmd/api-uat` is removed or clearly retained only as a test fixture with no README ambiguity

### 3. Add request correlation and API access logging
Purpose: trace one HTTP request across boundary validation, persistence, and enqueue behavior.

Files/functions:
- `internal/httpapi/middleware.go`
- `internal/httpapi/router.go`
- `internal/httpapi/jobs_handler.go`
- corresponding tests

Done when:
- the API accepts a valid incoming `X-Request-ID` or generates a UUID when absent/invalid
- the response includes `X-Request-ID`
- request ID is stored in context and added to request-scoped loggers
- access logs include method, route template, status, duration, and response bytes
- raw paths containing job IDs are not used as the low-cardinality route field
- job handler logs include both `request_id` and `job_id` after job creation or ID parsing
- invalid input and dependency failures are logged at appropriate levels without logging payload contents
- panic recovery logs the request context and returns a stable `500` response

### 4. Add API health and readiness endpoints
Purpose: distinguish a running process from one able to accept job traffic.

Files/functions:
- `internal/httpapi/health.go`
- `internal/httpapi/router.go`
- corresponding tests

Done when:
- `GET /livez` reports process liveness without dependency calls
- `GET /readyz` checks Postgres and Redis with a short request-scoped timeout
- readiness returns `200` only when all required dependencies pass
- failed readiness returns `503` with a stable JSON component summary
- health responses do not expose credentials or raw internal errors
- health endpoints are excluded from noisy info-level access logs or are handled through an explicit low-noise policy

### 5. Add API metrics
Purpose: quantify request load, latency, submission success, and boundary failures.

Files/functions:
- `internal/observability/metrics.go`
- `internal/httpapi/middleware.go`
- `internal/httpapi/jobs_handler.go`
- corresponding tests

Done when:
- `/metrics` serves Prometheus text exposition
- HTTP metrics include request count and duration by method, route template, and bounded status class/code policy
- job submission metrics distinguish accepted, validation failure, persistence failure, and enqueue failure
- enqueue-reschedule failures are separately observable
- metrics are registered explicitly and can use isolated registries in tests
- repeated construction in tests does not cause global registration panics

### 6. Instrument worker lifecycle metrics and logs
Purpose: expose worker throughput, failure decisions, contention, retry dispatch, recovery, and in-flight saturation.

Files/functions:
- `internal/worker/worker.go`
- `internal/worker/worker_test.go`
- `cmd/worker/main.go`
- `internal/observability/metrics.go`

Done when:
- dequeue attempts distinguish message, empty/timeout, and error outcomes without log spam
- processing metrics include started, completed, retry-scheduled, terminal-failed, claim-skipped, and persistence-error outcomes
- processing duration is observed for jobs that win the processing claim
- an in-flight gauge reflects active processor work and returns to zero
- transition logs use consistent transition and outcome fields
- retry-dispatch metrics distinguish claimed, enqueued, rescheduled, and failure outcomes
- recovery metrics distinguish retry and terminal decisions plus scanner failures
- worker startup/shutdown logs include service configuration safe to disclose and final shutdown outcome
- existing `job_id` traceability remains in logs but never appears in metrics

### 7. Add worker operations HTTP server
Purpose: make a headless worker inspectable by an orchestrator and metrics scraper.

Files/functions:
- `internal/opshttp/`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/worker/main.go`
- `cmd/worker/main_test.go`

Done when:
- a configurable worker operations address exposes `/livez`, `/readyz`, and `/metrics`
- liveness reports whether the operations process is serving
- readiness stays false until Postgres, Redis, and worker runtime initialization succeed
- readiness becomes false when shutdown begins
- operations server errors are returned to the process supervisor instead of silently ignored
- operations server shutdown shares the bounded worker shutdown lifecycle
- tests cover startup readiness, dependency failure, cancellation, and listener/server errors

### 8. Add operational documentation and close validation
Purpose: make the new signals usable during real failure scenarios.

Files/functions:
- `README.md`
- `.planning/STATE.md`
- `.planning/roadmap.md`
- `.planning/phases/05-observability-and-ops/PHASE-SUMMARY.md`
- `.planning/phases/05-observability-and-ops/PHASE-UAT.md`

Done when:
- README documents API and worker startup configuration
- README lists health and metrics endpoints
- README includes example log and metric queries for:
  - failed submissions
  - duplicate claim skips
  - jobs scheduled for retry
  - terminal failures
  - stale-processing recovery
  - retry enqueue failures
  - worker saturation
- metric label cardinality rules are documented
- summary records actual implementation choices and deviations
- UAT records automated and manual evidence
- formatting, targeted tests, `go test ./...`, and `go vet ./...` pass

## Acceptance criteria
- API and worker emit JSON structured logs with stable service/component fields
- every job-related log emitted after a job ID is known includes `job_id`
- API responses carry a request ID that is also present in request and job logs
- API and worker expose Prometheus-compatible metrics without high-cardinality labels
- liveness does not depend on Postgres or Redis
- readiness fails when required dependencies are unavailable and during shutdown
- API and worker HTTP servers shut down within configured timeouts
- operational docs explain how to diagnose the major failure paths implemented in Phases 01-04
- `gofmt`, relevant tests, `go test ./...`, and `go vet ./...` pass after implementation

## Public interfaces / contracts
- New API command: `go run ./cmd/api`
- HTTP endpoints:
  - `GET /livez`
  - `GET /readyz`
  - `GET /metrics`
- Response header:
  - `X-Request-ID`
- API runtime configuration, exact defaults finalized during implementation:
  - `API_ADDR`
  - `API_SHUTDOWN_TIMEOUT`
  - HTTP read/write/idle timeout settings
  - existing database, Redis, queue, and log settings
- Worker operations configuration:
  - `WORKER_OPS_ADDR`
  - operations HTTP timeout settings if separate values are justified
- Log format changes from text to JSON for production commands
- Existing job API response shapes and lifecycle semantics remain unchanged

## Metrics contract
Initial metric families:
- `async_jobs_http_requests_total`
- `async_jobs_http_request_duration_seconds`
- `async_jobs_submissions_total`
- `async_jobs_worker_dequeues_total`
- `async_jobs_worker_jobs_total`
- `async_jobs_worker_job_duration_seconds`
- `async_jobs_worker_in_flight`
- `async_jobs_job_transitions_total`
- `async_jobs_retry_dispatch_total`
- `async_jobs_processing_recovery_total`
- `async_jobs_worker_shutdown_total`

Allowed labels must be bounded enums such as:
- `service`
- `method`
- `route`
- `status_class` or bounded `status_code`
- `outcome`
- `transition`
- `decision`

Exact names may be refined during implementation, but any change must be reflected in research, tests, README, summary, and UAT.

## Implementation sequence
1. observability contracts and isolated metrics registry
2. production API config and runtime
3. request middleware, API logs, and API metrics
4. API health/readiness
5. worker metrics and normalized lifecycle logs
6. worker operations server and readiness lifecycle
7. operational docs, full validation, summary, and UAT

## Implementation notes
- prefer `log/slog` and standard `net/http`
- use a small Prometheus client dependency only if it materially reduces exposition and histogram correctness risk
- inject loggers, registries, clocks, and dependency checkers where tests need isolation
- keep instrumentation adjacent to the behavior it measures
- do not move lifecycle authority out of Postgres
- do not make metrics emission capable of failing job processing
- avoid logging payloads, credentials, database URLs, Redis passwords, or full connection strings
- preserve thin handlers and explicit worker control flow
