package main

import (
	"net/http"
)

func routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"GET /health",
		healthHandler,
	)

	mux.HandleFunc(
		"POST /deploy",
		deployHandler,
	)

	mux.HandleFunc(
		"GET /deployments",
		deploymentsHandler,
	)

	mux.HandleFunc(
		"GET /deployments/{app}/logs",
		logsHandler,
	)

	mux.HandleFunc(
		"GET /deployments/{app}/history",
		deploymentHistoryHandler,
	)

	mux.HandleFunc(
		"POST /deployments/{app}/rollback",
		rollbackDeploymentHandler,
	)

	mux.HandleFunc(
		"POST /deployments/{app}/restart",
		restartDeploymentHandler,
	)

	mux.HandleFunc(
		"POST /deployments/{app}/redeploy",
		redeployHandler,
	)

	mux.HandleFunc(
		"DELETE /deployments/{app}",
		deleteDeploymentHandler,
	)

	mux.HandleFunc(
		"POST /webhooks/github",
		githubWebhookHandler,
	)

	mux.HandleFunc(
		"GET /",
		dashboardHandler,
	)

	return mux
}
