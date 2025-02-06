package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/k11v/brick/internal/app"
)

func main() {
	run := func() int {
		ctx := context.Background()

		db, err := app.NewPostgresPool(ctx, "postgres://postgres:postgres@127.0.0.1:5432/postgres")
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		defer db.Close()
		s3 := app.NewS3Client("http://minioadmin:minioadmin@127.0.0.1:9000")
		worker := &Worker{DB: db, S3: s3}

		slog.Info("starting worker")
		err = worker.Run()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		return 0
	}
	os.Exit(run())
}
