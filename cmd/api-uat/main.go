package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/namta/async-job-system/internal/httpapi"
	"github.com/namta/async-job-system/internal/jobs/postgres"
	redisqueue "github.com/namta/async-job-system/internal/queue/redis"
)

func main() {
	db, err := sql.Open("pgx", "postgres://postgres:postgres@127.0.0.1:55432/async_jobs?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	rdb, err := redisqueue.NewRedisClient(ctx, "localhost:6379", "", 0)
	if err != nil {
		log.Fatal(err)
	}
	defer rdb.Close()

	repo := postgres.NewRepository(db)
	q := redisqueue.NewQueue(rdb, "jobs:queue", 3*time.Second)
	h := httpapi.NewJobsHandler(repo, q)

	log.Println("api listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", httpapi.NewRouter(h)))
}
