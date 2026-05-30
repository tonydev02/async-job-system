package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type fakeRepo struct {
	mu sync.Mutex

	createFn                  func(ctx context.Context, params jobs.CreateParams) (jobs.Job, error)
	getByIDFn                 func(ctx context.Context, id uuid.UUID) (jobs.Job, error)
	markProcessingFn          func(ctx context.Context, id uuid.UUID) (bool, error)
	markCompletedFn           func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error)
	markFailedFn              func(ctx context.Context, id uuid.UUID, errMsg string) (bool, error)
	handleProcessingFailureFn func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error)
	claimDueRetriesFn         func(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	rescheduleRetryFn         func(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error)
	recoverStaleProcessingFn  func(ctx context.Context, now time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error)

	createCalls                  int
	getByIDCalls                 int
	markProcessingCalls          int
	markCompletedCalls           int
	markFailedCalls              int
	handleProcessingFailureCalls int
	claimDueRetriesCalls         int
	rescheduleRetryCalls         int
	recoverStaleProcessingCalls  int

	lastGetByIDID                uuid.UUID
	lastMarkProcessingID         uuid.UUID
	lastMarkCompletedID          uuid.UUID
	lastMarkCompletedRes         json.RawMessage
	lastMarkFailedID             uuid.UUID
	lastMarkFailedErr            string
	lastHandleFailureID          uuid.UUID
	lastHandleFailureErr         string
	lastHandleFailureDelay       time.Duration
	lastClaimDueRetriesNow       time.Time
	lastClaimDueRetriesLimit     int
	lastRescheduleRetryID        uuid.UUID
	lastRescheduleRetryDelay     time.Duration
	lastRecoverNow               time.Time
	lastRecoverVisibilityTimeout time.Duration
	lastRecoverRetryDelay        time.Duration
	lastRecoverLimit             int
}

func (f *fakeRepo) Create(ctx context.Context, params jobs.CreateParams) (jobs.Job, error) {
	f.mu.Lock()
	f.createCalls++
	fn := f.createFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, params)
	}
	panic("unexpected call: fakeRepo.Create")
}

func (f *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (jobs.Job, error) {
	f.mu.Lock()
	f.getByIDCalls++
	f.lastGetByIDID = id
	fn := f.getByIDFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id)
	}
	panic("unexpected call: fakeRepo.GetByID")
}

func (f *fakeRepo) MarkProcessing(ctx context.Context, id uuid.UUID) (bool, error) {
	f.mu.Lock()
	f.markProcessingCalls++
	f.lastMarkProcessingID = id
	fn := f.markProcessingFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id)
	}
	panic("unexpected call: fakeRepo.MarkProcessing")
}

func (f *fakeRepo) MarkCompleted(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
	f.mu.Lock()
	f.markCompletedCalls++
	f.lastMarkCompletedID = id
	f.lastMarkCompletedRes = append(json.RawMessage(nil), result...)
	fn := f.markCompletedFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, result)
	}
	panic("unexpected call: fakeRepo.MarkCompleted")
}

func (f *fakeRepo) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) (bool, error) {
	f.mu.Lock()
	f.markFailedCalls++
	f.lastMarkFailedID = id
	f.lastMarkFailedErr = errMsg
	fn := f.markFailedFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, errMsg)
	}
	panic("unexpected call: fakeRepo.MarkFailed")
}

func (f *fakeRepo) HandleProcessingFailure(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
	f.mu.Lock()
	f.handleProcessingFailureCalls++
	f.lastHandleFailureID = id
	f.lastHandleFailureErr = errMsg
	f.lastHandleFailureDelay = retryDelay
	fn := f.handleProcessingFailureFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, errMsg, retryDelay)
	}
	panic("unexpected call: fakeRepo.HandleProcessingFailure")
}

func (f *fakeRepo) ClaimDueRetries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	f.mu.Lock()
	f.claimDueRetriesCalls++
	f.lastClaimDueRetriesNow = now
	f.lastClaimDueRetriesLimit = limit
	fn := f.claimDueRetriesFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, now, limit)
	}
	return nil, nil
}

func (f *fakeRepo) RescheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error) {
	f.mu.Lock()
	f.rescheduleRetryCalls++
	f.lastRescheduleRetryID = id
	f.lastRescheduleRetryDelay = delay
	fn := f.rescheduleRetryFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, id, delay)
	}
	return false, nil
}

func (f *fakeRepo) RecoverStaleProcessing(ctx context.Context, now time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error) {
	f.mu.Lock()
	f.recoverStaleProcessingCalls++
	f.lastRecoverNow = now
	f.lastRecoverVisibilityTimeout = visibilityTimeout
	f.lastRecoverRetryDelay = retryDelay
	f.lastRecoverLimit = limit
	fn := f.recoverStaleProcessingFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, now, visibilityTimeout, retryDelay, limit)
	}
	return nil, nil
}

type fakeQueue struct {
	mu sync.Mutex

	enqueueFn func(ctx context.Context, msg queue.Message) error
	dequeueFn func(ctx context.Context) (queue.Message, error)

	enqueueCalls int
	dequeueCalls int

	lastEnqueueMsg queue.Message
	enqueueMsgs    []queue.Message
}

func (f *fakeQueue) Enqueue(ctx context.Context, msg queue.Message) error {
	f.mu.Lock()
	f.enqueueCalls++
	f.lastEnqueueMsg = msg
	f.enqueueMsgs = append(f.enqueueMsgs, msg)
	fn := f.enqueueFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, msg)
	}
	panic("unexpected call: fakeQueue.Enqueue")
}

func (f *fakeQueue) Dequeue(ctx context.Context) (queue.Message, error) {
	f.mu.Lock()
	f.dequeueCalls++
	fn := f.dequeueFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	panic("unexpected call: fakeQueue.Dequeue")
}

type fakeProcessor struct {
	mu sync.Mutex

	processFn func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error)

	processCalls int
	lastJobID    uuid.UUID
}

func (f *fakeProcessor) Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
	f.mu.Lock()
	f.processCalls++
	f.lastJobID = jobID
	fn := f.processFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, jobID)
	}
	panic("unexpected call: fakeProcessor.Process")
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestWorker(repo jobs.Repository, q queue.Queue, processor Processor) *Worker {
	return NewWorker(repo, q, processor, newTestLogger())
}

func TestHandleMessage_MarkProcessingFalse_SkipProcessor(t *testing.T) {
	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, nil
		},
	}
	q := &fakeQueue{}
	processor := &fakeProcessor{}

	worker := newTestWorker(repo, q, processor)

	msg := queue.Message{JobID: uuid.New()}
	if err := worker.handleMessage(context.Background(), msg, worker.logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if processor.processCalls != 0 {
		t.Fatalf("expected processor.Process to not be called, but it was called %d times", processor.processCalls)
	}
	if repo.markProcessingCalls != 1 {
		t.Fatalf("expected repo.MarkProcessing to be called once, but it was called %d times", repo.markProcessingCalls)
	}
	if repo.lastMarkProcessingID != msg.JobID {
		t.Fatalf("expected repo.MarkProcessing to be called with job ID %s, but it was called with %s", msg.JobID, repo.lastMarkProcessingID)
	}
	if repo.markCompletedCalls != 0 {
		t.Fatalf("expected repo.MarkCompleted to not be called, but it was called %d times", repo.markCompletedCalls)
	}
	if repo.handleProcessingFailureCalls != 0 {
		t.Fatalf("expected repo.HandleProcessingFailure to not be called, but it was called %d times", repo.handleProcessingFailureCalls)
	}
}

func TestHandleMessage_LogsClaimTransitionOutcomesWithWorkerSlot(t *testing.T) {
	jobID := uuid.New()

	var skipped bytes.Buffer
	skipRepo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, nil
		},
	}
	skipWorker := NewWorker(
		skipRepo,
		&fakeQueue{},
		&fakeProcessor{},
		slog.New(slog.NewTextHandler(&skipped, nil)),
	)

	if err := skipWorker.handleMessage(context.Background(), queue.Message{JobID: jobID}, skipWorker.logger.With("worker_slot", 2)); err != nil {
		t.Fatalf("unexpected skip error: %v", err)
	}

	skipLog := skipped.String()
	for _, want := range []string{
		"job_id=" + jobID.String(),
		"worker_slot=2",
		"transition=pending_to_processing",
		"transition_applied=false",
		"transition_outcome=skipped",
	} {
		if !strings.Contains(skipLog, want) {
			t.Fatalf("expected skip log to contain %q, got %q", want, skipLog)
		}
	}

	var claimed bytes.Buffer
	claimRepo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}
	claimProcessor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	}
	claimWorker := NewWorker(
		claimRepo,
		&fakeQueue{},
		claimProcessor,
		slog.New(slog.NewTextHandler(&claimed, nil)),
	)

	if err := claimWorker.handleMessage(context.Background(), queue.Message{JobID: jobID}, claimWorker.logger.With("worker_slot", 1)); err != nil {
		t.Fatalf("unexpected claim error: %v", err)
	}

	claimLog := claimed.String()
	for _, want := range []string{
		"job_id=" + jobID.String(),
		"worker_slot=1",
		"transition=pending_to_processing",
		"transition_applied=true",
		"transition_outcome=claimed",
	} {
		if !strings.Contains(claimLog, want) {
			t.Fatalf("expected claim log to contain %q, got %q", want, claimLog)
		}
	}
}

func TestProcessJob_Success_MarksCompleted(t *testing.T) {
	expectedResult := json.RawMessage(`{"ok":true}`)
	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			if string(result) != string(expectedResult) {
				t.Fatalf("unexpected result: %s", string(result))
			}
			return true, nil
		},
	}
	q := &fakeQueue{}
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return expectedResult, nil
		},
	}

	worker := newTestWorker(repo, q, processor)

	jobID := uuid.New()
	if err := worker.processJob(context.Background(), jobID, worker.logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if processor.processCalls != 1 {
		t.Fatalf("expected processor.Process to be called once, but it was called %d times", processor.processCalls)
	}
	if repo.markCompletedCalls != 1 {
		t.Fatalf("expected repo.MarkCompleted to be called once, but it was called %d times", repo.markCompletedCalls)
	}
	if repo.handleProcessingFailureCalls != 0 {
		t.Fatalf("expected repo.HandleProcessingFailure to not be called, but it was called %d times", repo.handleProcessingFailureCalls)
	}
}

func TestProcessJob_ProcessorError_HandlesFailureTransitionRetry(t *testing.T) {
	processErr := errors.New("processor failed")
	repo := &fakeRepo{
		handleProcessingFailureFn: func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
			if errMsg != processErr.Error() {
				t.Fatalf("unexpected error message: %q", errMsg)
			}
			if retryDelay != defaultProcessingFailureRetryDelay {
				t.Fatalf("unexpected retry delay: got %s want %s", retryDelay, defaultProcessingFailureRetryDelay)
			}
			nextRunAt := time.Now().Add(retryDelay)
			return jobs.FailureTransitionResult{
				Applied:     true,
				Decision:    jobs.FailureDecisionRetry,
				Attempt:     1,
				MaxAttempts: 3,
				NextRunAt:   &nextRunAt,
			}, nil
		},
	}
	q := &fakeQueue{}
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return nil, processErr
		},
	}

	worker := newTestWorker(repo, q, processor)

	jobID := uuid.New()
	err := worker.processJob(context.Background(), jobID, worker.logger)
	if !errors.Is(err, processErr) {
		t.Fatalf("expected error %v, got %v", processErr, err)
	}

	if repo.handleProcessingFailureCalls != 1 {
		t.Fatalf("expected repo.HandleProcessingFailure to be called once, but it was called %d times", repo.handleProcessingFailureCalls)
	}
	if repo.lastHandleFailureID != jobID {
		t.Fatalf("expected HandleProcessingFailure to be called with job ID %s, but got %s", jobID, repo.lastHandleFailureID)
	}
	if repo.lastHandleFailureErr != processErr.Error() {
		t.Fatalf("expected HandleProcessingFailure error %q, got %q", processErr.Error(), repo.lastHandleFailureErr)
	}
	if repo.lastHandleFailureDelay != defaultProcessingFailureRetryDelay {
		t.Fatalf("expected HandleProcessingFailure retry delay %s, got %s", defaultProcessingFailureRetryDelay, repo.lastHandleFailureDelay)
	}
	if repo.markCompletedCalls != 0 {
		t.Fatalf("expected repo.MarkCompleted to not be called, but it was called %d times", repo.markCompletedCalls)
	}
}

func TestProcessJob_ProcessorError_HandlesFailureTransitionTerminal(t *testing.T) {
	processErr := errors.New("processor failed")
	repo := &fakeRepo{
		handleProcessingFailureFn: func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
			return jobs.FailureTransitionResult{
				Applied:     true,
				Decision:    jobs.FailureDecisionTerminal,
				Attempt:     3,
				MaxAttempts: 3,
			}, nil
		},
	}
	q := &fakeQueue{}
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return nil, processErr
		},
	}

	worker := newTestWorker(repo, q, processor)

	jobID := uuid.New()
	err := worker.processJob(context.Background(), jobID, worker.logger)
	if !errors.Is(err, processErr) {
		t.Fatalf("expected error %v, got %v", processErr, err)
	}
	if repo.handleProcessingFailureCalls != 1 {
		t.Fatalf("expected repo.HandleProcessingFailure to be called once, but it was called %d times", repo.handleProcessingFailureCalls)
	}
	if repo.markCompletedCalls != 0 {
		t.Fatalf("expected repo.MarkCompleted to not be called, but it was called %d times", repo.markCompletedCalls)
	}
}

func TestProcessJob_ProcessorError_TransitionAlreadyApplied(t *testing.T) {
	processErr := errors.New("processor failed")
	repo := &fakeRepo{
		handleProcessingFailureFn: func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
			return jobs.FailureTransitionResult{Applied: false}, nil
		},
	}
	q := &fakeQueue{}
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return nil, processErr
		},
	}

	worker := newTestWorker(repo, q, processor)

	jobID := uuid.New()
	err := worker.processJob(context.Background(), jobID, worker.logger)
	if !errors.Is(err, processErr) {
		t.Fatalf("expected error %v, got %v", processErr, err)
	}
	if repo.handleProcessingFailureCalls != 1 {
		t.Fatalf("expected repo.HandleProcessingFailure to be called once, but it was called %d times", repo.handleProcessingFailureCalls)
	}
	if repo.markCompletedCalls != 0 {
		t.Fatalf("expected repo.MarkCompleted to not be called, but it was called %d times", repo.markCompletedCalls)
	}
}

func TestRun_DequeueEmpty_StopsOnContextCancel(t *testing.T) {
	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			select {
			case <-ctx.Done():
				return queue.Message{}, ctx.Err()
			default:
				return queue.Message{}, queue.ErrEmpty
			}
		},
	}
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		worker.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after context cancellation")
	}

	if q.dequeueCalls == 0 {
		t.Fatal("expected queue.Dequeue to be called at least once")
	}
}

func TestRun_UsesBoundedWorkerPoolConcurrency(t *testing.T) {
	jobIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}

	var dequeueMu sync.Mutex
	next := 0
	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) { return true, nil },
		markCompletedFn:  func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) { return true, nil },
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			dequeueMu.Lock()
			defer dequeueMu.Unlock()
			if next < len(jobIDs) {
				msg := queue.Message{JobID: jobIDs[next]}
				next++
				return msg, nil
			}
			select {
			case <-ctx.Done():
				return queue.Message{}, ctx.Err()
			default:
				return queue.Message{}, queue.ErrEmpty
			}
		},
	}

	release := make(chan struct{})
	started := make(chan struct{}, len(jobIDs))
	var active int32
	var maxActive int32
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			cur := atomic.AddInt32(&active, 1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
					break
				}
			}
			started <- struct{}{}
			<-release
			atomic.AddInt32(&active, -1)
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	worker.concurrency = 2
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		worker.Run(ctx)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected two jobs to start processing concurrently")
		}
	}

	select {
	case <-started:
		t.Fatal("expected third job to wait for a free worker")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after context cancellation")
	}

	if got := atomic.LoadInt32(&maxActive); got > 2 {
		t.Fatalf("expected max active processors <= 2, got %d", got)
	}
}

func TestRun_DuplicateDeliveryConcurrentOnlyOneProcessingClaimWins(t *testing.T) {
	jobID := uuid.New()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	claimEntered := make(chan struct{}, 2)
	releaseClaims := make(chan struct{})
	var claimed atomic.Bool

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			if id != jobID {
				return false, errors.New("unexpected job ID")
			}
			claimEntered <- struct{}{}
			<-releaseClaims
			return claimed.CompareAndSwap(false, true), nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}

	var dequeued atomic.Int32
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			if dequeued.Add(1) <= 2 {
				return queue.Message{JobID: jobID}, nil
			}
			<-ctx.Done()
			return queue.Message{}, ctx.Err()
		},
	}

	processed := make(chan struct{}, 1)
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			processed <- struct{}{}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	if err := worker.SetConcurrency(2); err != nil {
		t.Fatalf("unexpected concurrency error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-claimEntered:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected duplicate deliveries to race for processing claim")
		}
	}

	close(releaseClaims)

	select {
	case <-processed:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected exactly one duplicate delivery to process")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after duplicate-delivery test cancellation")
	}

	if repo.markProcessingCalls != 2 {
		t.Fatalf("expected two processing claim attempts, got %d", repo.markProcessingCalls)
	}
	if processor.processCalls != 1 {
		t.Fatalf("expected one processor call, got %d", processor.processCalls)
	}
	if repo.markCompletedCalls != 1 {
		t.Fatalf("expected one completed transition, got %d", repo.markCompletedCalls)
	}
}

func TestRun_ActiveProcessingNeverExceedsConfiguredConcurrency(t *testing.T) {
	const configuredConcurrency = 3
	jobIDs := make([]uuid.UUID, 9)
	for i := range jobIDs {
		jobIDs[i] = uuid.New()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var dequeueMu sync.Mutex
	nextJob := 0
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			dequeueMu.Lock()
			defer dequeueMu.Unlock()
			if nextJob < len(jobIDs) {
				msg := queue.Message{JobID: jobIDs[nextJob]}
				nextJob++
				return msg, nil
			}
			<-ctx.Done()
			return queue.Message{}, ctx.Err()
		},
	}

	completed := make(chan struct{}, len(jobIDs))
	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			completed <- struct{}{}
			return true, nil
		},
	}

	release := make(chan struct{})
	started := make(chan struct{}, len(jobIDs))
	var active int32
	var maxActive int32
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
			cur := atomic.AddInt32(&active, 1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
					break
				}
			}
			started <- struct{}{}
			<-release
			atomic.AddInt32(&active, -1)
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	if err := worker.SetConcurrency(configuredConcurrency); err != nil {
		t.Fatalf("unexpected concurrency error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	for i := 0; i < configuredConcurrency; i++ {
		select {
		case <-started:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected %d jobs to start", configuredConcurrency)
		}
	}

	select {
	case <-started:
		t.Fatalf("expected active processing to be capped at %d", configuredConcurrency)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	for i := 0; i < len(jobIDs); i++ {
		select {
		case <-completed:
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("expected job %d to complete", i+1)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after concurrency-cap test cancellation")
	}

	if got := atomic.LoadInt32(&maxActive); got > configuredConcurrency {
		t.Fatalf("expected max active processors <= %d, got %d", configuredConcurrency, got)
	}
}

func TestRun_StartsRetryDispatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		claimDueRetriesFn: func(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
			cancel()
			return nil, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			<-ctx.Done()
			return queue.Message{}, ctx.Err()
		},
	}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after context cancellation")
	}

	if repo.claimDueRetriesCalls == 0 {
		t.Fatal("expected retry dispatcher to run when worker starts")
	}
}

func TestRun_StartsProcessingRecoveryScanner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		recoverStaleProcessingFn: func(ctx context.Context, now time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error) {
			cancel()
			return nil, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			<-ctx.Done()
			return queue.Message{}, ctx.Err()
		},
	}
	worker := newTestWorker(repo, q, &fakeProcessor{})

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after recovery scanner canceled context")
	}

	if repo.recoverStaleProcessingCalls == 0 {
		t.Fatal("expected processing recovery scanner to run when worker starts")
	}
}

func TestRun_CancelStopsAcceptingNewWork(t *testing.T) {
	jobID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}

	var dequeueCalls atomic.Int32
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			if dequeueCalls.Add(1) == 1 {
				return queue.Message{JobID: jobID}, nil
			}
			<-ctx.Done()
			return queue.Message{}, ctx.Err()
		},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var processCalls atomic.Int32
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			processCalls.Add(1)
			close(started)
			<-release
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected in-flight job to start")
	}

	cancel()

	select {
	case <-done:
		t.Fatal("worker.Run returned before in-flight job drained")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after in-flight job completed")
	}

	if got := processCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one processed job after cancellation, got %d", got)
	}
}

func TestRun_CancelStopsIntakeWhileAllowingInFlightCompletion(t *testing.T) {
	firstJobID := uuid.New()
	secondJobID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}

	secondDequeued := make(chan struct{})
	var dequeueCalls atomic.Int32
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			switch dequeueCalls.Add(1) {
			case 1:
				return queue.Message{JobID: firstJobID}, nil
			case 2:
				close(secondDequeued)
				return queue.Message{JobID: secondJobID}, nil
			default:
				<-ctx.Done()
				return queue.Message{}, ctx.Err()
			}
		},
	}

	started := make(chan uuid.UUID, 1)
	release := make(chan struct{})
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			started <- id
			<-release
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	if err := worker.SetConcurrency(1); err != nil {
		t.Fatalf("unexpected concurrency error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case id := <-started:
		if id != firstJobID {
			t.Fatalf("expected first job to start, got %s", id)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected first in-flight job to start")
	}

	select {
	case <-secondDequeued:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected second message to be dequeued before cancellation")
	}

	cancel()

	select {
	case <-done:
		t.Fatal("worker.Run returned before in-flight job completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after in-flight job completed")
	}

	if processor.processCalls != 1 {
		t.Fatalf("expected cancellation to prevent second job processing, got %d processor calls", processor.processCalls)
	}
	if repo.markCompletedCalls != 1 {
		t.Fatalf("expected only the in-flight job to complete, got %d completed transitions", repo.markCompletedCalls)
	}
}

func TestRun_CancelDrainsInFlightJobsWithoutCancelingJobContext(t *testing.T) {
	jobID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			return queue.Message{JobID: jobID}, nil
		},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			close(started)
			<-release
			if err := ctx.Err(); err != nil {
				t.Fatalf("expected in-flight job context to remain active during drain, got %v", err)
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	worker.shutdownTimeout = time.Second
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected in-flight job to start")
	}

	cancel()

	select {
	case <-done:
		t.Fatal("worker.Run returned before in-flight job completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not return after in-flight job completed")
	}
}

func TestRun_ShutdownTimeoutCancelsInFlightJobContext(t *testing.T) {
	jobID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		handleProcessingFailureFn: func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
			return jobs.FailureTransitionResult{
				Applied:     true,
				Decision:    jobs.FailureDecisionTerminal,
				Attempt:     1,
				MaxAttempts: 1,
			}, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			return queue.Message{JobID: jobID}, nil
		},
	}

	started := make(chan struct{})
	contextCanceled := make(chan error, 1)
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			contextCanceled <- ctx.Err()
			return nil, ctx.Err()
		},
	}

	worker := newTestWorker(repo, q, processor)
	worker.shutdownTimeout = 25 * time.Millisecond
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected in-flight job to start")
	}

	cancel()

	select {
	case err := <-contextCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected in-flight job context cancellation, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected shutdown timeout to cancel in-flight job context")
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not return after shutdown timeout canceled in-flight job")
	}
}

func TestRun_ShutdownTimeoutReturnsWhenInFlightJobIgnoresContext(t *testing.T) {
	jobID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		markProcessingFn: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return true, nil
		},
		markCompletedFn: func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
			return true, nil
		},
	}
	q := &fakeQueue{
		dequeueFn: func(ctx context.Context) (queue.Message, error) {
			return queue.Message{JobID: jobID}, nil
		},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	processor := &fakeProcessor{
		processFn: func(ctx context.Context, id uuid.UUID) (json.RawMessage, error) {
			close(started)
			<-release
			return json.RawMessage(`{"ok":true}`), nil
		},
	}

	worker := newTestWorker(repo, q, processor)
	worker.shutdownTimeout = 25 * time.Millisecond
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected in-flight job to start")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not return after shutdown timeout")
	}

	close(release)
}

func TestDispatchDueRetries_ClaimsAndEnqueues(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	now := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)

	repo := &fakeRepo{
		claimDueRetriesFn: func(ctx context.Context, claimNow time.Time, limit int) ([]uuid.UUID, error) {
			if !claimNow.Equal(now) {
				t.Fatalf("unexpected claim time: got %v want %v", claimNow, now)
			}
			if limit != defaultRetryDispatchBatchSize {
				t.Fatalf("unexpected claim limit: got %d want %d", limit, defaultRetryDispatchBatchSize)
			}
			return []uuid.UUID{id1, id2}, nil
		},
	}
	q := &fakeQueue{
		enqueueFn: func(ctx context.Context, msg queue.Message) error {
			return nil
		},
	}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)

	if err := worker.dispatchDueRetries(context.Background(), now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.claimDueRetriesCalls != 1 {
		t.Fatalf("expected ClaimDueRetries to be called once, got %d", repo.claimDueRetriesCalls)
	}
	if q.enqueueCalls != 2 {
		t.Fatalf("expected Enqueue to be called twice, got %d", q.enqueueCalls)
	}
	if len(q.enqueueMsgs) != 2 {
		t.Fatalf("expected 2 enqueue messages, got %d", len(q.enqueueMsgs))
	}
	if q.enqueueMsgs[0].JobID != id1 {
		t.Fatalf("first enqueued job ID mismatch: got %s want %s", q.enqueueMsgs[0].JobID, id1)
	}
	if q.enqueueMsgs[1].JobID != id2 {
		t.Fatalf("second enqueued job ID mismatch: got %s want %s", q.enqueueMsgs[1].JobID, id2)
	}
	if repo.rescheduleRetryCalls != 0 {
		t.Fatalf("expected no reschedule calls, got %d", repo.rescheduleRetryCalls)
	}
}

func TestDispatchDueRetries_EnqueueFailure_Reschedules(t *testing.T) {
	id := uuid.New()
	now := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)

	repo := &fakeRepo{
		claimDueRetriesFn: func(ctx context.Context, claimNow time.Time, limit int) ([]uuid.UUID, error) {
			return []uuid.UUID{id}, nil
		},
		rescheduleRetryFn: func(ctx context.Context, jobID uuid.UUID, delay time.Duration) (bool, error) {
			if jobID != id {
				t.Fatalf("unexpected job ID in reschedule: got %s want %s", jobID, id)
			}
			if delay != defaultRetryReenqueueDelay {
				t.Fatalf("unexpected reschedule delay: got %s want %s", delay, defaultRetryReenqueueDelay)
			}
			return true, nil
		},
	}
	q := &fakeQueue{
		enqueueFn: func(ctx context.Context, msg queue.Message) error {
			return errors.New("redis unavailable")
		},
	}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)

	if err := worker.dispatchDueRetries(context.Background(), now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if q.enqueueCalls != 1 {
		t.Fatalf("expected Enqueue to be called once, got %d", q.enqueueCalls)
	}
	if repo.rescheduleRetryCalls != 1 {
		t.Fatalf("expected RescheduleRetry to be called once, got %d", repo.rescheduleRetryCalls)
	}
	if repo.lastRescheduleRetryID != id {
		t.Fatalf("unexpected reschedule job ID: got %s want %s", repo.lastRescheduleRetryID, id)
	}
	if repo.lastRescheduleRetryDelay != defaultRetryReenqueueDelay {
		t.Fatalf("unexpected reschedule delay: got %s want %s", repo.lastRescheduleRetryDelay, defaultRetryReenqueueDelay)
	}
}

func TestDispatchDueRetries_ClaimDueRetriesError_ReturnsErrorAndSkipsEnqueue(t *testing.T) {
	claimErr := errors.New("claim failed")
	now := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)

	repo := &fakeRepo{
		claimDueRetriesFn: func(ctx context.Context, claimNow time.Time, limit int) ([]uuid.UUID, error) {
			return nil, claimErr
		},
	}
	q := &fakeQueue{
		enqueueFn: func(ctx context.Context, msg queue.Message) error { return nil },
	}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)

	err := worker.dispatchDueRetries(context.Background(), now)
	if !errors.Is(err, claimErr) {
		t.Fatalf("expected error %v, got %v", claimErr, err)
	}
	if q.enqueueCalls != 0 {
		t.Fatalf("expected Enqueue to not be called, got %d calls", q.enqueueCalls)
	}
	if repo.rescheduleRetryCalls != 0 {
		t.Fatalf("expected RescheduleRetry to not be called, got %d calls", repo.rescheduleRetryCalls)
	}
}

func TestRunRetryDispatcher_DispatchesImmediatelyOnStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repo := &fakeRepo{
		claimDueRetriesFn: func(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
			cancel()
			return nil, nil
		},
	}
	q := &fakeQueue{
		enqueueFn: func(ctx context.Context, msg queue.Message) error { return nil },
	}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)
	worker.retryDispatchInterval = time.Hour

	done := make(chan struct{})
	go func() {
		worker.runRetryDispatcher(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runRetryDispatcher did not stop after context cancellation")
	}

	if repo.claimDueRetriesCalls == 0 {
		t.Fatal("expected ClaimDueRetries to be called immediately on start")
	}
}

func TestRecoverStaleProcessing_LogsRecoveryDecisions(t *testing.T) {
	retryID := uuid.New()
	terminalID := uuid.New()
	nextRunAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 30, 9, 55, 0, 0, time.UTC)

	repo := &fakeRepo{
		recoverStaleProcessingFn: func(ctx context.Context, recoverNow time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error) {
			if !recoverNow.Equal(now) {
				t.Fatalf("unexpected recovery time: got %v want %v", recoverNow, now)
			}
			if visibilityTimeout != defaultProcessingVisibilityTimeout {
				t.Fatalf("unexpected visibility timeout: got %s want %s", visibilityTimeout, defaultProcessingVisibilityTimeout)
			}
			if retryDelay != defaultProcessingFailureRetryDelay {
				t.Fatalf("unexpected retry delay: got %s want %s", retryDelay, defaultProcessingFailureRetryDelay)
			}
			if limit != defaultProcessingRecoveryBatchSize {
				t.Fatalf("unexpected recovery limit: got %d want %d", limit, defaultProcessingRecoveryBatchSize)
			}
			return []jobs.RecoveryTransitionResult{
				{
					ID:          retryID,
					Decision:    jobs.RecoveryDecisionRetry,
					Attempt:     1,
					MaxAttempts: 3,
					NextRunAt:   &nextRunAt,
				},
				{
					ID:          terminalID,
					Decision:    jobs.RecoveryDecisionTerminal,
					Attempt:     3,
					MaxAttempts: 3,
				},
			}, nil
		},
	}

	var logs bytes.Buffer
	worker := NewWorker(repo, &fakeQueue{}, &fakeProcessor{}, slog.New(slog.NewTextHandler(&logs, nil)))

	if err := worker.recoverStaleProcessing(context.Background(), now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := logs.String()
	for _, want := range []string{
		"job_id=" + retryID.String(),
		"transition=processing_to_pending",
		"transition_outcome=recovered_for_retry",
		"job_id=" + terminalID.String(),
		"transition=processing_to_failed",
		"transition_outcome=terminal_failed",
		"visibility_timeout=5m0s",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected recovery log to contain %q, got %q", want, got)
		}
	}
}

func TestRunProcessingRecoveryScanner_DispatchesImmediatelyAndOnInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32
	repo := &fakeRepo{
		recoverStaleProcessingFn: func(ctx context.Context, now time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error) {
			if calls.Add(1) >= 2 {
				cancel()
			}
			return nil, nil
		},
	}
	worker := newTestWorker(repo, &fakeQueue{}, &fakeProcessor{})
	worker.processingRecoveryInterval = 10 * time.Millisecond

	done := make(chan struct{})
	go func() {
		worker.runProcessingRecoveryScanner(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recovery scanner did not stop after context cancellation")
	}

	if got := calls.Load(); got < 2 {
		t.Fatalf("expected at least two recovery scans, got %d", got)
	}
}

func TestRunProcessingRecoveryScanner_LogsErrorsAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	recoveryErr := errors.New("recover failed")

	repo := &fakeRepo{
		recoverStaleProcessingFn: func(ctx context.Context, now time.Time, visibilityTimeout time.Duration, retryDelay time.Duration, limit int) ([]jobs.RecoveryTransitionResult, error) {
			cancel()
			return nil, recoveryErr
		},
	}
	var logs bytes.Buffer
	worker := NewWorker(repo, &fakeQueue{}, &fakeProcessor{}, slog.New(slog.NewTextHandler(&logs, nil)))
	worker.processingRecoveryInterval = time.Hour

	done := make(chan struct{})
	go func() {
		worker.runProcessingRecoveryScanner(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recovery scanner did not stop after cancellation")
	}

	if got := logs.String(); !strings.Contains(got, "failed to recover stale processing jobs") || !strings.Contains(got, recoveryErr.Error()) {
		t.Fatalf("expected recovery error log, got %q", got)
	}
}

func TestDeterministicProcessor_ContextCanceled(t *testing.T) {
	p := &DeterministicProcessor{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Process(ctx, uuid.New())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDeterministicProcessor_FailJobID(t *testing.T) {
	jobID := uuid.New()
	p := &DeterministicProcessor{FailJobID: jobID.String()}

	_, err := p.Process(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected injected processor failure, got nil")
	}

	if got := err.Error(); got != "injected processor failure for UAT" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestSetRetryRuntimeConfig_AppliesValues(t *testing.T) {
	repo := &fakeRepo{}
	q := &fakeQueue{}
	processor := &fakeProcessor{}
	worker := newTestWorker(repo, q, processor)

	cfg := RetryRuntimeConfig{
		RetryDelay:        45 * time.Second,
		DispatchInterval:  20 * time.Second,
		DispatchBatchSize: 12,
		ReenqueueDelay:    15 * time.Second,
	}

	if err := worker.SetRetryRuntimeConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if worker.processingFailureRetryDelay != cfg.RetryDelay {
		t.Fatalf("unexpected processing retry delay: got %s want %s", worker.processingFailureRetryDelay, cfg.RetryDelay)
	}
	if worker.retryDispatchInterval != cfg.DispatchInterval {
		t.Fatalf("unexpected dispatch interval: got %s want %s", worker.retryDispatchInterval, cfg.DispatchInterval)
	}
	if worker.retryDispatchBatchSize != cfg.DispatchBatchSize {
		t.Fatalf("unexpected dispatch batch size: got %d want %d", worker.retryDispatchBatchSize, cfg.DispatchBatchSize)
	}
	if worker.retryReenqueueDelay != cfg.ReenqueueDelay {
		t.Fatalf("unexpected reenqueue delay: got %s want %s", worker.retryReenqueueDelay, cfg.ReenqueueDelay)
	}
}

func TestSetRetryRuntimeConfig_InvalidValues(t *testing.T) {
	repo := &fakeRepo{}
	q := &fakeQueue{}
	processor := &fakeProcessor{}

	testCases := []struct {
		name string
		cfg  RetryRuntimeConfig
	}{
		{
			name: "retry delay <= 0",
			cfg: RetryRuntimeConfig{
				RetryDelay:        0,
				DispatchInterval:  time.Second,
				DispatchBatchSize: 1,
				ReenqueueDelay:    time.Second,
			},
		},
		{
			name: "dispatch interval <= 0",
			cfg: RetryRuntimeConfig{
				RetryDelay:        time.Second,
				DispatchInterval:  0,
				DispatchBatchSize: 1,
				ReenqueueDelay:    time.Second,
			},
		},
		{
			name: "dispatch batch size <= 0",
			cfg: RetryRuntimeConfig{
				RetryDelay:        time.Second,
				DispatchInterval:  time.Second,
				DispatchBatchSize: 0,
				ReenqueueDelay:    time.Second,
			},
		},
		{
			name: "reenqueue delay <= 0",
			cfg: RetryRuntimeConfig{
				RetryDelay:        time.Second,
				DispatchInterval:  time.Second,
				DispatchBatchSize: 1,
				ReenqueueDelay:    0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			worker := newTestWorker(repo, q, processor)
			if err := worker.SetRetryRuntimeConfig(tc.cfg); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestSetProcessingRecoveryRuntimeConfig_AppliesValues(t *testing.T) {
	worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})
	cfg := ProcessingRecoveryRuntimeConfig{
		VisibilityTimeout: 2 * time.Minute,
		Interval:          15 * time.Second,
		BatchSize:         7,
		RetryDelay:        3 * time.Second,
	}

	if err := worker.SetProcessingRecoveryRuntimeConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if worker.processingVisibilityTimeout != cfg.VisibilityTimeout {
		t.Fatalf("unexpected visibility timeout: got %s want %s", worker.processingVisibilityTimeout, cfg.VisibilityTimeout)
	}
	if worker.processingRecoveryInterval != cfg.Interval {
		t.Fatalf("unexpected recovery interval: got %s want %s", worker.processingRecoveryInterval, cfg.Interval)
	}
	if worker.processingRecoveryBatchSize != cfg.BatchSize {
		t.Fatalf("unexpected recovery batch size: got %d want %d", worker.processingRecoveryBatchSize, cfg.BatchSize)
	}
	if worker.processingRecoveryRetryDelay != cfg.RetryDelay {
		t.Fatalf("unexpected recovery retry delay: got %s want %s", worker.processingRecoveryRetryDelay, cfg.RetryDelay)
	}
}

func TestSetProcessingRecoveryRuntimeConfig_InvalidValues(t *testing.T) {
	testCases := []struct {
		name string
		cfg  ProcessingRecoveryRuntimeConfig
	}{
		{
			name: "visibility timeout <= 0",
			cfg: ProcessingRecoveryRuntimeConfig{
				VisibilityTimeout: 0,
				Interval:          time.Second,
				BatchSize:         1,
				RetryDelay:        time.Second,
			},
		},
		{
			name: "interval <= 0",
			cfg: ProcessingRecoveryRuntimeConfig{
				VisibilityTimeout: time.Second,
				Interval:          0,
				BatchSize:         1,
				RetryDelay:        time.Second,
			},
		},
		{
			name: "batch size <= 0",
			cfg: ProcessingRecoveryRuntimeConfig{
				VisibilityTimeout: time.Second,
				Interval:          time.Second,
				BatchSize:         0,
				RetryDelay:        time.Second,
			},
		},
		{
			name: "retry delay <= 0",
			cfg: ProcessingRecoveryRuntimeConfig{
				VisibilityTimeout: time.Second,
				Interval:          time.Second,
				BatchSize:         1,
				RetryDelay:        0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})
			if err := worker.SetProcessingRecoveryRuntimeConfig(tc.cfg); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestSetConcurrency_AppliesValue(t *testing.T) {
	worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})

	if err := worker.SetConcurrency(3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if worker.concurrency != 3 {
		t.Fatalf("unexpected concurrency: got %d want 3", worker.concurrency)
	}
}

func TestSetConcurrency_InvalidValue(t *testing.T) {
	worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})

	if err := worker.SetConcurrency(0); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSetShutdownTimeout_AppliesValue(t *testing.T) {
	worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})
	timeout := 2 * time.Second

	if err := worker.SetShutdownTimeout(timeout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if worker.shutdownTimeout != timeout {
		t.Fatalf("unexpected shutdown timeout: got %s want %s", worker.shutdownTimeout, timeout)
	}
}

func TestSetShutdownTimeout_InvalidValue(t *testing.T) {
	worker := newTestWorker(&fakeRepo{}, &fakeQueue{}, &fakeProcessor{})

	if err := worker.SetShutdownTimeout(0); err == nil {
		t.Fatal("expected error, got nil")
	}
}
