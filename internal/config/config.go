package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type WorkerConfig struct {
	DatabaseURL            string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	RedisQueueKey          string
	RedisBlockTimeout      time.Duration
	RetryDelay             time.Duration
	RetryDispatchInterval  time.Duration
	RetryDispatchBatchSize int
	RetryReenqueueDelay    time.Duration
	ShutdownTimeout        time.Duration
	ProcessorFailJobID     string
	LogLevel               string
}

func LoadWorkerConfig() (WorkerConfig, error) {
	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		return WorkerConfig{}, fmt.Errorf("DATABASE_URL is required")
	}

	redisDB, err := getEnvInt("REDIS_DB", 0)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("REDIS_DB: %w", err)
	}

	blockTimeout, err := getEnvDuration("REDIS_BLOCK_TIMEOUT", 3*time.Second)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("REDIS_BLOCK_TIMEOUT: %w", err)
	}

	retryDelay, err := getEnvDuration("RETRY_DELAY", 30*time.Second)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("RETRY_DELAY: %w", err)
	}
	if retryDelay <= 0 {
		return WorkerConfig{}, fmt.Errorf("RETRY_DELAY must be greater than zero")
	}

	retryDispatchInterval, err := getEnvDuration("RETRY_DISPATCH_INTERVAL", 1*time.Minute)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("RETRY_DISPATCH_INTERVAL: %w", err)
	}
	if retryDispatchInterval <= 0 {
		return WorkerConfig{}, fmt.Errorf("RETRY_DISPATCH_INTERVAL must be greater than zero")
	}

	retryDispatchBatchSize, err := getEnvInt("RETRY_DISPATCH_BATCH_SIZE", 10)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("RETRY_DISPATCH_BATCH_SIZE: %w", err)
	}
	if retryDispatchBatchSize <= 0 {
		return WorkerConfig{}, fmt.Errorf("RETRY_DISPATCH_BATCH_SIZE must be greater than zero")
	}

	retryReenqueueDelay, err := getEnvDuration("RETRY_REENQUEUE_DELAY", 1*time.Minute)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("RETRY_REENQUEUE_DELAY: %w", err)
	}
	if retryReenqueueDelay <= 0 {
		return WorkerConfig{}, fmt.Errorf("RETRY_REENQUEUE_DELAY must be greater than zero")
	}

	shutdownTimeout, err := getEnvDuration("WORKER_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("WORKER_SHUTDOWN_TIMEOUT: %w", err)
	}

	return WorkerConfig{
		DatabaseURL:            dbURL,
		RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:          getEnv("REDIS_PASSWORD", ""),
		RedisDB:                redisDB,
		RedisQueueKey:          getEnv("REDIS_QUEUE_KEY", "jobs:queue"),
		RedisBlockTimeout:      blockTimeout,
		RetryDelay:             retryDelay,
		RetryDispatchInterval:  retryDispatchInterval,
		RetryDispatchBatchSize: retryDispatchBatchSize,
		RetryReenqueueDelay:    retryReenqueueDelay,
		ShutdownTimeout:        shutdownTimeout,
		ProcessorFailJobID:     getEnv("PROCESSOR_FAIL_JOB_ID", ""),
		LogLevel:               getEnv("LOG_LEVEL", "info"),
	}, nil
}

func getEnv(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, err
	}
	return d, nil
}
