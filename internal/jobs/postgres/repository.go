package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
)

const defaultMaxAttempts = 3

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	if db == nil {
		panic("database connection is required")
	}
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, params jobs.CreateParams) (jobs.Job, error) {
	if len(params.Payload) == 0 {
		return jobs.Job{}, errors.New("payload is required")
	}

	maxAttempts := params.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}

	id := uuid.New()
	const query = `
		INSERT INTO jobs (id, status, payload, max_attempts)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, payload, result, error, attempt, max_attempts, next_run_at, created_at, updated_at, started_at, completed_at
	`

	job, err := scanJob(r.db.QueryRowContext(ctx, query, id, jobs.StatusPending, params.Payload, maxAttempts))
	if err != nil {
		return jobs.Job{}, err
	}

	return job, nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (jobs.Job, error) {
	const query = `
		SELECT id, status, payload, result, error, attempt, max_attempts, next_run_at, created_at, updated_at, started_at, completed_at
		FROM jobs
		WHERE id = $1
	`

	job, err := scanJob(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return jobs.Job{}, err
	}

	return job, nil
}

func (r *Repository) MarkProcessing(ctx context.Context, id uuid.UUID) (bool, error) {
	const query = `
		UPDATE jobs
		SET status = $2,
			started_at = now(),
			updated_at = now(),
			attempt = attempt + 1
		WHERE id = $1 AND status = $3
	`

	res, err := r.db.ExecContext(ctx, query, id, jobs.StatusProcessing, jobs.StatusPending)
	if err != nil {
		return false, err
	}

	return rowsAffected(res)
}

func (r *Repository) MarkCompleted(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
	if len(result) == 0 {
		return false, errors.New("result is required")
	}

	const query = `
		UPDATE jobs
		SET status = $2,
			result = $3,
			error = NULL,
			completed_at = now(),
			updated_at = now()
		WHERE id = $1 AND status = $4
	`

	res, err := r.db.ExecContext(ctx, query, id, jobs.StatusCompleted, result, jobs.StatusProcessing)
	if err != nil {
		return false, err
	}

	return rowsAffected(res)
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) (bool, error) {
	if errMsg == "" {
		return false, errors.New("errMsg is required")
	}

	const query = `
		UPDATE jobs
		SET status = $2,
			error = $3,
			completed_at = now(),
			updated_at = now()
		WHERE id = $1 AND status = $4
	`

	res, err := r.db.ExecContext(ctx, query, id, jobs.StatusFailed, errMsg, jobs.StatusProcessing)
	if err != nil {
		return false, err
	}

	return rowsAffected(res)
}

func (r *Repository) HandleProcessingFailure(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
	if errMsg == "" {
		return jobs.FailureTransitionResult{}, errors.New("errMsg is required")
	}

	if retryDelay <= 0 {
		return jobs.FailureTransitionResult{}, errors.New("retryDelay must be greater than zero")
	}

	const query = `
		UPDATE jobs
		SET status = CASE	
			WHEN attempt >= max_attempts THEN $2
			ELSE $3
		END,
		error = $4,
		next_run_at = CASE
			WHEN attempt >= max_attempts THEN NULL
			ELSE now() + make_interval(secs => $5)
		END,
		completed_at = CASE
			WHEN attempt >= max_attempts THEN now()
			ELSE NULL
		END,
		updated_at = now()
		WHERE id = $1 AND status = $6
		RETURNING attempt, max_attempts, status, next_run_at
	`

	var (
		attempt     int
		maxAttempts int
		status      string
		nextRunAt   sql.NullTime
	)

	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		jobs.StatusFailed,
		jobs.StatusPending,
		errMsg,
		retryDelay.Seconds(),
		jobs.StatusProcessing,
	).Scan(&attempt, &maxAttempts, &status, &nextRunAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return jobs.FailureTransitionResult{Applied: false}, nil
		}
		return jobs.FailureTransitionResult{}, err
	}

	result := jobs.FailureTransitionResult{
		Applied:     true,
		Attempt:     attempt,
		MaxAttempts: maxAttempts,
	}

	if status == string(jobs.StatusPending) {
		result.Decision = jobs.FailureDecisionRetry
		if nextRunAt.Valid {
			t := nextRunAt.Time
			result.NextRunAt = &t
		}
	} else {
		result.Decision = jobs.FailureDecisionTerminal
		result.NextRunAt = nil
	}

	return result, nil
}

func (r *Repository) ClaimDueRetries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be greater than zero")
	}

	const query = `
		UPDATE jobs
		SET 
			next_run_at = NULL,
			updated_at = now()
		WHERE id IN (
			SELECT id
			FROM jobs
			WHERE status = $1
				AND next_run_at IS NOT NULL
				AND next_run_at <= $2
			ORDER BY next_run_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		RETURNING id
	`

	rows, err := r.db.QueryContext(ctx, query, jobs.StatusPending, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}

func (r *Repository) RescheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error) {
	if delay <= 0 {
		return false, errors.New("delay must be greater than zero")
	}

	query := `
		UPDATE jobs
		SET 
			next_run_at = now() + make_interval(secs => $2),
			updated_at = now()
		WHERE id = $1 
			AND status = $3
		RETURNING id
	`

	var returnedID uuid.UUID
	err := r.db.QueryRowContext(
		ctx,
		query,
		id,
		delay.Seconds(),
		jobs.StatusPending,
	).Scan(&returnedID)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func rowsAffected(res sql.Result) (bool, error) {
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected == 1, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (jobs.Job, error) {
	var (
		job         jobs.Job
		result      []byte
		errText     sql.NullString
		nextRunAt   sql.NullTime
		startedAt   sql.NullTime
		completedAt sql.NullTime
	)

	err := row.Scan(
		&job.ID,
		&job.Status,
		&job.Payload,
		&result,
		&errText,
		&job.Attempt,
		&job.MaxAttempts,
		&nextRunAt,
		&job.CreatedAt,
		&job.UpdatedAt,
		&startedAt,
		&completedAt,
	)
	if err != nil {
		return jobs.Job{}, err
	}

	if len(result) > 0 {
		job.Result = append([]byte(nil), result...)
	}

	if errText.Valid {
		v := errText.String
		job.Error = &v
	}

	if nextRunAt.Valid {
		t := nextRunAt.Time
		job.NextRunAt = &t
	}

	if startedAt.Valid {
		t := startedAt.Time
		job.StartedAt = &t
	}

	if completedAt.Valid {
		t := completedAt.Time
		job.CompletedAt = &t
	}

	return job, nil
}

var _ jobs.Repository = (*Repository)(nil)
