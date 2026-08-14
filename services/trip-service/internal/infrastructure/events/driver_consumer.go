package events

import (
	"context"
	"encoding/json"
	"fmt"
	"ride-sharing/services/trip-service/internal/domain"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"
	pbd "ride-sharing/shared/proto/driver"

	"github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type driverConsumer struct {
	rabbitmq *messaging.RabbitMQ
	service  domain.TripService
}

func NewDriverConsumer(rabbitmq *messaging.RabbitMQ, service domain.TripService) *driverConsumer {
	return &driverConsumer{
		rabbitmq: rabbitmq,
		service:  service,
	}
}

func (c *driverConsumer) Start(ctx context.Context) error {
	return c.rabbitmq.ConsumeMessages(ctx, messaging.DriverTripResponseQueue, func(ctx context.Context, msg amqp091.Delivery) error {
		var message contracts.AmqpMessage
		if err := json.Unmarshal(msg.Body, &message); err != nil {
			return err
		}

		var payload messaging.DriverTripResponseData
		if err := json.Unmarshal(message.Data, &payload); err != nil {
			return err
		}
		logger.WithTrace(ctx).Info("driver response received", zap.String("routing_key", msg.RoutingKey), zap.String("trip_id", payload.TripID), zap.Any("payload", payload))

		switch msg.RoutingKey {
		case contracts.DriverCmdTripAccept:
			return c.handleTripAccepted(ctx, payload.TripID, payload.Driver)
		case contracts.DriverCmdTripDecline:
			return c.handleTripDeclined(ctx, payload.TripID, payload.RiderID)
		}

		logger.WithTrace(ctx).Warn("unknown trip event", zap.String("routing_key", msg.RoutingKey))
		return nil
	})
}

func (c *driverConsumer) handleTripDeclined(ctx context.Context, tripID, riderID string) error {
	// When a driver declines, try to find another driver

	trip, err := c.service.GetTripByID(ctx, tripID)
	if err != nil {
		return err
	}

	newPayload := messaging.TripEventData{
		Trip: trip.ToProto(),
	}

	marshalledPayload, err := json.Marshal(newPayload)
	if err != nil {
		return err
	}

	if err := c.rabbitmq.PublishMessage(ctx, contracts.TripEventDriverNotInterested,
		contracts.AmqpMessage{
			OwnerID: riderID,
			Data:    marshalledPayload,
		},
	); err != nil {
		return err
	}

	return nil
}

func (c *driverConsumer) handleTripAccepted(ctx context.Context, tripID string, driver *pbd.Driver) error {
	// 1. Fetch the first
	trip, err := c.service.GetTripByID(ctx, tripID)
	if err != nil {
		return err
	}

	if trip == nil {
		return fmt.Errorf("Trip was not found %s", tripID)
	}

	// 2. Update the trip
	if err := c.service.UpdateTrip(ctx, tripID, "accepted", driver); err != nil {
		return err
	}

	trip, err = c.service.GetTripByID(ctx, tripID)
	if err != nil {
		return err
	}

	// 3. Driver has been assigned -> publish this event to RB
	marshalledTrip, err := json.Marshal(trip)
	if err != nil {
		return err
	}

	// Notify the rider that a driver has been assigned
	if err := c.rabbitmq.PublishMessage(ctx, contracts.TripEventDriverAssigned, contracts.AmqpMessage{
		OwnerID: trip.UserID,
		Data:    marshalledTrip,
	}); err != nil {
		return err
	}

	// Notify the payment service to start a payment link
	marshalledPayload, err := json.Marshal(messaging.PaymentTripResponseData{
		TripID:   tripID,
		UserID:   trip.UserID,
		DriverID: driver.Id,
		Amount:   trip.RideFare.TotalPriceInCents,
		Currency: "USD",
	})

	if err := c.rabbitmq.PublishMessage(ctx, contracts.PaymentCmdCreateSession,
		contracts.AmqpMessage{
			OwnerID: trip.UserID,
			Data:    marshalledPayload,
		},
	); err != nil {
		return err
	}

	return nil
}
