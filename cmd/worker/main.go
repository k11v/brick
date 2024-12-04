package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// TODO: Remove.
	var send bool
	flag.BoolVar(&send, "send", false, "send a message to the worker")
	flag.Parse()

	// TODO: Remove.
	if send {
		if err := runSend(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	return nil
}

// TODO: Remove.
func runSend() error {
	return nil
}
