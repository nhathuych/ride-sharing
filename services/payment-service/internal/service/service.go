package service

import (
	"context"
	"fmt"
	"ride-sharing/services/payment-service/internal/domain"
	"ride-sharing/services/payment-service/pkg/types"
	"ride-sharing/shared/logger"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type paymentService struct {
	paymentProcessor domain.PaymentProcessor
}

func NewPaymentService(paymentProcessor domain.PaymentProcessor) domain.Service {
	return &paymentService{
		paymentProcessor: paymentProcessor,
	}
}

func (s *paymentService) CreatePaymentSession(
	ctx context.Context,
	tripID string,
	userID string,
	driverID string,
	amount int64,
	currency string,
) (*types.PaymentIntent, error) {
	logger.WithTrace(ctx).Info("creating stripe checkout session", zap.String("trip_id", tripID), zap.Int64("amount", amount))

	metadata := map[string]string{
		"trip_id":   tripID,
		"user_id":   userID,
		"driver_id": driverID,
	}

	sessionID, err := s.paymentProcessor.CreatePaymentSession(ctx, amount, currency, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment session: %w", err)
	}

	paymentIntent := &types.PaymentIntent{
		ID:              uuid.New().String(),
		TripID:          tripID,
		UserID:          userID,
		DriverID:        driverID,
		Amount:          amount,
		Currency:        currency,
		StripeSessionID: sessionID,
		CreatedAt:       time.Now(),
	}

	logger.WithTrace(ctx).Info("stripe session created", zap.String("session_id", sessionID))
	return paymentIntent, nil
}
