package amqp

import (
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

	body, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("send build task: %w", err)
	}
	msg := amqp091.Publishing{
		ContentType: "application/json",
		Body:        body,
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

func (Broker) ReceiveBuildTask() (*build.Build, error) {
	panic("unimplemented")
}
