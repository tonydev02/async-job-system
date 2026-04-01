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
- [ ] run full API + Redis + Postgres + worker with Docker Compose and capture end-to-end evidence
