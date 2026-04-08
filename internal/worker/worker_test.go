package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/namta/async-job-system/internal/jobs"
	"github.com/namta/async-job-system/internal/queue"
)

type fakeRepo struct {
	createFn         func(ctx context.Context, params jobs.CreateParams) (jobs.Job, error)
	getByIDFn        func(ctx context.Context, id uuid.UUID) (jobs.Job, error)
	markProcessingFn func(ctx context.Context, id uuid.UUID) (bool, error)
	markCompletedFn  func(ctx context.Context, id uuid.UUID, result json.RawMessage) (bool, error)
	markFailedFn     func(ctx context.Context, id uuid.UUID, errMsg string) (bool, error)

	createCalls         int
	getByIDCalls        int
	markProcessingCalls int
	markCompletedCalls  int
	markFailedCalls     int

	lastGetByIDID        uuid.UUID
	lastMarkProcessingID uuid.UUID
	lastMarkCompletedID  uuid.UUID
	lastMarkCompletedRes json.RawMessage
	lastMarkFailedID     uuid.UUID
	lastMarkFailedErr    string
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
	return jobs.FailureTransitionResult{}, nil
}

func (f *fakeRepo) ClaimDueRetries(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	return nil, nil
}

func (f *fakeRepo) RescheduleRetry(ctx context.Context, id uuid.UUID, delay time.Duration) (bool, error) {
	return false, nil
}

type fakeQueue struct {
	enqueueFn func(ctx context.Context, msg queue.Message) error
	dequeueFn func(ctx context.Context) (queue.Message, error)

	enqueueCalls int
	dequeueCalls int

	lastEnqueueMsg queue.Message
}

func (f *fakeQueue) Enqueue(ctx context.Context, msg queue.Message) error {
	f.enqueueCalls++
	f.lastEnqueueMsg = msg
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
	if repo.markFailedCalls != 0 {
		t.Fatalf("expected repo.MarkFailed to not be called, but it was called %d times", repo.markFailedCalls)
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
	if repo.markFailedCalls != 0 {
		t.Fatalf("expected repo.MarkFailed to not be called, but it was called %d times", repo.markFailedCalls)
	}
}

func TestProcessJob_ProcessorError_MarksFailed(t *testing.T) {
	processErr := errors.New("processor failed")
	repo := &fakeRepo{
		markFailedFn: func(ctx context.Context, id uuid.UUID, errMsg string) (bool, error) {
			if errMsg != processErr.Error() {
				t.Fatalf("unexpected error message: %q", errMsg)
			}
			return true, nil
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

	if repo.markFailedCalls != 1 {
		t.Fatalf("expected repo.MarkFailed to be called once, but it was called %d times", repo.markFailedCalls)
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
