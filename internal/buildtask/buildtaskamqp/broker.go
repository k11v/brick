package buildtaskamqp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/k11v/brick/internal/buildtask"
	"github.com/rabbitmq/amqp091-go"
)

type Broker struct {
	connectionString string // required
}

func NewBroker(connectionString string) *Broker {
	return &Broker{
		connectionString: connectionString,
	}
}

func (broker *Broker) SendBuildTask(ctx context.Context, b *buildtask.Build) error {
	conn, err := amqp091.Dial(broker.connectionString)
	if err != nil {
		return fmt.Errorf("send build task: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("send build task: %w", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"hello", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return fmt.Errorf("send build task: %w", err)
	}

	contentType := "application/json"
	body := &bytes.Buffer{}
	if err = json.NewEncoder(body).Encode(b); err != nil {
		return fmt.Errorf("send build task: %w", err)
	}
	msg := amqp091.Publishing{
		ContentType: contentType,
		Body:        body.Bytes(),
	}

	err = ch.PublishWithContext(ctx,
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		msg,    // message
	)
	if err != nil {
		return fmt.Errorf("send build task: %w", err)
	}

	return nil
}

// FIXME: The builds channel handling is nonideal.
// We don't have a way to acknowledge, check the error, or close the msgs channel.
func (broker *Broker) ReceiveBuildTasks(ctx context.Context) (<-chan *buildtask.Build, error) {
	conn, err := amqp091.Dial(broker.connectionString)
	if err != nil {
		return nil, fmt.Errorf("receive build tasks: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("receive build tasks: %w", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"hello", // name
		false,   // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("receive build tasks: %w", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("receive build tasks: %w", err)
	}

	var builds chan *buildtask.Build
	go func() {
		for msg := range msgs {
			var b *buildtask.Build
			dec := json.NewDecoder(bytes.NewReader(msg.Body))
			dec.DisallowUnknownFields()
			if err = dec.Decode(&b); err != nil {
				slog.Default().Error("receive build tasks failed", "err", err)
				close(builds)
				// return nil, fmt.Errorf("receive build tasks: %w", err)
			}
			if dec.More() {
				slog.Default().Error("receive build tasks: multiple top-level values", "err", err)
				close(builds)
				// return nil, fmt.Errorf("receive build tasks: multiple top-level values")
			}
			builds <- b
		}
	}()

	return builds, nil
}

// Deprecated: Use ReceiveBuildTasks instead.
func (broker *Broker) ReceiveBuildTask(ctx context.Context) (*buildtask.Build, error) {
	tasks, err := broker.ReceiveBuildTasks(ctx)
	if err != nil {
		return nil, err
	}

	select {
	case t := <-tasks:
		return t, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
