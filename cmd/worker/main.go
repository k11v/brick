package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
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

	retries := 0

	slog.Info("starting worker")
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

		retries++
		select {
		case <-time.After(retryWaitDuration(retries - 1)):
		case <-ctx.Done():
			return ctx.Err()
		}
		slog.Default().Info("retrying", "retries", retries)
	}
}

// retryWaitDuration calculates the wait duration for a retry.
// It is calculated using exponential backoff with jitter.
// It grows with each retry and stops growing after thirteenth retry
// where it is chosen from the the interval (32.4s, 97.4s).
// The first retry number is 0, the thirteenth is 12.
func retryWaitDuration(retry int) time.Duration {
	n := min(retry, 12)
	second := int(time.Second)

	// start with 0.5s
	duration := second / 2

	// multiply by 1.5 to the power of n
	for i := 0; i < n; i++ {
		duration /= 2
		duration *= 3
	}

	// add or subtract up to 50%
	jitter := rand.IntN(duration) - duration/2
	duration += jitter

	return time.Duration(duration)
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
