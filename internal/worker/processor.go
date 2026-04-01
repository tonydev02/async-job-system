package worker

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

type Processor interface {
	Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error)
}

type DeterministicProcessor struct{}

func (p *DeterministicProcessor) Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
	result := map[string]string{
		"message": "Hello, World!",
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}
