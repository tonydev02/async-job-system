package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type Worker struct {
	repo      jobs.Repository
	queue     queue.Queue
	processor Processor
	logger    *slog.Logger
}

func NewWorker(repo jobs.Repository, queue queue.Queue, processor Processor, logger *slog.Logger) *Worker {
	if repo == nil {
		panic("jobs repository is required")
	}
	if queue == nil {
		panic("queue is required")
	}
	if processor == nil {
		panic("processor is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		repo:      repo,
		queue:     queue,
		processor: processor,
		logger:    logger,
	}
}

func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := w.queue.Dequeue(ctx)
			if err != nil {
				if errors.Is(err, queue.ErrEmpty) {
					continue
				}
				w.logger.Error("failed to dequeue message", "error", err)
				continue
			}
			if err := w.handleMessage(ctx, msg); err != nil {
				w.logger.Error("failed to handle message", "error", err)
			}
		}
	}
}

func (w *Worker) handleMessage(ctx context.Context, msg queue.Message) error {
	ok, err := w.repo.MarkProcessing(ctx, msg.JobID)
	if err != nil {
		w.logger.Error("failed to mark job as processing", "job_id", msg.JobID, "error", err)
		return err
	}
	if !ok {
		w.logger.Info("job is already being processed by another worker", "job_id", msg.JobID)
		return nil
	}

	return w.processJob(ctx, msg.JobID)
}

func (w *Worker) processJob(ctx context.Context, jobID uuid.UUID) error {
	result, err := w.processor.Process(ctx, jobID)
	if err != nil {
		w.logger.Error("failed to process job", "job_id", jobID, "error", err)
		ok, markErr := w.repo.MarkFailed(ctx, jobID, err.Error())
		if markErr != nil {
			w.logger.Error("failed to mark job as failed", "job_id", jobID, "error", markErr)
			return markErr
		}
		if !ok {
			w.logger.Info("job is already marked as failed by another worker", "job_id", jobID)
		}
		return err
	}

	ok, err := w.repo.MarkCompleted(ctx, jobID, result)
	if err != nil {
		w.logger.Error("failed to mark job as completed", "job_id", jobID, "error", err)
		return err
	}
	if !ok {
		w.logger.Info("job is already marked as completed by another worker", "job_id", jobID)
		return nil
	}

	w.logger.Info("successfully processed job", "job_id", jobID)
	return nil
}
