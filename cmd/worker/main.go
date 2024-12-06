package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	ctx := context.Background()

	maxRetries := 15
	retries := 0

	for {
		consumeErr := func() error {
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

			if err = ch.Qos(1, 0, false); err != nil {
				return err
			}

			msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
			if err != nil {
				return err
			}

			for msg := range msgs {
				slog.Default().Info("received", "msg", string(msg.Body))

				// TODO:
				// Good handler must call msg.Ack, msg.Nack, or msg.Reject.
				// These methods can fail and when they do, the channel
				// becomes invalid which LIKELY means it closes.
				// In this case We MAYBE need to range over the msgs
				// until the end and drop every msg we encounter
				// to prevent memory leaks or deadlocks.
				// The handler should not be called when we do this.
				// Right now we also rely on the channel not being closed
				// after message acknowledgement to reset retries.
				// Better API would MAYBE automatically issue a msg.Reject
				// when the handler forgets to acknowledge do anything.
				func(msg amqp.Delivery) {
					if err = msg.Ack(false); err != nil {
						slog.Default().Error("didn't acknowledge", "err", err)
					}
				}(msg)

				if retries > 0 && !ch.IsClosed() {
					slog.Default().Info("recovered", "retries", retries)
					retries = 0
				}
			}

			return errors.New("delivery channel is closed")
		}()
		slog.Default().Error("didn't consume", "err", consumeErr)

		if retries > maxRetries {
			return errors.New("max retries is exceeded")
		}
		retries++

		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		slog.Default().Info("retrying")
	}
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
