package jobs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Job struct {
	ID          uuid.UUID
	Status      Status
	Payload     json.RawMessage
	Result      json.RawMessage
	Error       *string
	Attempt     int
	MaxAttempts int
	NextRunAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}
