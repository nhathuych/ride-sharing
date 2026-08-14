package http

import (
	"encoding/json"
	"net/http"
	"ride-sharing/services/trip-service/internal/domain"
	"ride-sharing/shared/httpx"
	"ride-sharing/shared/logger"
	"ride-sharing/shared/types"

	"go.uber.org/zap"
)

type HttpHandler struct {
	Service domain.TripService
}

type previewTripRequest struct {
	UserID      string           `json:"userID"`
	Pickup      types.Coordinate `json:"pickup"`
	Destination types.Coordinate `json:"destination"`
}

func (s *HttpHandler) HandleTripPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqBody previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		logger.WithTrace(ctx).Warn("failed to parse preview request", zap.Error(err))
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	logger.WithTrace(ctx).Info("received trip preview",
		zap.String("user_id", reqBody.UserID),
		zap.Float64("pickup_lat", reqBody.Pickup.Latitude),
		zap.Float64("pickup_lng", reqBody.Pickup.Longitude),
		zap.Float64("dest_lat", reqBody.Destination.Latitude),
		zap.Float64("dest_lng", reqBody.Destination.Longitude),
	)

	t, err := s.Service.GetRoute(ctx, &reqBody.Pickup, &reqBody.Destination)
	if err != nil {
		logger.WithTrace(ctx).Error("failed to get route", zap.Error(err))
		http.Error(w, "failed to get route", http.StatusInternalServerError)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, t)
}
