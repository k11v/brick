package main

import (
	"fmt"
	"os"

	"github.com/k11v/brick/internal/run/runpg"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	connectionString, ok := os.LookupEnv("BRICK_PG_CONNECTION_STRING")
	if !ok {
		return fmt.Errorf("BRICK_PG_CONNECTION_STRING is unset")
	}

	if err := runpg.Setup(connectionString); err != nil {
		return err
	}

	return nil
}
