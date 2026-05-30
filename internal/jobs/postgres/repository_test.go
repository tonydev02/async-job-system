package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/jobs/postgres"
)

func TestMigrationUpDownSmoke(t *testing.T) {
	db := openTestDB(t)

	if err := applyMigrationFile(t, db, "000001_create_jobs.up.sql"); err != nil {
		t.Fatalf("apply up migration: %v", err)
	}

	var tableName sql.NullString
	if err := db.QueryRow("SELECT to_regclass('public.jobs')").Scan(&tableName); err != nil {
		t.Fatalf("check jobs table exists: %v", err)
	}
	if !tableName.Valid || tableName.String != "jobs" {
		t.Fatalf("expected jobs table to exist, got: %#v", tableName)
	}

	if err := applyMigrationFile(t, db, "000001_create_jobs.down.sql"); err != nil {
		t.Fatalf("apply down migration: %v", err)
	}

	if err := db.QueryRow("SELECT to_regclass('public.jobs')").Scan(&tableName); err != nil {
		t.Fatalf("check jobs table removed: %v", err)
	}
	if tableName.Valid {
		t.Fatalf("expected jobs table to be removed, got: %#v", tableName)
	}
}

func TestRepositoryCreateAndGet(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"send_email"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if created.Status != jobs.StatusPending {
		t.Fatalf("expected pending status, got %s", created.Status)
	}
	if created.Attempt != 0 {
		t.Fatalf("expected attempt 0, got %d", created.Attempt)
	}
	if created.MaxAttempts != 3 {
		t.Fatalf("expected default max_attempts 3, got %d", created.MaxAttempts)
	}
	if string(created.Payload) != `{"task":"send_email"}` {
		t.Fatalf("unexpected payload: %s", created.Payload)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}

	if fetched.ID != created.ID {
		t.Fatalf("expected same job id, got %s", fetched.ID)
	}
	if string(fetched.Payload) != string(created.Payload) {
		t.Fatalf("expected payload round-trip, got %s", fetched.Payload)
	}
	if fetched.Result != nil {
		t.Fatalf("expected nil result, got %s", fetched.Result)
	}
	if fetched.Error != nil {
		t.Fatalf("expected nil error, got %v", *fetched.Error)
	}
	if fetched.StartedAt != nil || fetched.CompletedAt != nil || fetched.NextRunAt != nil {
		t.Fatal("expected nullable timestamps to be nil for pending job")
	}
}

func TestRepositoryPendingToProcessingOnlyOnce(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"resize"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("mark processing first time: %v", err)
	}
	if !ok {
		t.Fatal("expected first pending->processing transition to apply")
	}

	ok, err = repo.MarkProcessing(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("mark processing second time: %v", err)
	}
	if ok {
		t.Fatal("expected second pending->processing transition to be rejected")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusProcessing {
		t.Fatalf("expected processing status, got %s", fetched.Status)
	}
	if fetched.Attempt != 1 {
		t.Fatalf("expected attempt increment to 1, got %d", fetched.Attempt)
	}
	if fetched.StartedAt == nil {
		t.Fatal("expected started_at to be set")
	}
}

func TestRepositoryConcurrentMarkProcessingSingleWinner(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"contention-processing"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	const callers = 16
	results := runConcurrent(callers, func(int) (bool, error) {
		return repo.MarkProcessing(context.Background(), created.ID)
	})

	assertNoConcurrentErrors(t, results)
	if got := countApplied(results); got != 1 {
		t.Fatalf("expected exactly one successful MarkProcessing call, got %d", got)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusProcessing {
		t.Fatalf("expected processing status, got %s", fetched.Status)
	}
	if fetched.Attempt != 1 {
		t.Fatalf("expected exactly one attempt increment, got %d", fetched.Attempt)
	}
}

func TestRepositoryProcessingToCompleted(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"render"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	ok, err = repo.MarkCompleted(context.Background(), created.ID, json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if !ok {
		t.Fatal("expected processing->completed transition to apply")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed status, got %s", fetched.Status)
	}
	if string(fetched.Result) != `{"ok":true}` {
		t.Fatalf("unexpected result json: %s", fetched.Result)
	}
	if fetched.Error != nil {
		t.Fatalf("expected nil error, got %v", *fetched.Error)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestRepositoryConcurrentTerminalTransitionsAtMostOneApplies(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"contention-terminal"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	attempts := []func() (bool, error){
		func() (bool, error) {
			return repo.MarkCompleted(context.Background(), created.ID, json.RawMessage(`{"worker":1}`))
		},
		func() (bool, error) {
			return repo.MarkFailed(context.Background(), created.ID, "worker 2 failed")
		},
		func() (bool, error) {
			return repo.MarkCompleted(context.Background(), created.ID, json.RawMessage(`{"worker":3}`))
		},
		func() (bool, error) {
			return repo.MarkFailed(context.Background(), created.ID, "worker 4 failed")
		},
	}

	results := runConcurrent(len(attempts), func(i int) (bool, error) {
		return attempts[i]()
	})

	assertNoConcurrentErrors(t, results)
	if got := countApplied(results); got > 1 {
		t.Fatalf("expected at most one terminal transition to apply, got %d", got)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusCompleted && fetched.Status != jobs.StatusFailed {
		t.Fatalf("expected terminal status, got %s", fetched.Status)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected completed_at to be set by terminal transition")
	}
}

func TestRepositoryProcessingToFailed(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"render"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	ok, err = repo.MarkFailed(context.Background(), created.ID, "processor crashed")
	if err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if !ok {
		t.Fatal("expected processing->failed transition to apply")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusFailed {
		t.Fatalf("expected failed status, got %s", fetched.Status)
	}
	if fetched.Error == nil || *fetched.Error != "processor crashed" {
		t.Fatalf("expected persisted error, got %v", fetched.Error)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestRepositoryHandleProcessingFailure_SchedulesRetryBeforeMaxAttempts(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{
		Payload:     json.RawMessage(`{"task":"retry-first-failure"}`),
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	transition, err := repo.HandleProcessingFailure(context.Background(), created.ID, "temporary network issue", 2*time.Second)
	if err != nil {
		t.Fatalf("handle processing failure: %v", err)
	}
	if !transition.Applied {
		t.Fatal("expected failure transition to be applied")
	}
	if transition.Decision != jobs.FailureDecisionRetry {
		t.Fatalf("expected retry decision, got %s", transition.Decision)
	}
	if transition.NextRunAt == nil {
		t.Fatal("expected next_run_at for retry decision")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusPending {
		t.Fatalf("expected status pending after retry transition, got %s", fetched.Status)
	}
	if fetched.Attempt != 1 {
		t.Fatalf("expected attempt to be incremented to 1, got %d", fetched.Attempt)
	}
	if fetched.Error == nil || *fetched.Error != "temporary network issue" {
		t.Fatalf("expected persisted error text, got %v", fetched.Error)
	}
	if fetched.NextRunAt == nil {
		t.Fatal("expected next_run_at to be set")
	}
	if fetched.CompletedAt != nil {
		t.Fatal("expected completed_at to be nil for retry transition")
	}
}

func TestRepositoryHandleProcessingFailure_MarksTerminalAtMaxAttempts(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{
		Payload:     json.RawMessage(`{"task":"terminal-failure"}`),
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	transition, err := repo.HandleProcessingFailure(context.Background(), created.ID, "permanent failure", 2*time.Second)
	if err != nil {
		t.Fatalf("handle processing failure: %v", err)
	}
	if !transition.Applied {
		t.Fatal("expected failure transition to be applied")
	}
	if transition.Decision != jobs.FailureDecisionTerminal {
		t.Fatalf("expected terminal decision, got %s", transition.Decision)
	}
	if transition.NextRunAt != nil {
		t.Fatalf("expected next_run_at nil for terminal decision, got %v", *transition.NextRunAt)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusFailed {
		t.Fatalf("expected status failed, got %s", fetched.Status)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected completed_at to be set for terminal failure")
	}
	if fetched.NextRunAt != nil {
		t.Fatalf("expected next_run_at to be nil for terminal failure, got %v", *fetched.NextRunAt)
	}
}

func TestRepositoryRecoverStaleProcessing_SchedulesRetryBeforeMaxAttempts(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)

	created, err := repo.Create(context.Background(), jobs.CreateParams{
		Payload:     json.RawMessage(`{"task":"stale-retry"}`),
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}
	setStartedAt(t, db, created.ID, now.Add(-10*time.Minute))

	results, err := repo.RecoverStaleProcessing(context.Background(), now, 5*time.Minute, 2*time.Second, 10)
	if err != nil {
		t.Fatalf("recover stale processing: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one recovered job, got %d", len(results))
	}
	if results[0].ID != created.ID {
		t.Fatalf("unexpected recovered ID: got %s want %s", results[0].ID, created.ID)
	}
	if results[0].Decision != jobs.RecoveryDecisionRetry {
		t.Fatalf("expected retry decision, got %s", results[0].Decision)
	}
	if results[0].Attempt != 1 {
		t.Fatalf("expected attempt 1, got %d", results[0].Attempt)
	}
	if results[0].NextRunAt == nil {
		t.Fatal("expected next_run_at in recovery result")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusPending {
		t.Fatalf("expected pending after recovery, got %s", fetched.Status)
	}
	if fetched.Attempt != 1 {
		t.Fatalf("expected recovery not to increment attempt, got %d", fetched.Attempt)
	}
	if fetched.Error == nil || *fetched.Error != "processing visibility timeout expired" {
		t.Fatalf("expected recovery error text, got %v", fetched.Error)
	}
	if fetched.NextRunAt == nil {
		t.Fatal("expected next_run_at to be set")
	}
	if fetched.CompletedAt != nil {
		t.Fatal("expected completed_at nil for retry recovery")
	}
}

func TestRepositoryRecoverStaleProcessing_MarksTerminalAtMaxAttempts(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)

	created, err := repo.Create(context.Background(), jobs.CreateParams{
		Payload:     json.RawMessage(`{"task":"stale-terminal"}`),
		MaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}
	setStartedAt(t, db, created.ID, now.Add(-10*time.Minute))

	results, err := repo.RecoverStaleProcessing(context.Background(), now, 5*time.Minute, 2*time.Second, 10)
	if err != nil {
		t.Fatalf("recover stale processing: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one recovered job, got %d", len(results))
	}
	if results[0].Decision != jobs.RecoveryDecisionTerminal {
		t.Fatalf("expected terminal decision, got %s", results[0].Decision)
	}
	if results[0].NextRunAt != nil {
		t.Fatalf("expected terminal recovery next_run_at nil, got %v", *results[0].NextRunAt)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.Status != jobs.StatusFailed {
		t.Fatalf("expected failed after recovery, got %s", fetched.Status)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected completed_at for terminal recovery")
	}
	if fetched.NextRunAt != nil {
		t.Fatalf("expected next_run_at nil for terminal recovery, got %v", *fetched.NextRunAt)
	}
}

func TestRepositoryRecoverStaleProcessing_SkipsFreshAndNonProcessingJobs(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)

	fresh, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"fresh"}`)})
	if err != nil {
		t.Fatalf("create fresh job: %v", err)
	}
	ok, err := repo.MarkProcessing(context.Background(), fresh.ID)
	if err != nil || !ok {
		t.Fatalf("mark fresh processing: ok=%v err=%v", ok, err)
	}
	setStartedAt(t, db, fresh.ID, now.Add(-1*time.Minute))

	pending, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"pending"}`)})
	if err != nil {
		t.Fatalf("create pending job: %v", err)
	}

	completed, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"completed"}`)})
	if err != nil {
		t.Fatalf("create completed job: %v", err)
	}
	ok, err = repo.MarkProcessing(context.Background(), completed.ID)
	if err != nil || !ok {
		t.Fatalf("mark completed processing: ok=%v err=%v", ok, err)
	}
	ok, err = repo.MarkCompleted(context.Background(), completed.ID, json.RawMessage(`{"ok":true}`))
	if err != nil || !ok {
		t.Fatalf("mark completed: ok=%v err=%v", ok, err)
	}
	setStartedAt(t, db, completed.ID, now.Add(-10*time.Minute))

	failed, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"failed"}`)})
	if err != nil {
		t.Fatalf("create failed job: %v", err)
	}
	ok, err = repo.MarkProcessing(context.Background(), failed.ID)
	if err != nil || !ok {
		t.Fatalf("mark failed processing: ok=%v err=%v", ok, err)
	}
	ok, err = repo.MarkFailed(context.Background(), failed.ID, "failed already")
	if err != nil || !ok {
		t.Fatalf("mark failed: ok=%v err=%v", ok, err)
	}
	setStartedAt(t, db, failed.ID, now.Add(-10*time.Minute))

	results, err := repo.RecoverStaleProcessing(context.Background(), now, 5*time.Minute, 2*time.Second, 10)
	if err != nil {
		t.Fatalf("recover stale processing: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no recoveries, got %#v", results)
	}

	for _, tc := range []struct {
		id     uuid.UUID
		status jobs.Status
	}{
		{id: fresh.ID, status: jobs.StatusProcessing},
		{id: pending.ID, status: jobs.StatusPending},
		{id: completed.ID, status: jobs.StatusCompleted},
		{id: failed.ID, status: jobs.StatusFailed},
	} {
		fetched, err := repo.GetByID(context.Background(), tc.id)
		if err != nil {
			t.Fatalf("get job by id: %v", err)
		}
		if fetched.Status != tc.status {
			t.Fatalf("expected status %s for %s, got %s", tc.status, tc.id, fetched.Status)
		}
	}
}

func TestRepositoryConcurrentRecoverStaleProcessingDoesNotDuplicateIDs(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)
	now := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)

	const staleJobs = 12
	for i := 0; i < staleJobs; i++ {
		created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"concurrent-stale"}`)})
		if err != nil {
			t.Fatalf("create stale job %d: %v", i, err)
		}
		ok, err := repo.MarkProcessing(context.Background(), created.ID)
		if err != nil || !ok {
			t.Fatalf("mark processing %d: ok=%v err=%v", i, ok, err)
		}
		setStartedAt(t, db, created.ID, now.Add(-10*time.Minute))
	}

	const callers = 6
	type recoveryResult struct {
		ids []string
		err error
	}

	start := make(chan struct{})
	results := make([]recoveryResult, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start

			recovered, err := repo.RecoverStaleProcessing(context.Background(), now, 5*time.Minute, 2*time.Second, 3)
			results[i].err = err
			for _, result := range recovered {
				results[i].ids = append(results[i].ids, result.ID.String())
			}
		}(i)
	}

	close(start)
	wg.Wait()

	seen := make(map[string]struct{})
	totalRecovered := 0
	for i, result := range results {
		if result.err != nil {
			t.Fatalf("recover stale processing caller %d: %v", i, result.err)
		}
		for _, id := range result.ids {
			totalRecovered++
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate recovered job id across callers: %s", id)
			}
			seen[id] = struct{}{}
		}
	}

	if totalRecovered != staleJobs {
		t.Fatalf("expected all stale jobs to be recovered once, got %d of %d", totalRecovered, staleJobs)
	}
}

func TestRepositoryRetrySchedulingPreservesSubSecondDelay(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"retry-precision"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkProcessing(context.Background(), created.ID)
	if err != nil || !ok {
		t.Fatalf("mark processing: ok=%v err=%v", ok, err)
	}

	beforeFailureTransition := time.Now()
	transition, err := repo.HandleProcessingFailure(context.Background(), created.ID, "transient error", 1500*time.Millisecond)
	if err != nil {
		t.Fatalf("handle processing failure: %v", err)
	}
	if !transition.Applied || transition.Decision != jobs.FailureDecisionRetry || transition.NextRunAt == nil {
		t.Fatalf("unexpected transition result: %+v", transition)
	}

	if transition.NextRunAt.Before(beforeFailureTransition.Add(1200 * time.Millisecond)) {
		t.Fatalf("expected next_run_at to preserve sub-second delay, got %v", transition.NextRunAt)
	}

	beforeReschedule := time.Now()
	ok, err = repo.RescheduleRetry(context.Background(), created.ID, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("reschedule retry: %v", err)
	}
	if !ok {
		t.Fatal("expected reschedule retry to apply")
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.NextRunAt == nil {
		t.Fatal("expected next_run_at after reschedule")
	}
	if fetched.NextRunAt.Before(beforeReschedule.Add(100 * time.Millisecond)) {
		t.Fatalf("expected sub-second reschedule to be preserved, got next_run_at=%v", *fetched.NextRunAt)
	}
}

func TestRepositoryClaimDueRetries_ClearsNextRunAtOnClaim(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"claim-clear-next-run-at"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.RescheduleRetry(context.Background(), created.ID, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("reschedule retry: %v", err)
	}
	if !ok {
		t.Fatal("expected reschedule retry to apply")
	}

	claimedIDs, err := repo.ClaimDueRetries(context.Background(), time.Now().Add(2*time.Second), 10)
	if err != nil {
		t.Fatalf("claim due retries: %v", err)
	}
	if len(claimedIDs) != 1 || claimedIDs[0] != created.ID {
		t.Fatalf("unexpected claimed ids: %#v", claimedIDs)
	}

	fetched, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get job by id: %v", err)
	}
	if fetched.NextRunAt != nil {
		t.Fatalf("expected next_run_at to be cleared during claim, got %v", *fetched.NextRunAt)
	}
}

func TestRepositoryConcurrentClaimDueRetriesDoesNotDuplicateIDs(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	const dueJobs = 12
	for i := 0; i < dueJobs; i++ {
		created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"contention-claim"}`)})
		if err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}

		ok, err := repo.RescheduleRetry(context.Background(), created.ID, time.Hour)
		if err != nil {
			t.Fatalf("reschedule retry %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("expected reschedule retry %d to apply", i)
		}
	}

	const callers = 6
	type claimResult struct {
		ids []string
		err error
	}

	claimCutoff := time.Now().Add(2 * time.Hour)
	start := make(chan struct{})
	results := make([]claimResult, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			<-start

			ids, err := repo.ClaimDueRetries(context.Background(), claimCutoff, 3)
			results[i].err = err
			for _, id := range ids {
				results[i].ids = append(results[i].ids, id.String())
			}
		}(i)
	}

	close(start)
	wg.Wait()

	seen := make(map[string]struct{})
	totalClaimed := 0
	for i, result := range results {
		if result.err != nil {
			t.Fatalf("claim due retries caller %d: %v", i, result.err)
		}
		for _, id := range result.ids {
			totalClaimed++
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate claimed job id across callers: %s", id)
			}
			seen[id] = struct{}{}
		}
	}

	if totalClaimed != dueJobs {
		t.Fatalf("expected all due jobs to be claimed once, got %d of %d", totalClaimed, dueJobs)
	}
}

func TestRepositoryInvalidTransitionPendingToCompleted(t *testing.T) {
	db := setupDBWithJobsTable(t)
	repo := postgres.NewRepository(db)

	created, err := repo.Create(context.Background(), jobs.CreateParams{Payload: json.RawMessage(`{"task":"render"}`)})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	ok, err := repo.MarkCompleted(context.Background(), created.ID, json.RawMessage(`{"ok":true}`))
	if err != nil {
		t.Fatalf("mark completed from pending: %v", err)
	}
	if ok {
		t.Fatal("expected pending->completed transition to be rejected")
	}
}

type concurrentBoolResult struct {
	applied bool
	err     error
}

func runConcurrent(count int, fn func(int) (bool, error)) []concurrentBoolResult {
	start := make(chan struct{})
	results := make([]concurrentBoolResult, count)

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(i int) {
			defer wg.Done()
			<-start

			applied, err := fn(i)
			results[i] = concurrentBoolResult{
				applied: applied,
				err:     err,
			}
		}(i)
	}

	close(start)
	wg.Wait()

	return results
}

func assertNoConcurrentErrors(t *testing.T, results []concurrentBoolResult) {
	t.Helper()

	for i, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent call %d: %v", i, result.err)
		}
	}
}

func countApplied(results []concurrentBoolResult) int {
	var count int
	for _, result := range results {
		if result.applied {
			count++
		}
	}
	return count
}

func setStartedAt(t *testing.T, db *sql.DB, id uuid.UUID, startedAt time.Time) {
	t.Helper()

	res, err := db.Exec(`UPDATE jobs SET started_at = $2, updated_at = $2 WHERE id = $1`, id, startedAt)
	if err != nil {
		t.Fatalf("set started_at: %v", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		t.Fatalf("set started_at rows affected: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected one row updated for started_at, got %d", affected)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration tests")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func setupDBWithJobsTable(t *testing.T) *sql.DB {
	t.Helper()

	db := openTestDB(t)
	if err := applyMigrationFile(t, db, "000001_create_jobs.down.sql"); err != nil {
		t.Fatalf("cleanup down migration: %v", err)
	}
	if err := applyMigrationFile(t, db, "000001_create_jobs.up.sql"); err != nil {
		t.Fatalf("setup up migration: %v", err)
	}

	t.Cleanup(func() {
		_ = applyMigrationFile(t, db, "000001_create_jobs.down.sql")
	})

	return db
}

func applyMigrationFile(t *testing.T, db *sql.DB, fileName string) error {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(projectRoot(t), "migrations", fileName))
	if err != nil {
		return err
	}

	for _, stmt := range splitSQLStatements(string(content)) {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	stmts := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		stmts = append(stmts, stmt)
	}
	return stmts
}

func projectRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine current file path")
	}

	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
