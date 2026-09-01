package main

import (
	"errors"
	"log"
	"net/http"
)

const (
	frontendModePrivateAdmin = "private-admin"
	frontendModePublic       = "public"
)

func guestDeploymentsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	records, err := store.List()
	if err != nil {
		log.Printf(
			"failed to load guest deployments: %v",
			err,
		)

		http.Error(
			w,
			"failed to retrieve deployments",
			http.StatusInternalServerError,
		)
		return
	}

	deployments := make(
		[]GuestDeploymentResponse,
		0,
		len(records),
	)

	for _, record := range records {
		response, err := guestDeploymentResponse(
			record,
			deploymentProjectStatus(record),
		)
		if errors.Is(err, ErrReservedPublicHostname) {
			continue
		}

		if err != nil {
			log.Printf(
				"skipping invalid guest deployment %q: %v",
				record.App,
				err,
			)
			continue
		}

		deployments = append(deployments, response)
	}

	writeJSON(w, http.StatusOK, deployments)
}

func guestDeploymentResponse(
	record DeploymentRecord,
	status string,
) (GuestDeploymentResponse, error) {
	record = normalizeDeploymentRecord(record)

	hostname := publicHostnameForApp(record.App)
	if hostname == "" {
		if isReservedPublicApp(record.App) {
			return GuestDeploymentResponse{},
				ErrReservedPublicHostname
		}

		return GuestDeploymentResponse{},
			errors.New("invalid public hostname")
	}

	return GuestDeploymentResponse{
		App:    record.App,
		URL:    "https://" + hostname,
		Status: status,
	}, nil
}

func adminSessionHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	identity, ok := accessIdentityFromContext(r.Context())
	if !ok {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		AdminSessionResponse{
			Role:  "admin",
			Email: identity.Email,
		},
	)
}

func publicDashboardHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	serveFrontendIndex(w, frontendModePublic)
}

func publicRootHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	publicDashboardHandler(w, r)
}
