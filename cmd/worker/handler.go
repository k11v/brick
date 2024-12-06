package main

import "github.com/rabbitmq/amqp091-go"

type Handler struct{}

func (*Handler) RunBuild(m amqp091.Delivery) {
}
