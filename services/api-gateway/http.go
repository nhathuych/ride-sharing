package main

import (
	"encoding/json"
	"io"
	"net/http"
	"ride-sharing/services/api-gateway/grpc_client"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/env"
	"ride-sharing/shared/httpx"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/messaging"
	"ride-sharing/shared/tracing"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"go.uber.org/zap"
)

var tracer = tracing.GetTracer("api-gateway")

func handleTripStart(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "handleTripStart")
	defer span.End()

	var reqBody startTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		logger.WithTrace(ctx).Warn("failed to parse JSON", zap.Error(err))
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	// Why we need to create a new client for each connection:
	// because if a service is down, we don't want to block the whole application
	// so we create a new client for each connection
	tripService, err := grpc_client.NewTripServiceClient()
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Error("failed to connect trip service", zap.Error(err))
		http.Error(w, "Failed to start trip", http.StatusInternalServerError)
	}
	defer tripService.Close()

	trip, err := tripService.Client.CreateTrip(ctx, reqBody.toProto())
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Error("failed to start trip", zap.Error(err))
		http.Error(w, "Failed to start trip", http.StatusInternalServerError)
		return
	}

	logger.WithTrace(ctx).Info("trip started", zap.String("trip_id", trip.GetTripID()))
	response := contracts.APIResponse{Data: trip}
	httpx.WriteJSON(w, http.StatusCreated, response)
}

func handleTripPreview(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "handleTripPreview")
	defer span.End()

	var reqBody previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Warn("failed to parse JSON", zap.Error(err))
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	if reqBody.UserID == "" {
		logger.WithTrace(ctx).Warn("user ID is required")
		http.Error(w, "user ID is required", http.StatusBadRequest)
		return
	}

	// TODO: refactor to singleton after tutorial
	tripService, err := grpc_client.NewTripServiceClient()
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Error("failed to connect trip service", zap.Error(err))
		http.Error(w, "Failed to preview trip", http.StatusInternalServerError)
	}
	defer tripService.Close()

	tripPreview, err := tripService.Client.PreviewTrip(ctx, reqBody.toProto())
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Error("failed to preview trip", zap.Error(err), zap.String("user_id", reqBody.UserID))
		http.Error(w, "Failed to preview trip", http.StatusInternalServerError)
		return
	}

	logger.WithTrace(ctx).Info("trip previewed", zap.String("user_id", reqBody.UserID))
	response := contracts.APIResponse{Data: tripPreview}
	httpx.WriteJSON(w, http.StatusCreated, response)
}

func handleStripeWebhook(w http.ResponseWriter, r *http.Request, rabbitmq *messaging.RabbitMQ) {
	ctx, span := tracer.Start(r.Context(), "handleStripeWebhook")
	defer span.End()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Error("failed to read webhook body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	webhookKey := env.GetString("STRIPE_WEBHOOK_KEY", "")
	if webhookKey == "" {
		logger.WithTrace(ctx).Error("stripe webhook key is missing")
		http.Error(w, "Webhook key required", http.StatusInternalServerError)
		return
	}

	event, err := webhook.ConstructEventWithOptions(
		body,
		r.Header.Get("Stripe-Signature"),
		webhookKey,
		webhook.ConstructEventOptions{
			IgnoreAPIVersionMismatch: true,
		},
	)
	if err != nil {
		span.RecordError(err)
		logger.WithTrace(ctx).Warn("invalid stripe signature", zap.Error(err))
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	logger.WithTrace(ctx).Info("received stripe event", zap.Any("event", event))

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession

		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			span.RecordError(err)
			logger.WithTrace(ctx).Error("failed to unmarshal checkout session", zap.Error(err))
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		payload := messaging.PaymentStatusUpdateData{
			TripID:   session.Metadata["trip_id"],
			UserID:   session.Metadata["user_id"],
			DriverID: session.Metadata["driver_id"],
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			span.RecordError(err)
			logger.WithTrace(ctx).Error("failed to marshal payment payload", zap.Error(err))
			http.Error(w, "Failed to marshal payload", http.StatusInternalServerError)
			return
		}

		message := contracts.AmqpMessage{
			OwnerID: session.Metadata["user_id"],
			Data:    payloadBytes,
		}

		if err := rabbitmq.PublishMessage(
			ctx,
			contracts.PaymentEventSuccess,
			message,
		); err != nil {
			span.RecordError(err)
			logger.WithTrace(ctx).Error("failed to publish payment success", zap.Error(err), zap.String("trip_id", payload.TripID))
			http.Error(w, "Failed to publish payment event", http.StatusInternalServerError)
			return
		}

		logger.WithTrace(ctx).Info("payment success published", zap.String("trip_id", payload.TripID), zap.String("user_id", payload.UserID))
	}
}
