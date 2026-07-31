package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"ride-sharing/services/api-gateway/grpc_client"
	"ride-sharing/shared/contracts"
	"ride-sharing/shared/httpx"
)

func handleTripPreview(w http.ResponseWriter, r *http.Request) {
	var reqBody previewTripRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "failed to parse JSON data", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	if reqBody.UserID == "" {
		http.Error(w, "user ID is required", http.StatusBadRequest)
		return
	}

	jsonBody, _ := json.Marshal(reqBody)
	reader := bytes.NewReader(jsonBody)

	// TODO: refactor to singleton after tutorial
	tripService, err := grpc_client.NewTripServiceClient()
	if err != nil {
		log.Println("Failed to connect to trip service:", err)
	}
	defer tripService.Close()

	// TODO: Call trip service
	resp, err := http.Post("http://trip-service:8083/preview", "application/json", reader)
	if err != nil {
		log.Print(err)
		return
	}
	defer resp.Body.Close()

	var respBody any
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		http.Error(w, "failed to parse JSON data from trip service", http.StatusBadRequest)
		return
	}

	response := contracts.APIResponse{Data: respBody}
	httpx.WriteJSON(w, http.StatusCreated, response)
}
