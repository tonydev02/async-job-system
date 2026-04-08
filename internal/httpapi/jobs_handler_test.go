package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type fakeRepo struct {
	createFn  func(ctx context.Context, params jobs.CreateParams) (jobs.Job, error)
	getByIDFn func(ctx context.Context, id uuid.UUID) (jobs.Job, error)
}

type fakeQueue struct {
	enqueueFn func(ctx context.Context, msg queue.Message) error
	dequeueFn func(ctx context.Context) (queue.Message, error)
}

func (f *fakeRepo) Create(ctx context.Context, params jobs.CreateParams) (jobs.Job, error) {
	if f.createFn != nil {
		return f.createFn(ctx, params)
	}
	return jobs.Job{}, nil
}

func (f *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (jobs.Job, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return jobs.Job{}, nil
}

func (f *fakeRepo) MarkProcessing(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeRepo) MarkCompleted(context.Context, uuid.UUID, json.RawMessage) (bool, error) {
	return false, nil
}

func (f *fakeRepo) MarkFailed(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}

func (f *fakeRepo) HandleProcessingFailure(context.Context, uuid.UUID, string, time.Duration) (jobs.FailureTransitionResult, error) {
	return jobs.FailureTransitionResult{}, nil
}

func (f *fakeRepo) ClaimDueRetries(context.Context, time.Time, int) ([]uuid.UUID, error) {
	return nil, nil
}

func (f *fakeRepo) RescheduleRetry(context.Context, uuid.UUID, time.Duration) (bool, error) {
	return false, nil
}

func (f *fakeQueue) Enqueue(ctx context.Context, msg queue.Message) error {
	if f.enqueueFn != nil {
		return f.enqueueFn(ctx, msg)
	}
	return nil
}

func (f *fakeQueue) Dequeue(ctx context.Context) (queue.Message, error) {
	if f.dequeueFn != nil {
		return f.dequeueFn(ctx)
	}
	return queue.Message{}, nil
}

func TestNewJobsHandlerNilRepo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when repo is nil, but it didn't panic")
		}
	}()

	NewJobsHandler(nil, &fakeQueue{})
}

func TestNewJobsHandlerNilQueue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when queue is nil, but it didn't panic")
		}
	}()

	NewJobsHandler(&fakeRepo{}, nil)
}

func TestPostJobsSuccess(t *testing.T) {
	expectedID := uuid.New()
	repo := &fakeRepo{
		createFn: func(_ context.Context, params jobs.CreateParams) (jobs.Job, error) {
			if string(params.Payload) != `{"task":"send_email"}` {
				t.Fatalf("unexpected payload: %s", string(params.Payload))
			}
			return jobs.Job{
				ID:     expectedID,
				Status: jobs.StatusPending,
			}, nil
		},
	}

	enqueueCalled := false
	queue := &fakeQueue{
		enqueueFn: func(_ context.Context, msg queue.Message) error {
			enqueueCalled = true
			if msg.JobID != expectedID {
				t.Fatalf("expected to enqueue job ID %s, got %s", expectedID, msg.JobID)
			}
			return nil
		},
	}
	handler := NewJobsHandler(repo, queue)
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{"payload":{"task":"send_email"}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := resp["job_id"]; got != expectedID.String() {
		t.Fatalf("expected job_id %q, got %#v", expectedID.String(), got)
	}
	if got := resp["status"]; got != string(jobs.StatusPending) {
		t.Fatalf("expected status %q, got %#v", jobs.StatusPending, got)
	}
	if !enqueueCalled {
		t.Fatal("expected Enqueue to be called, but it wasn't")
	}
}
func TestPostJobsEnqueueFailure(t *testing.T) {
	repo := &fakeRepo{
		createFn: func(_ context.Context, params jobs.CreateParams) (jobs.Job, error) {
			return jobs.Job{
				ID:     uuid.New(),
				Status: jobs.StatusPending,
			}, nil
		},
	}

	queue := &fakeQueue{
		enqueueFn: func(_ context.Context, _ queue.Message) error {
			return errors.New("enqueue failed")
		},
	}

	handler := NewJobsHandler(repo, queue)
	router := NewRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{"payload":{"task":"send_email"}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "failed to enqueue job") {
		t.Fatalf("expected error body to contain %q, got %q", "failed to enqueue job", rec.Body.String())
	}
}

func TestGetJobByIDNotFound(t *testing.T) {
	repo := &fakeRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (jobs.Job, error) {
			return jobs.Job{}, sql.ErrNoRows
		},
	}

	queue := &fakeQueue{}
	handler := NewJobsHandler(repo, queue)
	router := NewRouter(handler)
	jobID := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "job not found") {
		t.Fatalf("expected error body to contain %q, got %q", "job not found", rec.Body.String())
	}
}
