# PHASE-UAT.md

## Objective
Verify the MVP async flow end-to-end in local development.

## Test cases

### 1. Submit job
- [ ] call `POST /jobs`
- [ ] expect HTTP 202 or 201
- [ ] expect returned `job_id`
- [ ] expect DB record with status `pending`

### 2. Worker picks job
- [ ] start worker
- [ ] expect log containing job ID
- [ ] expect DB record changes to `processing`

### 3. Worker completes job
- [ ] wait for processing
- [ ] expect DB record changes to `completed`
- [ ] expect `result` field populated

### 4. Poll status endpoint
- [ ] call `GET /jobs/{id}`
- [ ] expect correct job state and timestamps

### 5. Failure path
- [ ] submit intentionally bad payload
- [ ] expect final status `failed`
- [ ] expect `error` field populated
