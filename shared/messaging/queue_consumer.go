package messaging

import (
	"context"
	"encoding/json"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/tracing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type QueueConsumer struct {
	rabbitmq    *RabbitMQ
	connManager *ConnectionManager
	queueName   string
}

func NewQueueConsumer(rabbitmq *RabbitMQ, connManager *ConnectionManager, queueName string) *QueueConsumer {
	return &QueueConsumer{
		rabbitmq:    rabbitmq,
		connManager: connManager,
		queueName:   queueName,
	}
}

func (qc *QueueConsumer) Start(ctx context.Context) error {
	msgs, err := qc.rabbitmq.Channel.Consume(
		qc.queueName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	logger.WithTrace(ctx).Info("Queue consumer started", zap.String("queue", qc.queueName))

	go func() {
		for msg := range msgs {

			err := tracing.TracedConsumer(msg, func(msgCtx context.Context, d amqp.Delivery) error {
				logger.WithTrace(msgCtx).Info("Received queue message", zap.String("queue", qc.queueName))

				var msgBody contracts.AmqpMessage
				if err := json.Unmarshal(d.Body, &msgBody); err != nil {
					logger.WithTrace(msgCtx).Warn("Failed to unmarshal message", zap.String("queue", qc.queueName), zap.Error(err))
					return err
				}

				userID := msgBody.OwnerID

				var payload any
				if msgBody.Data != nil {
					if err := json.Unmarshal(msgBody.Data, &payload); err != nil {
						logger.WithTrace(msgCtx).Warn("Failed to unmarshal payload", zap.String("queue", qc.queueName), zap.Error(err))
						return err
					}
				}

				clientMsg := contracts.WSMessage{
					Type: msg.RoutingKey,
					Data: payload,
				}

				if err := qc.connManager.SendMessage(userID, clientMsg); err != nil {
					logger.WithTrace(msgCtx).Error("Failed to send message to user",
						zap.String("queue", qc.queueName),
						zap.String("user_id", userID),
						zap.Error(err),
					)

					return err
				}

				logger.WithTrace(msgCtx).Info("Forwarded message to WS",
					zap.String("queue", qc.queueName),
					zap.String("user_id", userID),
					zap.String("routing_key", msg.RoutingKey),
				)

				return nil
			},
			)

			if err != nil {
				logger.WithTrace(ctx).Error("Error processing queue message", zap.String("queue", qc.queueName), zap.Error(err))
			}
		}
	}()

	return nil
}
