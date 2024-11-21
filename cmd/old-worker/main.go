package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

type worker struct{}

func (w *worker) Run() error {
	for {
		time.Sleep(time.Second)
	}
}

func main() {
	if err := run(os.Stdout, os.Environ()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run(stdout io.Writer, environ []string) error {
	_ = context.Background()

	cfg, err := parseConfig(environ)
	if err != nil {
		return err
	}
	log := newLogger(stdout, cfg.Development)

	w := &worker{}

	log.Info("starting worker", "development", cfg.Development)
	if err = w.Run(); err != nil {
		return err
	}

	return nil
}
