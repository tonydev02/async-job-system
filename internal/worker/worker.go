package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

const defaultProcessingFailureRetryDelay = 30 * time.Second
const defaultRetryDispatchInterval = 1 * time.Minute
const defaultRetryDispatchBatchSize = 10
const defaultRetryReenqueueDelay = 1 * time.Minute
const defaultWorkerConcurrency = 1
const defaultShutdownTimeout = 10 * time.Second

type Worker struct {
	repo                        jobs.Repository
	queue                       queue.Queue
	processor                   Processor
	logger                      *slog.Logger
	processingFailureRetryDelay time.Duration
	retryDispatchInterval       time.Duration
	retryDispatchBatchSize      int
	retryReenqueueDelay         time.Duration
	concurrency                 int
	shutdownTimeout             time.Duration
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
		concurrency:                 defaultWorkerConcurrency,
		shutdownTimeout:             defaultShutdownTimeout,
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

func (w *Worker) SetConcurrency(concurrency int) error {
	if concurrency <= 0 {
		return errors.New("worker concurrency must be greater than zero")
	}
	w.concurrency = concurrency
	return nil
}

func (w *Worker) SetShutdownTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return errors.New("worker shutdown timeout must be greater than zero")
	}
	w.shutdownTimeout = timeout
	return nil
}

func (w *Worker) Run(ctx context.Context) {
	go w.runRetryDispatcher(ctx)

	msgs := make(chan queue.Message)
	var wg sync.WaitGroup
	drainCtx, cancelDrain := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelDrain()

	for i := 0; i < w.concurrency; i++ {
		workerSlot := i + 1
		wg.Add(1)
		go func(workerSlot int) {
			defer wg.Done()
			logger := w.logger.With("worker_slot", workerSlot)
			for msg := range msgs {
				if err := w.handleMessage(drainCtx, msg, logger); err != nil {
					logger.Error("failed to handle message", "job_id", msg.JobID, "error", err)
				}
			}
		}(workerSlot)
	}
	defer func() {
		close(msgs)
		w.waitForInFlightJobs(&wg, cancelDrain)
	}()

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
			select {
			case <-ctx.Done():
				return
			case msgs <- msg:
			}
		}
	}
}

func (w *Worker) waitForInFlightJobs(wg *sync.WaitGroup, cancelDrain context.CancelFunc) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(w.shutdownTimeout)
	defer timer.Stop()

	select {
	case <-done:
		return
	case <-timer.C:
		w.logger.Warn("worker shutdown drain timeout expired", "timeout", w.shutdownTimeout)
		cancelDrain()
		return
	}
}

func (w *Worker) handleMessage(ctx context.Context, msg queue.Message, logger *slog.Logger) error {
	ok, err := w.repo.MarkProcessing(ctx, msg.JobID)
	if err != nil {
		logger.Error(
			"failed to mark job as processing",
			"job_id", msg.JobID,
			"transition", "pending_to_processing",
			"error", err,
		)
		return err
	}
	if !ok {
		logger.Info(
			"job processing claim transition skipped",
			"job_id", msg.JobID,
			"transition", "pending_to_processing",
			"transition_applied", false,
			"transition_outcome", "skipped",
		)
		return nil
	}

	logger.Info(
		"job processing claim transition applied",
		"job_id", msg.JobID,
		"transition", "pending_to_processing",
		"transition_applied", true,
		"transition_outcome", "claimed",
	)
	return w.processJob(ctx, msg.JobID, logger)
}

func (w *Worker) processJob(ctx context.Context, jobID uuid.UUID, logger *slog.Logger) error {
	result, err := w.processor.Process(ctx, jobID)
	if err != nil {
		logger.Error("failed to process job", "job_id", jobID, "error", err)

		transition, markErr := w.repo.HandleProcessingFailure(ctx, jobID, err.Error(), w.processingFailureRetryDelay)
		if markErr != nil {
			logger.Error(
				"failed to handle processing failure",
				"job_id", jobID,
				"transition", "processing_to_failure_decision",
				"error", markErr,
			)
			return markErr
		}
		if !transition.Applied {
			logger.Info(
				"processing failure transition skipped",
				"job_id", jobID,
				"transition", "processing_to_failure_decision",
				"transition_applied", false,
				"transition_outcome", "skipped",
			)
			return err
		}

		switch transition.Decision {
		case jobs.FailureDecisionRetry:
			logger.Info(
				"job failure transitioned to retry",
				"job_id", jobID,
				"transition", "processing_to_pending",
				"transition_applied", true,
				"transition_outcome", "retry_scheduled",
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
				"next_run_at", transition.NextRunAt,
			)
		case jobs.FailureDecisionTerminal:
			logger.Info(
				"job failure transitioned to terminal failed",
				"job_id", jobID,
				"transition", "processing_to_failed",
				"transition_applied", true,
				"transition_outcome", "terminal_failed",
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
			)
		default:
			logger.Warn(
				"job failure transition returned unknown decision",
				"job_id", jobID,
				"transition", "processing_to_failure_decision",
				"transition_applied", true,
				"transition_outcome", "unknown",
				"decision", transition.Decision,
				"attempt", transition.Attempt,
				"max_attempts", transition.MaxAttempts,
			)
		}
		return err
	}

	ok, err := w.repo.MarkCompleted(ctx, jobID, result)
	if err != nil {
		logger.Error(
			"failed to mark job as completed",
			"job_id", jobID,
			"transition", "processing_to_completed",
			"error", err,
		)
		return err
	}
	if !ok {
		logger.Info(
			"job completion transition skipped",
			"job_id", jobID,
			"transition", "processing_to_completed",
			"transition_applied", false,
			"transition_outcome", "skipped",
		)
		return nil
	}

	logger.Info(
		"successfully processed job",
		"job_id", jobID,
		"transition", "processing_to_completed",
		"transition_applied", true,
		"transition_outcome", "completed",
	)
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
