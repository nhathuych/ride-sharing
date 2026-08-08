package messaging

import (
	"context"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQ struct {
	conn    *amqp.Connection
	Channel *amqp.Channel
}

type MessageHandler func(context.Context, amqp.Delivery) error

func NewRabbitMQ(uri string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	rmq := &RabbitMQ{
		conn:    conn,
		Channel: ch,
	}

	if err := rmq.setupExchangesAndQueues(); err != nil {
		rmq.Close() // Clean up if setup fails
		return nil, fmt.Errorf("failed to setup exchanges and queues: %w", err)
	}

	return rmq, nil
}

func (r *RabbitMQ) PublishMessage(ctx context.Context, routingKey string, message string) error {
	return r.Channel.PublishWithContext(
		ctx,
		"",         // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType:  "text/plain",
			Body:         []byte(message),
			DeliveryMode: amqp.Persistent,
		},
	)
}

func (r *RabbitMQ) ConsumeMessages(ctx context.Context, queueName string, handler MessageHandler) error {
	msgs, err := r.Channel.Consume(
		queueName, // queue
		"",        // consumer
		false,     // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		return err
	}

	for msg := range msgs {
		log.Printf("Received a message: %s", msg.Body)

		if err := handler(ctx, msg); err != nil {
			log.Printf("ERROR: Failed to handle message: %v. Message body: %s", err, msg.Body)

			// Nack the message. Set requeue to false to avoid immediate redelivery loops.
			// Consider a dead-letter exchange (DLQ) or a more sophisticated retry mechanism for production.
			if nackErr := msg.Nack(false, false); nackErr != nil {
				log.Printf("ERROR: Failed to Nack message: %v", nackErr)
			}

			continue // Continue to the next message
		}

		// Only Ack if the handler succeeds
		if ackErr := msg.Ack(false); ackErr != nil {
			log.Printf("ERROR: Failed to Ack message: %v. Message body: %s", ackErr, msg.Body)
		}
	}

	return nil
}

func (r *RabbitMQ) setupExchangesAndQueues() error {
	_, err := r.Channel.QueueDeclare(
		"hello", // name
		true,    // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *RabbitMQ) Close() {
	if r.Channel != nil {
		r.Channel.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}
