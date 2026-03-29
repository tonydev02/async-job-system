package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
)

type fakeRepo struct {
	createFn  func(ctx context.Context, params jobs.CreateParams) (jobs.Job, error)
	getByIDFn func(ctx context.Context, id uuid.UUID) (jobs.Job, error)
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

	handler := NewJobsHandler(repo)
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
}

func TestGetJobByIDNotFound(t *testing.T) {
	repo := &fakeRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (jobs.Job, error) {
			return jobs.Job{}, sql.ErrNoRows
		},
	}

	handler := NewJobsHandler(repo)
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
