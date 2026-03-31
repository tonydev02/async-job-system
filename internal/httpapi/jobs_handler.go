package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type JobsHandler struct {
	repo  jobs.Repository
	queue queue.Queue
}

type createJobRequest struct {
	Payload json.RawMessage `json:"payload"`
}

type createJobResponse struct {
	JobID  string      `json:"job_id"`
	Status jobs.Status `json:"status"`
}

type getJobResponse struct {
	ID          string          `json:"id"`
	Status      jobs.Status     `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	Result      json.RawMessage `json:"result"`
	Error       *string         `json:"error"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	NextRunAt   *string         `json:"next_run_at"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	StartedAt   *string         `json:"started_at"`
	CompletedAt *string         `json:"completed_at"`
}

func NewJobsHandler(repo jobs.Repository, q queue.Queue) *JobsHandler {
	if repo == nil {
		panic("jobs repository is required")
	}
	if q == nil {
		panic("queue is required")
	}
	return &JobsHandler{repo: repo, queue: q}
}

func (h *JobsHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Payload) == 0 {
		http.Error(w, "payload is required", http.StatusBadRequest)
		return
	}

	job, err := h.repo.Create(r.Context(), jobs.CreateParams{Payload: req.Payload})
	if err != nil {
		http.Error(w, "failed to create job", http.StatusInternalServerError)
		return
	}

	if err := h.queue.Enqueue(r.Context(), queue.Message{JobID: job.ID}); err != nil {
		http.Error(w, "failed to enqueue job", http.StatusServiceUnavailable)
		return
	}

	resp := createJobResponse{JobID: job.ID.String(), Status: job.Status}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

func (h *JobsHandler) GetJobByID(w http.ResponseWriter, r *http.Request) {
	paramID := strings.TrimPrefix(r.URL.Path, "/jobs/")
	id, err := uuid.Parse(paramID)
	if err != nil {
		http.Error(w, "invalid job ID", http.StatusBadRequest)
		return
	}

	job, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "job not found", http.StatusNotFound)
		} else {
			http.Error(w, "failed to fetch job", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(getJobResponse{
		ID:          job.ID.String(),
		Status:      job.Status,
		Payload:     job.Payload,
		Result:      job.Result,
		Error:       job.Error,
		Attempt:     job.Attempt,
		MaxAttempts: job.MaxAttempts,
		NextRunAt:   formatTimePtr(job.NextRunAt),
		CreatedAt:   job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   job.UpdatedAt.Format(time.RFC3339),
		StartedAt:   formatTimePtr(job.StartedAt),
		CompletedAt: formatTimePtr(job.CompletedAt),
	})
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.RFC3339)
	return &s
}
