package http

import (
	"encoding/json"
	"log"
	"net/http"
	"ride-sharing/services/trip-service/internal/domain"
	"ride-sharing/shared/httpx"
	"ride-sharing/shared/types"
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
	var reqBody previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	log.Printf("[HandleTripPreview] Received preview request for UserID: %s | Pickup: (%f, %f) -> Destination: (%f, %f)\n",
		reqBody.UserID,
		reqBody.Pickup.Latitude, reqBody.Pickup.Longitude,
		reqBody.Destination.Latitude, reqBody.Destination.Longitude,
	)

	ctx := r.Context()
	t, err := s.Service.GetRoute(ctx, &reqBody.Pickup, &reqBody.Destination)
	if err != nil {
		log.Println(err)
	}

	httpx.WriteJSON(w, http.StatusOK, t)
}
