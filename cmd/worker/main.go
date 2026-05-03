package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/namta/async-job-system/internal/config"
	"github.com/namta/async-job-system/internal/jobs/postgres"
	redisqueue "github.com/namta/async-job-system/internal/queue/redis"
	"github.com/namta/async-job-system/internal/worker"
)

func main() {
	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load worker config: %v\n", err)
		os.Exit(1)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	done := make(chan error, 1)
	go func() {
		done <- run(rootCtx, cfg) // run with rootCtx, no startup lifetime cap
	}()

	select {
	case err := <-done:
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker failed: %v\n", err)
			os.Exit(1)
		}
	case <-rootCtx.Done():
		// signal received, now apply bounded shutdown wait
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		select {
		case err := <-done:
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker failed during shutdown: %v\n", err)
				os.Exit(1)
			}
		case <-shutdownCtx.Done():
			fmt.Fprintf(os.Stderr, "forced shutdown after timeout: %v\n", cfg.ShutdownTimeout)
			os.Exit(1)
		}
	}
}

func run(ctx context.Context, cfg config.WorkerConfig) error {
	logger := newLogger(cfg.LogLevel)

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(startupCtx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	redisClient, err := redisqueue.NewRedisClient(startupCtx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return fmt.Errorf("init redis: %w", err)
	}
	defer redisClient.Close()

	repo := postgres.NewRepository(db)
	q := redisqueue.NewQueue(redisClient, cfg.RedisQueueKey, cfg.RedisBlockTimeout)
	processor := &worker.DeterministicProcessor{FailJobID: cfg.ProcessorFailJobID}
	workerLogger := logger.With("worker_concurrency", cfg.WorkerConcurrency)
	w := worker.NewWorker(repo, q, processor, workerLogger)
	if err := w.SetRetryRuntimeConfig(worker.RetryRuntimeConfig{
		RetryDelay:        cfg.RetryDelay,
		DispatchInterval:  cfg.RetryDispatchInterval,
		DispatchBatchSize: cfg.RetryDispatchBatchSize,
		ReenqueueDelay:    cfg.RetryReenqueueDelay,
	}); err != nil {
		return fmt.Errorf("configure worker retry runtime: %w", err)
	}

	w.Run(ctx)
	return nil
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
