package events

import (
	"context"
	"encoding/json"
	"ride-sharing/services/payment-service/internal/domain"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"

	"github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type TripConsumer struct {
	rabbitmq *messaging.RabbitMQ
	service  domain.Service
}

func NewTripConsumer(rabbitmq *messaging.RabbitMQ, service domain.Service) *TripConsumer {
	return &TripConsumer{
		rabbitmq: rabbitmq,
		service:  service,
	}
}

func (c *TripConsumer) Start(ctx context.Context) error {
	return c.rabbitmq.ConsumeMessages(ctx, messaging.PaymentTripResponseQueue, func(ctx context.Context, msg amqp091.Delivery) error {
		var message contracts.AmqpMessage
		if err := json.Unmarshal(msg.Body, &message); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal amqp message", zap.Error(err))
			return err
		}

		var payload messaging.PaymentTripResponseData
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal payment payload", zap.Error(err))
			return err
		}

		logger.WithTrace(ctx).Info("payment event received",
			zap.String("routing_key", msg.RoutingKey),
			zap.String("trip_id", payload.TripID),
		)

		switch msg.RoutingKey {
		case contracts.PaymentCmdCreateSession:
			if err := c.handleTripAccepted(ctx, payload); err != nil {
				logger.WithTrace(ctx).Error("failed to handle trip accepted", zap.Error(err), zap.String("trip_id", payload.TripID))
				return err
			}
		}

		return nil
	})
}

func (c *TripConsumer) handleTripAccepted(ctx context.Context, payload messaging.PaymentTripResponseData) error {
	logger.WithTrace(ctx).Info("handling trip accepted", zap.String("trip_id", payload.TripID), zap.String("user_id", payload.UserID))

	paymentSession, err := c.service.CreatePaymentSession(
		ctx,
		payload.TripID,
		payload.UserID,
		payload.DriverID,
		int64(payload.Amount),
		payload.Currency,
	)
	if err != nil {
		logger.WithTrace(ctx).Error("failed to create payment session", zap.Error(err), zap.String("trip_id", payload.TripID))
		return err
	}

	logger.WithTrace(ctx).Info("payment session created", zap.String("trip_id", payload.TripID), zap.String("stripe_session_id", paymentSession.StripeSessionID))

	// Publish payment session created event
	paymentPayload := messaging.PaymentEventSessionCreatedData{
		TripID:    payload.TripID,
		SessionID: paymentSession.StripeSessionID,
		Amount:    float64(paymentSession.Amount) / 100.0, // Convert from cents to dollars
		Currency:  paymentSession.Currency,
	}

	payloadBytes, err := json.Marshal(paymentPayload)
	if err != nil {
		logger.WithTrace(ctx).Error("failed to marshal session payload", zap.Error(err))
		return err
	}

	if err := c.rabbitmq.PublishMessage(ctx, contracts.PaymentEventSessionCreated,
		contracts.AmqpMessage{
			OwnerID: payload.UserID,
			Data:    payloadBytes,
		},
	); err != nil {
		logger.WithTrace(ctx).Error("failed to publish session created", zap.Error(err))
		return err
	}

	logger.WithTrace(ctx).Info("published payment session created", zap.String("trip_id", payload.TripID))
	return nil
}
