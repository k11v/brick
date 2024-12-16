package main

import (
	"context"
	"fmt"
	"os"

	"github.com/k11v/brick/internal/run/runpg"
	"github.com/k11v/brick/internal/run/runs3"
)

func main() {
	ctx := context.Background()

	err := runpg.Setup("postgres://postgres:postgres@127.0.0.1:5432/postgres")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = runs3.Setup(ctx, "http://minioadmin:minioadmin@127.0.0.1:9000")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}
