package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type WorkerConfig struct {
	DatabaseURL       string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RedisQueueKey     string
	RedisBlockTimeout time.Duration
	ShutdownTimeout   time.Duration
	LogLevel          string
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

	shutdownTimeout, err := getEnvDuration("WORKER_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return WorkerConfig{}, fmt.Errorf("WORKER_SHUTDOWN_TIMEOUT: %w", err)
	}

	return WorkerConfig{
		DatabaseURL:       dbURL,
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		RedisDB:           redisDB,
		RedisQueueKey:     getEnv("REDIS_QUEUE_KEY", "jobs:queue"),
		RedisBlockTimeout: blockTimeout,
		ShutdownTimeout:   shutdownTimeout,
		LogLevel:          getEnv("LOG_LEVEL", "info"),
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
