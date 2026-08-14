package events

import (
	"context"
	"encoding/json"
	"ride-sharing/services/trip-service/internal/domain"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type paymentConsumer struct {
	rabbitmq *messaging.RabbitMQ
	service  domain.TripService
}

func NewPaymentConsumer(rabbitmq *messaging.RabbitMQ, service domain.TripService) *paymentConsumer {
	return &paymentConsumer{
		rabbitmq: rabbitmq,
		service:  service,
	}
}

func (c *paymentConsumer) Start(ctx context.Context) error {
	return c.rabbitmq.ConsumeMessages(ctx, messaging.NotifyPaymentSuccessQueue, func(ctx context.Context, msg amqp.Delivery) error {
		var message contracts.AmqpMessage
		if err := json.Unmarshal(msg.Body, &message); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal message", zap.Error(err))
			return err
		}
		var payload messaging.PaymentStatusUpdateData
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal payment payload", zap.Error(err))
			return err
		}

		logger.WithTrace(ctx).Info("Trip has been completed and paid", zap.String("trip_id", payload.TripID))

		return c.service.UpdateTrip(ctx, payload.TripID, "paid", nil)
	})
}
