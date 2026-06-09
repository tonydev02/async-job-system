# PHASE-UAT.md

## Objective
Validate that operators can determine API and worker health, correlate requests with jobs, inspect lifecycle behavior through metrics, and diagnose the major failure paths from Phases 01-04.

## Test cases

### 1. Structured logger contract
- [ ] verify API and worker production commands emit valid JSON logs
- [ ] verify base logs include `service`
- [ ] verify subsystem logs include `component`
- [ ] verify errors use a consistent `error` field
- [ ] verify durations use the documented representation
- [ ] verify credentials, connection strings, and payload contents are not logged

### 2. API request correlation
- [ ] send a request without `X-Request-ID` and verify a generated ID appears in the response and logs
- [ ] send a valid caller-provided `X-Request-ID` and verify it is preserved
- [ ] send an invalid or oversized request ID and verify it is replaced
- [ ] verify job creation logs contain both `request_id` and `job_id`
- [ ] verify job lookup logs contain both values after ID parsing

### 3. API access logging and panic recovery
- [ ] verify access logs include method, route template, status, response bytes, and duration
- [ ] verify `/jobs/{id}` uses a bounded route value rather than the raw UUID path
- [ ] verify validation failures and dependency failures use appropriate levels/outcomes
- [ ] force a handler panic and verify a correlated `500` response and error log
- [ ] verify health/metrics access logs follow the documented low-noise policy

### 4. API liveness and readiness
- [ ] verify `GET /livez` returns `200` without checking dependencies
- [ ] verify `GET /readyz` returns `200` when Postgres and Redis checks pass
- [ ] verify `GET /readyz` returns `503` when Postgres fails
- [ ] verify `GET /readyz` returns `503` when Redis fails
- [ ] verify readiness checks obey a short timeout
- [ ] verify responses expose stable component states without raw internal errors

### 5. API metrics
- [ ] verify `GET /metrics` returns Prometheus-compatible text
- [ ] verify request counters distinguish method, route, and bounded status outcome
- [ ] verify request duration observations are recorded
- [ ] verify submission outcomes cover accepted, validation failure, persistence failure, enqueue failure, and reschedule failure
- [ ] verify repeated test/server construction does not panic from duplicate registration
- [ ] verify `job_id`, `request_id`, raw paths, payloads, and arbitrary errors are absent from labels

### 6. Production API runtime
- [ ] verify API config defaults and overrides
- [ ] verify missing required config and malformed/non-positive values fail fast
- [ ] verify startup dependency failures prevent the server from becoming ready
- [ ] verify configured HTTP timeouts are applied
- [ ] verify signal cancellation performs graceful shutdown within timeout
- [ ] verify the documented API command uses environment config rather than hardcoded connections

### 7. Worker lifecycle metrics
- [ ] verify dequeue metrics distinguish message, empty/timeout, and error outcomes
- [ ] verify processing outcomes include completed, retry scheduled, terminal failed, claim skipped, and persistence error
- [ ] verify processing duration is observed only after a successful processing claim
- [ ] verify in-flight gauge increments and returns to zero on success
- [ ] verify in-flight gauge returns to zero on processor error and cancellation
- [ ] verify transition counters use bounded transition/outcome labels
- [ ] verify retry dispatch metrics cover claim, enqueue, reschedule, and failure outcomes
- [ ] verify recovery metrics cover retry, terminal, and scanner-error outcomes
- [ ] verify no worker metric contains a job ID label

### 8. Worker structured logs
- [ ] verify worker startup logs include service and safe runtime configuration
- [ ] verify job logs include `job_id`, `worker_slot`, transition, and outcome where applicable
- [ ] verify retry and recovery logs use the shared event vocabulary
- [ ] verify queue-empty polling does not create info/error log spam
- [ ] verify shutdown logs distinguish drained, timed out, and server error outcomes

### 9. Worker operations server
- [ ] verify worker `/livez` responds while the operations server is running
- [ ] verify worker `/readyz` is false during startup
- [ ] verify worker `/readyz` becomes true after dependency and runtime initialization
- [ ] verify worker `/readyz` becomes false when shutdown begins
- [ ] verify dependency check failures produce `503`
- [ ] verify worker `/metrics` exposes the worker registry
- [ ] verify listener/server errors reach the process supervisor
- [ ] verify operations HTTP shutdown completes within the worker shutdown budget

### 10. Operational debugging workflows
- [ ] create and complete a job, then correlate API and worker logs by `job_id`
- [ ] trigger duplicate delivery and identify the skipped claim in logs and metrics
- [ ] trigger processor failure with attempts remaining and identify retry scheduling
- [ ] trigger terminal failure and identify the terminal transition
- [ ] trigger retry enqueue failure and identify reschedule behavior
- [ ] arrange stale `processing` recovery and identify retry/terminal decision
- [ ] saturate configured worker concurrency and observe the in-flight gauge
- [ ] verify README commands and example queries match actual output

## Automated evidence to capture
- [ ] logger JSON/field tests
- [ ] request-ID middleware tests
- [ ] access-log status/route/duration tests
- [ ] panic recovery test
- [ ] API health/readiness dependency tests
- [ ] API metric family/value/cardinality tests
- [ ] API config and graceful shutdown tests
- [ ] worker processing metric outcome tests
- [ ] worker in-flight gauge cleanup tests
- [ ] retry dispatcher and recovery metric tests
- [ ] worker operations readiness lifecycle tests
- [ ] worker operations server cancellation/error tests

## Command validation
- [ ] `gofmt -w` on changed Go files
- [ ] targeted config, observability, HTTP API, worker, and command package tests
- [ ] `go test ./...`
- [ ] `go vet ./...`

## Manual verification
- [ ] run API and worker together with separate configured addresses
- [ ] inspect representative API and worker JSON log lines
- [ ] `curl` API `/livez`, `/readyz`, and `/metrics`
- [ ] `curl` worker `/livez`, `/readyz`, and `/metrics`
- [ ] stop Postgres and verify readiness behavior without liveness restart behavior
- [ ] stop Redis and verify readiness plus enqueue/dequeue observability
- [ ] send `SIGTERM` and verify both processes stop accepting work and shut down within configured timeouts
- [ ] review metric labels for bounded cardinality
- [ ] review README operational runbook against observed behavior
