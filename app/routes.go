package main

import (
	"net/http"
	"strings"
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
		"GET /deployments/{app}/deploy-logs",
		deploymentLogsHandler,
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

	frontendFiles := http.FileServer(
		http.Dir("/srv/minideploy/frontend/dist"),
	)

	mux.Handle(
		"GET /assets/",
		frontendFiles,
	)

	mux.Handle(
		"GET /favicon.svg",
		frontendFiles,
	)

	mux.Handle(
		"GET /icons.svg",
		frontendFiles,
	)

	mux.HandleFunc(
		"GET /",
		dashboardHandler,
	)

	return mux
}

func publicRoutes(
	validator AccessTokenValidator,
) http.Handler {
	mux := http.NewServeMux()

	frontendFiles := http.FileServer(
		http.Dir("/srv/minideploy/frontend/dist"),
	)

	mux.Handle(
		"GET /assets/",
		frontendFiles,
	)

	mux.Handle(
		"GET /favicon.svg",
		frontendFiles,
	)

	mux.Handle(
		"GET /icons.svg",
		frontendFiles,
	)

	mux.HandleFunc(
		"GET /api/guest/deployments",
		guestDeploymentsHandler,
	)

	registerPublicAdminRoutes(mux)

	mux.HandleFunc(
		"GET /",
		dashboardHandler,
	)

	return protectPublicAdminRoutes(validator, mux)
}

func registerPublicAdminRoutes(
	mux *http.ServeMux,
) {
	admin := func(
		pattern string,
		handler http.HandlerFunc,
	) {
		mux.HandleFunc(
			pattern,
			handler,
		)
	}

	admin(
		"GET /api/admin/session",
		adminSessionHandler,
	)

	admin(
		"POST /api/admin/deploy",
		deployHandler,
	)

	admin(
		"GET /api/admin/deployments",
		deploymentsHandler,
	)

	admin(
		"GET /api/admin/deployments/{app}/logs",
		logsHandler,
	)

	admin(
		"GET /api/admin/deployments/{app}/history",
		deploymentHistoryHandler,
	)

	admin(
		"GET /api/admin/deployments/{app}/deploy-logs",
		deploymentLogsHandler,
	)

	admin(
		"POST /api/admin/deployments/{app}/rollback",
		rollbackDeploymentHandler,
	)

	admin(
		"POST /api/admin/deployments/{app}/restart",
		restartDeploymentHandler,
	)

	admin(
		"POST /api/admin/deployments/{app}/redeploy",
		redeployHandler,
	)

	admin(
		"DELETE /api/admin/deployments/{app}",
		deleteDeploymentHandler,
	)

	admin(
		"GET /admin",
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(
				w,
				r,
				"/admin/",
				http.StatusTemporaryRedirect,
			)
		},
	)

	admin(
		"GET /admin/",
		publicDashboardHandler,
	)
}

func protectPublicAdminRoutes(
	validator AccessTokenValidator,
	next http.Handler,
) http.Handler {
	protected := requireAccess(validator, next)

	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if r.URL.Path == "/admin" ||
			strings.HasPrefix(r.URL.Path, "/admin/") ||
			r.URL.Path == "/api/admin" ||
			strings.HasPrefix(r.URL.Path, "/api/admin/") {

			protected.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
