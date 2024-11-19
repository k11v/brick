package amqp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/rabbitmq/amqp091-go"

	"github.com/k11v/brick/internal/app/build"
)

type Broker struct {
	connectionString string // required
}

func NewBroker(connectionString string) *Broker {
	return &Broker{
		connectionString: connectionString,
	}
}

func (broker Broker) SendBuildTask(ctx context.Context, b *build.Build) error {
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

// TODO: Consider returning a channel.
func (broker Broker) ReceiveBuildTask(ctx context.Context) (*build.Build, error) {
	conn, err := amqp091.Dial(broker.connectionString)
	if err != nil {
		return nil, fmt.Errorf("receive build task: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("receive build task: %w", err)
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
		return nil, fmt.Errorf("receive build task: %w", err)
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
		return nil, fmt.Errorf("receive build task: %w", err)
	}

	var msg amqp091.Delivery
	select {
	case msg = <-msgs:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	var b build.Build
	dec := json.NewDecoder(bytes.NewReader(msg.Body))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("receive build task: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("receive build task: multiple top-level values")
	}

	return &b, nil
}
