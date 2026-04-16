package config

import (
	"testing"
	"time"
)

func TestLoadWorkerConfig_RetryDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("RETRY_DELAY", "")
	t.Setenv("RETRY_DISPATCH_INTERVAL", "")
	t.Setenv("RETRY_DISPATCH_BATCH_SIZE", "")
	t.Setenv("RETRY_REENQUEUE_DELAY", "")

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
}

func TestLoadWorkerConfig_RetryOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/async_jobs?sslmode=disable")
	t.Setenv("RETRY_DELAY", "45s")
	t.Setenv("RETRY_DISPATCH_INTERVAL", "15s")
	t.Setenv("RETRY_DISPATCH_BATCH_SIZE", "25")
	t.Setenv("RETRY_REENQUEUE_DELAY", "10s")

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
