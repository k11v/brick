package app

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
)

const AMQPQueueBuildCreated = "build.created"

type AMQPQueueDeclareParams struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp091.Table
}

type AMQPClient struct {
	connectionString   string
	queueDeclareParams *AMQPQueueDeclareParams
}

func NewAMQPClient(connectionString string, queueDeclareParams *AMQPQueueDeclareParams) *AMQPClient {
	return &AMQPClient{
		connectionString:   connectionString,
		queueDeclareParams: queueDeclareParams,
	}
}

// Publish proxies [amqp091.Channel.PublishWithContext].
func (cli *AMQPClient) Publish(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error {
	conn, err := amqp091.Dial(cli.connectionString)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(
		cli.queueDeclareParams.Name,
		cli.queueDeclareParams.Durable,
		cli.queueDeclareParams.AutoDelete,
		cli.queueDeclareParams.Exclusive,
		cli.queueDeclareParams.NoWait,
		cli.queueDeclareParams.Args,
	)
	if err != nil {
		return err
	}

	return ch.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}
