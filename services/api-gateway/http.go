package main

import (
	"encoding/json"
	"net/http"
	"ride-sharing/shared/contracts"
)

func handleTripPreview(w http.ResponseWriter, r *http.Request) {
	var reqBoby previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBoby); err != nil {
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	if reqBoby.UserID == "" {
		http.Error(w, "user ID is required", http.StatusBadRequest)
		return
	}

	// TODO: Call trip service

	response := contracts.APIResponse{Data: "ok"}
	writeJSON(w, http.StatusCreated, response)
}
