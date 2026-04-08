package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type CreateParams struct {
	Payload     json.RawMessage
	MaxAttempts int
}

type Repository interface {
	Create(ctx context.Context, params CreateParams) (Job, error)
	GetByID(ctx context.Context, id uuid.UUID) (Job, error)
	MarkProcessing(ctx context.Context, id uuid.UUID) (bool, error)
	MarkCompleted(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error)
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) (bool, error)
	HandleProcessingFailure(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (FailureTransitionResult, error)
	ClaimDueRetries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	RescheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error)
}

type FailureDecision string

const (
	FailureDecisionRetry    FailureDecision = "retry"
	FailureDecisionTerminal FailureDecision = "terminal_failed"
)

type FailureTransitionResult struct {
	Applied     bool
	Decision    FailureDecision
	Attempt     int
	MaxAttempts int
	NextRunAt   *time.Time
}
