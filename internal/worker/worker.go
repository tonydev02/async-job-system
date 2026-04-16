package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

const defaultProcessingFailureRetryDelay = 30 * time.Second
const defaultRetryDispatchInterval = 1 * time.Minute
const defaultRetryDispatchBatchSize = 10
const defaultRetryReenqueueDelay = 1 * time.Minute

type Worker struct {
	repo                        jobs.Repository
	queue                       queue.Queue
	processor                   Processor
	logger                      *slog.Logger
	processingFailureRetryDelay time.Duration
	retryDispatchInterval       time.Duration
	retryDispatchBatchSize      int
	retryReenqueueDelay         time.Duration
}

type RetryRuntimeConfig struct {
	RetryDelay        time.Duration
	DispatchInterval  time.Duration
	DispatchBatchSize int
	ReenqueueDelay    time.Duration
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
		repo:                        repo,
		queue:                       queue,
		processor:                   processor,
		logger:                      logger,
		processingFailureRetryDelay: defaultProcessingFailureRetryDelay,
		retryDispatchInterval:       defaultRetryDispatchInterval,
		retryDispatchBatchSize:      defaultRetryDispatchBatchSize,
		retryReenqueueDelay:         defaultRetryReenqueueDelay,
	}
}

func (w *Worker) SetRetryRuntimeConfig(cfg RetryRuntimeConfig) error {
	if cfg.RetryDelay <= 0 {
		return errors.New("retry delay must be greater than zero")
	}
	if cfg.DispatchInterval <= 0 {
		return errors.New("retry dispatch interval must be greater than zero")
	}
	if cfg.DispatchBatchSize <= 0 {
		return errors.New("retry dispatch batch size must be greater than zero")
	}
	if cfg.ReenqueueDelay <= 0 {
		return errors.New("retry reenqueue delay must be greater than zero")
	}

	w.processingFailureRetryDelay = cfg.RetryDelay
	w.retryDispatchInterval = cfg.DispatchInterval
	w.retryDispatchBatchSize = cfg.DispatchBatchSize
	w.retryReenqueueDelay = cfg.ReenqueueDelay
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	go w.runRetryDispatcher(ctx)

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

		transition, markErr := w.repo.HandleProcessingFailure(ctx, jobID, err.Error(), w.processingFailureRetryDelay)
		if markErr != nil {
			w.logger.Error("failed to handle processing failure", "job_id", jobID, "error", markErr)
			return markErr
		}
		if !transition.Applied {
			w.logger.Info("processing failure transition was already applied by another worker", "job_id", jobID)
			return err
		}

		switch transition.Decision {
		case jobs.FailureDecisionRetry:
			w.logger.Info(
				"job failure transitioned to retry",
				"job_id", jobID,
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
				"next_run_at", transition.NextRunAt,
			)
		case jobs.FailureDecisionTerminal:
			w.logger.Info(
				"job failure transitioned to terminal failed",
				"job_id", jobID,
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
			)
		default:
			w.logger.Warn(
				"job failure transition returned unknown decision",
				"job_id", jobID,
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
			)
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

func (w *Worker) runRetryDispatcher(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	if err := w.dispatchDueRetries(ctx, time.Now()); err != nil {
		w.logger.Error("failed to dispatch retries", "error", err)
	}

	ticker := time.NewTicker(w.retryDispatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.dispatchDueRetries(ctx, time.Now()); err != nil {
				w.logger.Error("failed to dispatch retries", "error", err)
			}
		}
	}
}

func (w *Worker) dispatchDueRetries(ctx context.Context, now time.Time) error {
	ids, err := w.repo.ClaimDueRetries(ctx, now, w.retryDispatchBatchSize)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := w.queue.Enqueue(ctx, queue.Message{JobID: id}); err != nil {
			w.logger.Error("failed to re-enqueue job for retry", "job_id", id, "error", err)
			ok, resErr := w.repo.RescheduleRetry(ctx, id, w.retryReenqueueDelay)
			if resErr != nil {
				w.logger.Error("failed to reschedule retry after enqueue failure", "job_id", id, "error", resErr)
			}
			if !ok {
				w.logger.Warn("retry reschedule was not applied", "job_id", id)
			}
			continue
		}
		w.logger.Info("dispatched job for retry", "job_id", id)
	}
	return nil
}
