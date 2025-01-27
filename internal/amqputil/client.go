package amqputil

import (
	"context"

	"github.com/rabbitmq/amqp091-go"
)

type QueueDeclareParams struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp091.Table
}

type Client struct {
	connectionString   string
	queueDeclareParams *QueueDeclareParams
}

func NewClient(connectionString string, queueDeclareParams *QueueDeclareParams) *Client {
	return &Client{
		connectionString:   connectionString,
		queueDeclareParams: queueDeclareParams,
	}
}

// Publish proxies [amqp091.Channel.PublishWithContext].
func (cli *Client) Publish(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp091.Publishing) error {
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
