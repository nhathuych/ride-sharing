package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type tripConsumer struct {
	rabbitmq *messaging.RabbitMQ
	service  *Service
}

func NewTripConsumer(rabbitmq *messaging.RabbitMQ, service *Service) *tripConsumer {
	return &tripConsumer{
		rabbitmq: rabbitmq,
		service:  service,
	}
}

func (c *tripConsumer) Start(ctx context.Context) error {
	return c.rabbitmq.ConsumeMessages(ctx, messaging.FindAvailableDriversQueue, func(ctx context.Context, msg amqp.Delivery) error {
		var tripEvent contracts.AmqpMessage
		if err := json.Unmarshal(msg.Body, &tripEvent); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal amqp message", zap.Error(err))
			return err
		}

		var payload messaging.TripEventData
		if err := json.Unmarshal(tripEvent.Data, &payload); err != nil {
			logger.WithTrace(ctx).Error("failed to unmarshal trip data", zap.Error(err))
			return err
		}

		logger.WithTrace(ctx).Info("driver received message",
			zap.String("routing_key", msg.RoutingKey),
			zap.String("trip_id", payload.Trip.Id),
			zap.Any("payload", payload),
		)

		switch msg.RoutingKey {
		case contracts.TripEventCreated, contracts.TripEventDriverNotInterested:
			return c.handleFindAndNotifyDrivers(ctx, payload)
		}

		logger.WithTrace(ctx).Warn("unknown trip event", zap.Any("payload", payload))
		return nil
	})
}

func (c *tripConsumer) handleFindAndNotifyDrivers(ctx context.Context, payload messaging.TripEventData) error {
	suitableIDs := c.service.FindAvailableDrivers(payload.Trip.SelectedFare.PackageSlug)
	logger.WithTrace(ctx).Info("found suitable drivers",
		zap.Int("count", len(suitableIDs)),
		zap.String("package", payload.Trip.SelectedFare.PackageSlug),
	)

	if len(suitableIDs) == 0 {
		// Notify the driver that no drivers are available
		if err := c.rabbitmq.PublishMessage(ctx, contracts.TripEventNoDriversFound, contracts.AmqpMessage{
			OwnerID: payload.Trip.UserID,
		}); err != nil {
			logger.WithTrace(ctx).Error("failed to publish no drivers found", zap.Error(err))
			return err
		}

		return nil
	}

	randomIndex := rand.Intn(len(suitableIDs))
	suitableDriverID := suitableIDs[randomIndex]

	marshalledEvent, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Notify the driver about a potential trip
	if err := c.rabbitmq.PublishMessage(ctx, contracts.DriverCmdTripRequest, contracts.AmqpMessage{
		OwnerID: suitableDriverID,
		Data:    marshalledEvent,
	}); err != nil {
		logger.WithTrace(ctx).Error("failed to publish driver request", zap.Error(err))
		return err
	}

	logger.WithTrace(ctx).Info("notified driver", zap.String("driver_id", suitableDriverID))
	return nil
}
