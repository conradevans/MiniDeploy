package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func deploymentVisibilityHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	req, err := decodeDeploymentVisibilityRequest(r.Body)
	if err != nil {
		http.Error(w, "invalid visibility request", http.StatusBadRequest)
		return
	}

	record, err := store.UpdateGuestVisibility(
		r.PathValue("app"),
		*req.GuestVisible,
	)
	if errors.Is(err, ErrDeploymentNotFound) {
		http.Error(w, "deployment not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to save visibility", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, DeploymentVisibilityResponse{
		App:          record.App,
		GuestVisible: record.GuestVisible,
	})
}

func decodeDeploymentVisibilityRequest(
	reader io.Reader,
) (DeploymentVisibilityRequest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var req DeploymentVisibilityRequest
	if err := decoder.Decode(&req); err != nil {
		return DeploymentVisibilityRequest{}, err
	}
	if req.GuestVisible == nil {
		return DeploymentVisibilityRequest{}, errors.New("guestVisible is required")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return DeploymentVisibilityRequest{}, errors.New("request must contain one JSON object")
	}

	return req, nil
}
