package jobs

import (
	"context"
	"encoding/json"

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
}
