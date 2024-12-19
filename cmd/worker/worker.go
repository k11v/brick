package main

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
)

type Worker struct {
	DB *pgxpool.Pool // required
	S3 *s3.Client    // required
}

func (w *Worker) Run() error {
	ctx := context.Background()

	retries := 0
	for {
		consumeErr := func() error {
			conn, err := amqp091.Dial("amqp://guest:guest@localhost:5672/")
			if err != nil {
				return err
			}
			defer conn.Close()

			ch, err := conn.Channel()
			if err != nil {
				return err
			}
			defer ch.Close()

			q, err := ch.QueueDeclare("operation.created", false, false, false, false, nil)
			if err != nil {
				return err
			}

			if err = ch.Qos(1, 0, false); err != nil {
				return err
			}

			messages, err := ch.Consume(q.Name, "", false, false, false, false, nil)
			if err != nil {
				return err
			}

			slog.Info("starting consuming")
			for m := range messages {
				slog.Info("received message")
				handler := &Handler{DB: w.DB, S3: w.S3}
				handler.Run(m)
				slog.Info("handled message")
				if retries > 0 && !ch.IsClosed() {
					slog.Info("recovered", "retries", retries)
					retries = 0
				}
			}

			return errors.New("delivery channel is closed")
		}()
		slog.Error("didn't consume", "err", consumeErr)

		retries++
		select {
		case <-time.After(retryWaitDuration(retries - 1)):
		case <-ctx.Done():
			return ctx.Err()
		}
		slog.Info("retrying", "retries", retries)
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
