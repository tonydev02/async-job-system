package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type fakeRepo struct {
	createFn                  func(ctx context.Context, params jobs.CreateParams) (jobs.Job, error)
	getByIDFn                 func(ctx context.Context, id uuid.UUID) (jobs.Job, error)
	markProcessingFn          func(ctx context.Context, id uuid.UUID) (bool, error)
	markCompletedFn           func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error)
	markFailedFn              func(ctx context.Context, id uuid.UUID, errMsg string) (bool, error)
	handleProcessingFailureFn func(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error)
	claimDueRetriesFn         func(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	rescheduleRetryFn         func(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error)

	createCalls                  int
	getByIDCalls                 int
	markProcessingCalls          int
	markCompletedCalls           int
	markFailedCalls              int
	handleProcessingFailureCalls int
	claimDueRetriesCalls         int
	rescheduleRetryCalls         int

	lastGetByIDID            uuid.UUID
	lastMarkProcessingID     uuid.UUID
	lastMarkCompletedID      uuid.UUID
	lastMarkCompletedRes     json.RawMessage
	lastMarkFailedID         uuid.UUID
	lastMarkFailedErr        string
	lastHandleFailureID      uuid.UUID
	lastHandleFailureErr     string
	lastHandleFailureDelay   time.Duration
	lastClaimDueRetriesNow   time.Time
	lastClaimDueRetriesLimit int
	lastRescheduleRetryID    uuid.UUID
	lastRescheduleRetryDelay time.Duration
}

func (f *fakeRepo) Create(ctx context.Context, params jobs.CreateParams) (jobs.Job, error) {
	f.createCalls++
	if f.createFn != nil {
		return f.createFn(ctx, params)
	}
	panic("unexpected call: fakeRepo.Create")
}

func (f *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (jobs.Job, error) {
	f.getByIDCalls++
	f.lastGetByIDID = id
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	panic("unexpected call: fakeRepo.GetByID")
}

func (f *fakeRepo) MarkProcessing(ctx context.Context, id uuid.UUID) (bool, error) {
	f.markProcessingCalls++
	f.lastMarkProcessingID = id
	if f.markProcessingFn != nil {
		return f.markProcessingFn(ctx, id)
	}
	panic("unexpected call: fakeRepo.MarkProcessing")
}

func (f *fakeRepo) MarkCompleted(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error) {
	f.markCompletedCalls++
	f.lastMarkCompletedID = id
	f.lastMarkCompletedRes = append(json.RawMessage(nil), result...)
	if f.markCompletedFn != nil {
		return f.markCompletedFn(ctx, id, result)
	}
	panic("unexpected call: fakeRepo.MarkCompleted")
}

func (f *fakeRepo) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) (bool, error) {
	f.markFailedCalls++
	f.lastMarkFailedID = id
	f.lastMarkFailedErr = errMsg
	if f.markFailedFn != nil {
		return f.markFailedFn(ctx, id, errMsg)
	}
	panic("unexpected call: fakeRepo.MarkFailed")
}

func (f *fakeRepo) HandleProcessingFailure(ctx context.Context, id uuid.UUID, errMsg string, retryDelay time.Duration) (jobs.FailureTransitionResult, error) {
	f.handleProcessingFailureCalls++
	f.lastHandleFailureID = id
	f.lastHandleFailureErr = errMsg
	f.lastHandleFailureDelay = retryDelay
	if f.handleProcessingFailureFn != nil {
		return f.handleProcessingFailureFn(ctx, id, errMsg, retryDelay)
	}
	panic("unexpected call: fakeRepo.HandleProcessingFailure")
}

func (f *fakeRepo) ClaimDueRetries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	f.claimDueRetriesCalls++
	f.lastClaimDueRetriesNow = now
	f.lastClaimDueRetriesLimit = limit
	if f.claimDueRetriesFn != nil {
		return f.claimDueRetriesFn(ctx, now, limit)
	}
	return nil, nil
}

func (f *fakeRepo) RescheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error) {
	f.rescheduleRetryCalls++
	f.lastRescheduleRetryID = id
	f.lastRescheduleRetryDelay = delay
	if f.rescheduleRetryFn != nil {
		return f.rescheduleRetryFn(ctx, id, delay)
	}
	return false, nil
}

type fakeQueue struct {
	enqueueFn func(ctx context.Context, msg queue.Message) error
	dequeueFn func(ctx context.Context) (queue.Message, error)

	enqueueCalls int
	dequeueCalls int

	lastEnqueueMsg queue.Message
	enqueueMsgs    []queue.Message
}

func (f *fakeQueue) Enqueue(ctx context.Context, msg queue.Message) error {
	f.enqueueCalls++
	f.lastEnqueueMsg = msg
	f.enqueueMsgs = append(f.enqueueMsgs, msg)
	if f.enqueueFn != nil {
		return f.enqueueFn(ctx, msg)
	}
	panic("unexpected call: fakeQueue.Enqueue")
}

func (f *fakeQueue) Dequeue(ctx context.Context) (queue.Message, error) {
	f.dequeueCalls++
	if f.dequeueFn != nil {
		return f.dequeueFn(ctx)
	}
	panic("unexpected call: fakeQueue.Dequeue")
}

type fakeProcessor struct {
	processFn func(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error)

	processCalls int
	lastJobID    uuid.UUID
}

func (f *fakeProcessor) Process(ctx context.Context, jobID uuid.UUID) (json.RawMessage, error) {
	f.processCalls++
	f.lastJobID = jobID
	if f.processFn != nil {
		return f.processFn(ctx, jobID)
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
	if err := worker.handleMessage(context.Background(), msg); err != nil {
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
	if err := worker.processJob(context.Background(), jobID); err != nil {
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
	err := worker.processJob(context.Background(), jobID)
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
	err := worker.processJob(context.Background(), jobID)
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
	err := worker.processJob(context.Background(), jobID)
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
