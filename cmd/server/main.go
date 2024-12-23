package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/run/runpg"
	"github.com/k11v/brick/internal/run/runs3"
)

func main() {
	run := func() int {
		ctx := context.Background()

		db, err := runpg.NewPool(ctx, "postgres://postgres:postgres@127.0.0.1:5432/postgres")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer db.Close()
		mq, err := amqp091.Dial("amqp://guest:guest@127.0.0.1:5672/")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer func() {
			_ = mq.Close()
		}()
		s3Client := runs3.NewClient("http://minioadmin:minioadmin@127.0.0.1:9000")
		server := NewServer(db, mq, s3Client, &Config{})

		slog.Info("starting server", "addr", server.Addr)
		err = server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		return 0
	}
	os.Exit(run())
}
