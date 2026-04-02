package worker

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Processor interface {
	Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error)
}

type DeterministicProcessor struct {
	FailJobID string
}

func (p *DeterministicProcessor) Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
	if p.FailJobID != "" && p.FailJobID == jobID.String() {
		return nil, errors.New("injected processor failure for UAT")
	}

	result := map[string]string{
		"message": "Hello, World!",
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return json.Marshal(result)
}
