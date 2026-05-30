package config

import (
	"testing"
	"time"
)

func TestLoadWorkerConfig_RetryDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("WORKER_CONCURRENCY", "")
	t.Setenv("RETRY_DELAY", "")
	t.Setenv("RETRY_DISPATCH_INTERVAL", "")
	t.Setenv("RETRY_DISPATCH_BATCH_SIZE", "")
	t.Setenv("RETRY_REENQUEUE_DELAY", "")
	t.Setenv("PROCESSING_VISIBILITY_TIMEOUT", "")
	t.Setenv("PROCESSING_RECOVERY_INTERVAL", "")
	t.Setenv("PROCESSING_RECOVERY_BATCH_SIZE", "")
	t.Setenv("PROCESSING_RECOVERY_RETRY_DELAY", "")

	cfg, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RetryDelay != 30*time.Second {
		t.Fatalf("unexpected RETRY_DELAY default: got %s want %s", cfg.RetryDelay, 30*time.Second)
	}
	if cfg.RetryDispatchInterval != 1*time.Minute {
		t.Fatalf("unexpected RETRY_DISPATCH_INTERVAL default: got %s want %s", cfg.RetryDispatchInterval, time.Minute)
	}
	if cfg.RetryDispatchBatchSize != 10 {
		t.Fatalf("unexpected RETRY_DISPATCH_BATCH_SIZE default: got %d want %d", cfg.RetryDispatchBatchSize, 10)
	}
	if cfg.RetryReenqueueDelay != 1*time.Minute {
		t.Fatalf("unexpected RETRY_REENQUEUE_DELAY default: got %s want %s", cfg.RetryReenqueueDelay, time.Minute)
	}
	if cfg.WorkerConcurrency != 4 {
		t.Fatalf("unexpected WORKER_CONCURRENCY default: got %d want %d", cfg.WorkerConcurrency, 4)
	}
	if cfg.ProcessingVisibilityTimeout != 5*time.Minute {
		t.Fatalf("unexpected PROCESSING_VISIBILITY_TIMEOUT default: got %s want %s", cfg.ProcessingVisibilityTimeout, 5*time.Minute)
	}
	if cfg.ProcessingRecoveryInterval != time.Minute {
		t.Fatalf("unexpected PROCESSING_RECOVERY_INTERVAL default: got %s want %s", cfg.ProcessingRecoveryInterval, time.Minute)
	}
	if cfg.ProcessingRecoveryBatchSize != 10 {
		t.Fatalf("unexpected PROCESSING_RECOVERY_BATCH_SIZE default: got %d want %d", cfg.ProcessingRecoveryBatchSize, 10)
	}
	if cfg.ProcessingRecoveryRetryDelay != cfg.RetryDelay {
		t.Fatalf("unexpected PROCESSING_RECOVERY_RETRY_DELAY default: got %s want retry delay %s", cfg.ProcessingRecoveryRetryDelay, cfg.RetryDelay)
	}
}

func TestLoadWorkerConfig_RetryOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("WORKER_CONCURRENCY", "8")
	t.Setenv("RETRY_DELAY", "45s")
	t.Setenv("RETRY_DISPATCH_INTERVAL", "15s")
	t.Setenv("RETRY_DISPATCH_BATCH_SIZE", "25")
	t.Setenv("RETRY_REENQUEUE_DELAY", "10s")
	t.Setenv("PROCESSING_VISIBILITY_TIMEOUT", "2m")
	t.Setenv("PROCESSING_RECOVERY_INTERVAL", "20s")
	t.Setenv("PROCESSING_RECOVERY_BATCH_SIZE", "9")
	t.Setenv("PROCESSING_RECOVERY_RETRY_DELAY", "5s")

	cfg, err := LoadWorkerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RetryDelay != 45*time.Second {
		t.Fatalf("unexpected RETRY_DELAY: got %s want %s", cfg.RetryDelay, 45*time.Second)
	}
	if cfg.RetryDispatchInterval != 15*time.Second {
		t.Fatalf("unexpected RETRY_DISPATCH_INTERVAL: got %s want %s", cfg.RetryDispatchInterval, 15*time.Second)
	}
	if cfg.RetryDispatchBatchSize != 25 {
		t.Fatalf("unexpected RETRY_DISPATCH_BATCH_SIZE: got %d want %d", cfg.RetryDispatchBatchSize, 25)
	}
	if cfg.RetryReenqueueDelay != 10*time.Second {
		t.Fatalf("unexpected RETRY_REENQUEUE_DELAY: got %s want %s", cfg.RetryReenqueueDelay, 10*time.Second)
	}
	if cfg.WorkerConcurrency != 8 {
		t.Fatalf("unexpected WORKER_CONCURRENCY: got %d want %d", cfg.WorkerConcurrency, 8)
	}
	if cfg.ProcessingVisibilityTimeout != 2*time.Minute {
		t.Fatalf("unexpected PROCESSING_VISIBILITY_TIMEOUT: got %s want %s", cfg.ProcessingVisibilityTimeout, 2*time.Minute)
	}
	if cfg.ProcessingRecoveryInterval != 20*time.Second {
		t.Fatalf("unexpected PROCESSING_RECOVERY_INTERVAL: got %s want %s", cfg.ProcessingRecoveryInterval, 20*time.Second)
	}
	if cfg.ProcessingRecoveryBatchSize != 9 {
		t.Fatalf("unexpected PROCESSING_RECOVERY_BATCH_SIZE: got %d want %d", cfg.ProcessingRecoveryBatchSize, 9)
	}
	if cfg.ProcessingRecoveryRetryDelay != 5*time.Second {
		t.Fatalf("unexpected PROCESSING_RECOVERY_RETRY_DELAY: got %s want %s", cfg.ProcessingRecoveryRetryDelay, 5*time.Second)
	}
}

func TestLoadWorkerConfig_InvalidRetryDispatchBatchSize(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("RETRY_DISPATCH_BATCH_SIZE", "not-an-int")

	_, err := LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadWorkerConfig_InvalidRetryDelay(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("RETRY_DELAY", "not-a-duration")

	_, err := LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadWorkerConfig_InvalidProcessingRecoveryValues(t *testing.T) {
	testCases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "visibility timeout duration", key: "PROCESSING_VISIBILITY_TIMEOUT", value: "not-a-duration"},
		{name: "recovery interval duration", key: "PROCESSING_RECOVERY_INTERVAL", value: "not-a-duration"},
		{name: "recovery batch size int", key: "PROCESSING_RECOVERY_BATCH_SIZE", value: "not-an-int"},
		{name: "recovery retry delay duration", key: "PROCESSING_RECOVERY_RETRY_DELAY", value: "not-a-duration"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
			t.Setenv(tc.key, tc.value)

			_, err := LoadWorkerConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLoadWorkerConfig_NonPositiveRetryValues(t *testing.T) {
	testCases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "retry delay zero", key: "RETRY_DELAY", value: "0s"},
		{name: "retry dispatch interval negative", key: "RETRY_DISPATCH_INTERVAL", value: "-1s"},
		{name: "retry dispatch batch size zero", key: "RETRY_DISPATCH_BATCH_SIZE", value: "0"},
		{name: "retry reenqueue delay zero", key: "RETRY_REENQUEUE_DELAY", value: "0s"},
		{name: "processing visibility timeout zero", key: "PROCESSING_VISIBILITY_TIMEOUT", value: "0s"},
		{name: "processing recovery interval negative", key: "PROCESSING_RECOVERY_INTERVAL", value: "-1s"},
		{name: "processing recovery batch size zero", key: "PROCESSING_RECOVERY_BATCH_SIZE", value: "0"},
		{name: "processing recovery retry delay zero", key: "PROCESSING_RECOVERY_RETRY_DELAY", value: "0s"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
			t.Setenv(tc.key, tc.value)

			_, err := LoadWorkerConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLoadWorkerConfig_InvalidWorkerConcurrency(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("WORKER_CONCURRENCY", "not-an-int")

	_, err := LoadWorkerConfig()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadWorkerConfig_NonPositiveWorkerConcurrency(t *testing.T) {
	testCases := []struct {
		name  string
		value string
	}{
		{name: "worker concurrency zero", value: "0"},
		{name: "worker concurrency negative", value: "-1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
			t.Setenv("WORKER_CONCURRENCY", tc.value)

			_, err := LoadWorkerConfig()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
