package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{
		Status: "ok",
	})
}

func deployHandler(w http.ResponseWriter, r *http.Request) {
	var req DeployRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.RepoURL == "" {
		http.Error(w, "repoUrl is required", http.StatusBadRequest)
		return
	}

	appName := repoName(req.RepoURL)

	if appName == "" {
		http.Error(w, "invalid repository URL", http.StatusBadRequest)
		return
	}

	if isReservedPublicApp(appName) {
		http.Error(
			w,
			"deployment name is reserved",
			http.StatusBadRequest,
		)
		return
	}

	if _, err := store.Get(appName); err == nil {
		http.Error(
			w,
			"deployment already exists; use redeploy",
			http.StatusConflict,
		)
		return
	} else if !errors.Is(err, ErrDeploymentNotFound) {
		http.Error(
			w,
			"failed to check deployment metadata",
			http.StatusInternalServerError,
		)
		return
	}

	containerPort := normalizedContainerPort(
		req.ContainerPort,
	)

	healthPath := normalizedHealthPath(
		req.HealthPath,
	)

	if err := validateDeploymentConfig(
		containerPort,
		healthPath,
	); err != nil {
		http.Error(
			w,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	record, err := deployRepository(
		req.RepoURL,
		containerPort,
		healthPath,
	)

	if err != nil {
		deploymentEvent(
			appName,
			"ERROR: deployment failed: %v",
			err,
		)
		log.Printf("deployment failed: %v", err)
		http.Error(w, "deployment failed", http.StatusInternalServerError)
		return
	}

	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"proxy sync failed after deploying %s: %v",
			record.App,
			err,
		)
		http.Error(
			w,
			"deployment succeeded but proxy sync failed",
			http.StatusInternalServerError,
		)
		return
	}

	deploymentEvent(
		record.App,
		"Proxy routes synchronized. Deployment is live at https://%s.reactorlab.dev",
		record.App,
	)

	writeJSON(w, http.StatusCreated, deploymentResponse(record))
}

func redeployHandler(w http.ResponseWriter, r *http.Request) {
	record, err := getDeployment(r.PathValue("app"))

	if errors.Is(err, ErrDeploymentNotFound) {
		http.Error(w, "deployment not found", http.StatusNotFound)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	newRecord, err := safeRedeploy(record)

	if err != nil {
		deploymentEvent(
			record.App,
			"ERROR: redeployment failed: %v",
			err,
		)

		log.Printf(
			"safe redeployment failed for %s: %v",
			record.App,
			err,
		)

		http.Error(
			w,
			"redeployment failed; previous version kept running",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(w, http.StatusOK, deploymentResponse(newRecord))
}

func deploymentHistoryHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	app := r.PathValue("app")

	if _, err := getDeployment(app); err != nil {
		if errors.Is(err, ErrDeploymentNotFound) {
			http.Error(
				w,
				"deployment not found",
				http.StatusNotFound,
			)
			return
		}

		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	versions, err := historyStore.List(app)

	if err != nil {
		log.Printf(
			"failed to load history for %s: %v",
			app,
			err,
		)

		http.Error(
			w,
			"failed to load deployment history",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		HistoryResponse{
			App:      app,
			Versions: versions,
		},
	)
}

func rollbackDeploymentHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	app := r.PathValue("app")

	current, err := getDeployment(app)

	if errors.Is(err, ErrDeploymentNotFound) {
		http.Error(
			w,
			"deployment not found",
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	record, err := rollbackDeployment(current)

	if errors.Is(err, ErrNoRollbackVersion) {
		http.Error(
			w,
			"no previous deployment available",
			http.StatusConflict,
		)
		return
	}

	if err != nil {
		deploymentEvent(
			app,
			"ERROR: rollback failed: %v",
			err,
		)

		log.Printf(
			"rollback failed for %s: %v",
			app,
			err,
		)

		http.Error(
			w,
			"rollback failed; current version kept running",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		deploymentResponse(record),
	)
}

func deploymentsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	records, err := store.List()

	if err != nil {
		log.Printf(
			"failed to load deployments: %v",
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
		[]DeploymentResponse,
		0,
		len(records),
	)

	for _, record := range records {
		deployments = append(
			deployments,
			deploymentResponse(record),
		)
	}

	writeJSON(
		w,
		http.StatusOK,
		deployments,
	)
}

func logsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	record, err := getDeployment(
		r.PathValue("app"),
	)

	if errors.Is(
		err,
		ErrDeploymentNotFound,
	) {
		http.Error(
			w,
			"deployment not found",
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	if !containerExists(record.Container) {
		http.Error(
			w,
			"deployment container not found",
			http.StatusNotFound,
		)
		return
	}

	output, err := containerLogs(
		record.Container,
		200,
	)

	if err != nil {
		log.Printf(
			"failed to retrieve logs for %s:\n%s",
			record.App,
			output,
		)

		http.Error(
			w,
			"failed to retrieve logs",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		LogsResponse{
			App:       record.App,
			Container: record.Container,
			Logs:      output,
		},
	)
}

func deploymentLogsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	record, err := getDeployment(
		r.PathValue("app"),
	)

	if errors.Is(err, ErrDeploymentNotFound) {
		http.Error(
			w,
			"deployment not found",
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	output, err := readDeploymentLog(record.App)
	if err != nil {
		log.Printf(
			"failed to retrieve deployment logs for %s: %v",
			record.App,
			err,
		)

		http.Error(
			w,
			"failed to retrieve deployment logs",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		DeploymentLogsResponse{
			App:  record.App,
			Logs: output,
		},
	)
}

func restartDeploymentHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	record, err := getDeployment(
		r.PathValue("app"),
	)

	if errors.Is(
		err,
		ErrDeploymentNotFound,
	) {
		http.Error(
			w,
			"deployment not found",
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	if !containerExists(record.Container) {
		http.Error(
			w,
			"deployment container not found",
			http.StatusNotFound,
		)
		return
	}

	output, err := runCommand(
		"",
		"docker",
		"restart",
		record.Container,
	)

	if err != nil {
		log.Printf(
			"failed to restart %s:\n%s",
			record.App,
			output,
		)

		http.Error(
			w,
			"failed to restart deployment",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		ActionResponse{
			Status:    "running",
			App:       record.App,
			Container: record.Container,
		},
	)
}

func deleteDeploymentHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	record, err := getDeployment(
		r.PathValue("app"),
	)

	if errors.Is(
		err,
		ErrDeploymentNotFound,
	) {
		http.Error(
			w,
			"deployment not found",
			http.StatusNotFound,
		)
		return
	}

	if err != nil {
		http.Error(
			w,
			"failed to load deployment",
			http.StatusInternalServerError,
		)
		return
	}

	if containerExists(record.Container) {
		output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			record.Container,
		)

		if err != nil {
			log.Printf(
				"failed to delete container %s:\n%s",
				record.App,
				output,
			)

			http.Error(
				w,
				"failed to delete deployment",
				http.StatusInternalServerError,
			)
			return
		}
	}

	deployPath := filepath.Join(
		deploymentsDir,
		record.App,
	)

	if err := os.RemoveAll(
		deployPath,
	); err != nil {
		log.Printf(
			"failed to remove source for %s: %v",
			record.App,
			err,
		)
	}

	versions, historyErr := historyStore.List(
		record.App,
	)

	if historyErr != nil {
		log.Printf(
			"warning: failed to load history during delete for %s: %v",
			record.App,
			historyErr,
		)
	} else {
		for _, version := range versions {
			if version.Image == "" ||
				version.Image == record.Image {
				continue
			}

			if output, err := runCommand(
				"",
				"docker",
				"image",
				"rm",
				version.Image,
			); err != nil {
				log.Printf(
					"warning: failed to remove historical image %s:\n%s",
					version.Image,
					output,
				)
			}
		}
	}

	if err := historyStore.Clear(
		record.App,
	); err != nil {
		log.Printf(
			"warning: failed to clear history for %s: %v",
			record.App,
			err,
		)
	}

	if record.Image != "" {
		if output, err := runCommand(
			"",
			"docker",
			"image",
			"rm",
			record.Image,
		); err != nil {
			log.Printf(
				"failed to remove image %s:\n%s",
				record.Image,
				output,
			)
		}
	}

	if err := store.Delete(
		record.App,
	); err != nil {
		log.Printf(
			"failed to remove metadata for %s: %v",
			record.App,
			err,
		)

		http.Error(
			w,
			"failed to remove deployment metadata",
			http.StatusInternalServerError,
		)
		return
	}

	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"proxy sync failed after deleting %s: %v",
			record.App,
			err,
		)
		http.Error(
			w,
			"deployment deleted but proxy sync failed",
			http.StatusInternalServerError,
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		ActionResponse{
			Status:    "deleted",
			App:       record.App,
			Container: record.Container,
		},
	)
}

func getDeployment(
	app string,
) (DeploymentRecord, error) {
	if app == "" {
		return DeploymentRecord{},
			ErrDeploymentNotFound
	}

	record, err := store.Get(app)

	if err != nil {
		return DeploymentRecord{}, err
	}

	return normalizeDeploymentRecord(record), nil
}

func deploymentResponse(
	record DeploymentRecord,
) DeploymentResponse {
	record = normalizeDeploymentRecord(record)

	return DeploymentResponse{
		App:           record.App,
		RepoURL:       record.RepoURL,
		Container:     record.Container,
		Image:         record.Image,
		Port:          record.Port,
		ContainerPort: record.ContainerPort,
		HealthPath:    record.HealthPath,
		Status:        containerStatus(record.Container),
	}
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	value any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(
		value,
	); err != nil {
		log.Printf(
			"failed to encode response: %v",
			err,
		)
	}
}

func dashboardHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(
		w,
		r,
		"/srv/minideploy/frontend/dist/index.html",
	)
}
