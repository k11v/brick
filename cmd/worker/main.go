package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/rabbitmq/amqp091-go"
	amqp "github.com/rabbitmq/amqp091-go"
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
	ctx := context.Background()

	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare("example", false, false, false, false, nil)
	if err != nil {
		return err
	}

	msg := amqp091.Publishing{
		ContentType: "text/plain",
		Body:        []byte("Hello World!"),
	}
	err = ch.PublishWithContext(ctx, "", q.Name, false, false, msg)
	if err != nil {
		return err
	}
	slog.Default().Info("sent", "msg", string(msg.Body))

	return nil
}
