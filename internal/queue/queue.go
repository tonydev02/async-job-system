package queue

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Message struct {
	JobID uuid.UUID
}

type Queue interface {
	Enqueue(ctx context.Context, msg Message) error
	Dequeue(ctx context.Context) (Message, error)
}

var ErrEmpty = errors.New("queue is empty")
