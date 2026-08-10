package main

import (
	"context"
	"log"
	"ride-sharing/shared/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
)

type tripConsumer struct {
	rabbitmq *messaging.RabbitMQ
}

func NewTripConsumer(rabbitmq *messaging.RabbitMQ) *tripConsumer {
	return &tripConsumer{
		rabbitmq: rabbitmq,
	}
}

func (c *tripConsumer) Start(ctx context.Context) error {
	return c.rabbitmq.ConsumeMessages(ctx, messaging.FindAvailableDriversQueue, func(ctx context.Context, msg amqp.Delivery) error {
		log.Printf("driver received message: %v", msg)
		return nil
	})
}
