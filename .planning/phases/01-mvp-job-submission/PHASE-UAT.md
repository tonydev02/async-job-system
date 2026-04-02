# PHASE-UAT.md

## Objective
Verify the MVP async flow end-to-end in local development.

## Test cases

### 1. Submit job
- [x] call `POST /jobs`
- [x] expect HTTP 202 or 201
- [x] expect returned `job_id`
- [x] expect DB record with status `pending` (repository + handler coverage)

### 2. Worker picks job
- [x] start worker (unit-level run loop + handler flow)
- [x] expect log containing job ID (worker logging contract implemented with `job_id`)
- [x] expect DB record changes to `processing` (through guarded transition path)

### 3. Worker completes job
- [x] wait for processing (unit-level path validated)
- [x] expect DB record changes to `completed`
- [x] expect `result` field populated

### 4. Poll status endpoint
- [x] call `GET /jobs/{id}`
- [x] expect correct job state and timestamps

### 5. Failure path
- [x] submit intentionally bad payload (simulated via processor error test path)
- [x] expect final status `failed`
- [x] expect `error` field populated

## Remaining manual verification
- [x] run full API + Redis + Postgres + worker locally and capture end-to-end evidence
- [x] capture success-path evidence (`completed` + `result`) from live local run
- [x] capture failure-path evidence (`failed` + `error`) from live local run
- [x] capture signal-driven worker shutdown evidence (`Ctrl+C` clean exit)

## Captured evidence (latest local run)
- Success record:
  - `job_id`: `a21dea10-f713-449b-a737-fe8bc6ff9e19`
  - terminal status: `completed`
  - result: `{"message":"Hello, World!"}`
- Failure record:
  - `job_id`: `004ede7e-3a75-4317-a574-931f6890c80c`
  - terminal status: `failed`
  - error: `injected processor failure for UAT`
- Worker log contract:
  - success path includes `msg="successfully processed job"` with `job_id`
  - failure path includes `msg="failed to process job"` with `job_id`
- Signal handling:
  - worker exits cleanly on `Ctrl+C` without timeout-forced shutdown output

## Step 5 verification status
- [x] worker executable compiles via `go test ./...` (`cmd/worker` package)
- [x] worker runtime has startup config parsing and connectivity checks for Postgres/Redis
- [x] capture manual local run evidence for signal-driven shutdown behavior
