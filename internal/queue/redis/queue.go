package redis

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/namta/async-job-system/internal/queue"
	"github.com/redis/go-redis/v9"
)

type Queue struct {
	client       *redis.Client
	key          string
	blockTimeout time.Duration
}

func NewRedisClient(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func NewQueue(client *redis.Client, key string, blockTimeout time.Duration) *Queue {
	return &Queue{
		client:       client,
		key:          key,
		blockTimeout: blockTimeout,
	}
}

func (q *Queue) Enqueue(ctx context.Context, msg queue.Message) error {
	return q.client.RPush(ctx, q.key, msg.JobID.String()).Err()
}

func (q *Queue) Dequeue(ctx context.Context) (queue.Message, error) {
	result, err := q.client.BLPop(ctx, q.blockTimeout, q.key).Result()
	if err != nil {
		if err == redis.Nil {
			return queue.Message{}, queue.ErrEmpty
		}
		return queue.Message{}, err
	}

	if len(result) != 2 {
		return queue.Message{}, queue.ErrEmpty
	}

	jobIDStr := result[1]
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		return queue.Message{}, err
	}

	return queue.Message{JobID: jobID}, nil
}

var _ queue.Queue = (*Queue)(nil)
