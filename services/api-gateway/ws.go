package main

import (
	"context"
	"encoding/json"
	"net/http"
	"ride-sharing/services/api-gateway/grpc_client"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/proto/driver"

	"go.uber.org/zap"
)

var (
	connManager = messaging.NewConnectionManager()
)

func handleRidersWebSocket(w http.ResponseWriter, r *http.Request, rb *messaging.RabbitMQ) {
	ctx := r.Context()

	conn, err := connManager.Upgrade(w, r)
	if err != nil {
		logger.WithTrace(ctx).Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	userID := r.URL.Query().Get("userID")
	if userID == "" {
		logger.WithTrace(ctx).Warn("No user ID provided for rider WS")
		return
	}

	// Add connection to manager
	connManager.Add(ctx, userID, conn)
	defer connManager.Remove(userID)

	logger.WithTrace(ctx).Info("Rider connected", zap.String("user_id", userID))

	// Initialize queue consumers
	queues := []string{
		messaging.NotifyDriverNoDriversFoundQueue,
		messaging.NotifyDriverAssignQueue,
		messaging.NotifyPaymentSessionCreatedQueue,
	}

	for _, q := range queues {
		consumer := messaging.NewQueueConsumer(rb, connManager, q)

		if err := consumer.Start(ctx); err != nil {
			logger.WithTrace(ctx).Error("Failed to start consumer for queue", zap.String("queue", q), zap.Error(err))
		}
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.WithTrace(ctx).Warn("Rider WS closed", zap.String("user_id", userID), zap.Error(err))
			break
		}

		msgCtx, span := tracer.Start(context.Background(), "websocket.message")
		logger.WithTrace(msgCtx).Info("Received rider message", zap.String("user_id", userID), zap.ByteString("message", message))
		span.End()
	}
}

func handleDriversWebSocket(w http.ResponseWriter, r *http.Request, rb *messaging.RabbitMQ) {
	ctx := r.Context()

	conn, err := connManager.Upgrade(w, r)
	if err != nil {
		logger.WithTrace(ctx).Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	userID := r.URL.Query().Get("userID")
	if userID == "" {
		logger.WithTrace(ctx).Warn("No user ID provided for driver WS")
		return
	}

	packageSlug := r.URL.Query().Get("packageSlug")
	if packageSlug == "" {
		logger.WithTrace(ctx).Warn("No package slug provided", zap.String("user_id", userID))
		return
	}

	// Add connection to manager
	connManager.Add(ctx, userID, conn)

	driverService, err := grpc_client.NewDriverServiceClient()
	if err != nil {
		logger.WithTrace(ctx).Fatal("Failed to create driver service client", zap.Error(err))
	}

	// Closing connections
	defer func() {
		connManager.Remove(userID)

		driverService.Client.UnregisterDriver(ctx, &driver.RegisterDriverRequest{
			DriverID:    userID,
			PackageSlug: packageSlug,
		})
		driverService.Close()

		logger.WithTrace(ctx).Info("Driver unregistered", zap.String("user_id", userID))
	}()

	driverData, err := driverService.Client.RegisterDriver(ctx, &driver.RegisterDriverRequest{
		DriverID:    userID,
		PackageSlug: packageSlug,
	})
	if err != nil {
		logger.WithTrace(ctx).Error("Error registering driver", zap.String("user_id", userID), zap.Error(err))
		return
	}

	if err := connManager.SendMessage(userID, contracts.WSMessage{
		Type: contracts.DriverCmdRegister,
		Data: driverData.Driver,
	}); err != nil {
		logger.WithTrace(ctx).Error("Error sending message", zap.String("user_id", userID), zap.Error(err))
		return
	}

	// Initialize queue consumers
	queues := []string{
		messaging.DriverCmdTripRequestQueue,
	}

	for _, q := range queues {
		consumer := messaging.NewQueueConsumer(rb, connManager, q)

		if err := consumer.Start(ctx); err != nil {
			logger.WithTrace(ctx).Error("Failed to start consumer", zap.String("queue", q), zap.Error(err))
		}
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logger.WithTrace(ctx).Warn("Error reading message", zap.String("user_id", userID), zap.Error(err))
			break
		}

		msgCtx, span := tracer.Start(context.Background(), "websocket.message")
		logger.WithTrace(msgCtx).Info("Received driver message", zap.String("user_id", userID))

		var driverMsg contracts.WSDriverMessage
		if err := json.Unmarshal(message, &driverMsg); err != nil {
			logger.WithTrace(msgCtx).Warn("Error unmarshaling driver message", zap.Error(err))
			span.End()
			continue
		}

		// Handle the different message type
		switch driverMsg.Type {
		case contracts.DriverCmdLocation:
			// Handle driver location update in the future
		case contracts.DriverCmdTripAccept, contracts.DriverCmdTripDecline:
			// Forward the message to RabbitMQ
			if err := rb.PublishMessage(msgCtx, driverMsg.Type, contracts.AmqpMessage{
				OwnerID: userID,
				Data:    driverMsg.Data,
			}); err != nil {
				logger.WithTrace(msgCtx).Error("Error publishing driver response to RabbitMQ", zap.Error(err))
			}
		default:
			logger.WithTrace(msgCtx).Warn("Unknown driver message type", zap.String("type", driverMsg.Type))
		}

		span.End()
	}
}
