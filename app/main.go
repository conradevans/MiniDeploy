package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	deploymentsDir = "/srv/minideploy/managed-deployments"
	metadataPath   = "/srv/minideploy/data/deployments.json"
	minDeployPort  = 8081
	maxDeployPort  = 8999
)

var (
	deployMu sync.Mutex
	store    DeploymentStore = NewJSONStore(metadataPath)
)

type HealthResponse struct {
	Status string `json:"status"`
}

type DeployRequest struct {
	RepoURL string `json:"repoUrl"`
}

type DeploymentResponse struct {
	App       string `json:"app"`
	RepoURL   string `json:"repoUrl"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

type LogsResponse struct {
	App       string `json:"app"`
	Container string `json:"container"`
	Logs      string `json:"logs"`
}

type ActionResponse struct {
	Status    string `json:"status"`
	App       string `json:"app"`
	Container string `json:"container"`
}

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

	record, err := deployRepository(req.RepoURL)

	if err != nil {
		log.Printf("deployment failed: %v", err)
		http.Error(w, "deployment failed", http.StatusInternalServerError)
		return
	}

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

func deployRepository(repoURL string) (DeploymentRecord, error) {
	deployMu.Lock()
	defer deployMu.Unlock()

	appName := repoName(repoURL)

	if appName == "" {
		return DeploymentRecord{}, fmt.Errorf("invalid repository URL")
	}

	deployPath := filepath.Join(deploymentsDir, appName)
	imageName := versionedImageName(appName)
	containerName := "minideploy-" + appName

	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"create deployment directory: %w",
			err,
		)
	}

	if err := os.RemoveAll(deployPath); err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"prepare deployment directory: %w",
			err,
		)
	}

	if output, err := runCommand(
		"",
		"git",
		"clone",
		repoURL,
		deployPath,
	); err != nil {
		log.Printf("git clone failed:\n%s", output)

		return DeploymentRecord{}, fmt.Errorf(
			"git clone failed: %w",
			err,
		)
	}

	if output, err := runCommand(
		deployPath,
		"docker",
		"build",
		"-t",
		imageName,
		".",
	); err != nil {
		log.Printf("docker build failed:\n%s", output)

		return DeploymentRecord{}, fmt.Errorf(
			"docker build failed: %w",
			err,
		)
	}

	port, err := findAvailablePort(
		minDeployPort,
		maxDeployPort,
	)

	if err != nil {
		return DeploymentRecord{}, err
	}

	if err := startManagedContainer(
		containerName,
		imageName,
		port,
	); err != nil {
		return DeploymentRecord{}, err
	}

	if err := verifyContainerStartup(containerName); err != nil {
		logs, _ := containerLogs(containerName, 100)

		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			containerName,
		)

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			imageName,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"container failed startup verification: %w; logs: %s",
			err,
			logs,
		)
	}

	record := DeploymentRecord{
		App:       appName,
		RepoURL:   repoURL,
		Container: containerName,
		Image:     imageName,
		Port:      port,
	}

	if err := store.Save(record); err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			containerName,
		)

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			imageName,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"save deployment metadata: %w",
			err,
		)
	}

	return record, nil
}

func safeRedeploy(
	old DeploymentRecord,
) (DeploymentRecord, error) {
	deployMu.Lock()
	defer deployMu.Unlock()

	version := fmt.Sprintf(
		"%d",
		time.Now().UnixNano(),
	)

	candidatePath := filepath.Join(
		deploymentsDir,
		old.App+"-candidate-"+version,
	)

	currentPath := filepath.Join(
		deploymentsDir,
		old.App,
	)

	newImage := fmt.Sprintf(
		"minideploy-%s:%s",
		old.App,
		version,
	)

	candidateContainer := fmt.Sprintf(
		"minideploy-%s-candidate-%s",
		old.App,
		version,
	)

	defer os.RemoveAll(candidatePath)

	log.Printf(
		"Building candidate for %s while current version stays live",
		old.App,
	)

	if output, err := runCommand(
		"",
		"git",
		"clone",
		old.RepoURL,
		candidatePath,
	); err != nil {
		log.Printf(
			"candidate git clone failed:\n%s",
			output,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"git clone candidate: %w",
			err,
		)
	}

	if output, err := runCommand(
		candidatePath,
		"docker",
		"build",
		"-t",
		newImage,
		".",
	); err != nil {
		log.Printf(
			"candidate Docker build failed:\n%s",
			output,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"build candidate image: %w",
			err,
		)
	}

	candidatePort, err := findAvailablePort(
		minDeployPort,
		maxDeployPort,
	)

	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"allocate candidate port: %w",
			err,
		)
	}

	if output, err := runCommand(
		"",
		"docker",
		"run",
		"-d",
		"--name",
		candidateContainer,
		"-p",
		fmt.Sprintf(
			"%d:80",
			candidatePort,
		),
		newImage,
	); err != nil {
		log.Printf(
			"candidate Docker run failed:\n%s",
			output,
		)

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"start candidate container: %w",
			err,
		)
	}

	if err := verifyContainerStartup(
		candidateContainer,
	); err != nil {
		logs, _ := containerLogs(
			candidateContainer,
			100,
		)

		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			candidateContainer,
		)

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"candidate failed startup verification: %w; logs: %s",
			err,
			logs,
		)
	}

	log.Printf(
		"Candidate for %s passed startup verification",
		old.App,
	)

	_, _ = runCommand(
		"",
		"docker",
		"rm",
		"-f",
		candidateContainer,
	)

	oldWasRunning :=
		containerStatus(old.Container) == "running"

	if containerExists(old.Container) {
		if output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			old.Container,
		); err != nil {
			log.Printf(
				"failed to remove old container:\n%s",
				output,
			)

			return DeploymentRecord{}, fmt.Errorf(
				"remove old container: %w",
				err,
			)
		}
	}

	if err := startManagedContainer(
		old.Container,
		newImage,
		old.Port,
	); err != nil {
		rollbackErr := restorePreviousContainer(
			old,
			oldWasRunning,
		)

		if rollbackErr != nil {
			log.Printf(
				"rollback also failed for %s: %v",
				old.App,
				rollbackErr,
			)
		}

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"start new live container: %w",
			err,
		)
	}

	if err := verifyContainerStartup(
		old.Container,
	); err != nil {
		logs, _ := containerLogs(
			old.Container,
			100,
		)

		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			old.Container,
		)

		rollbackErr := restorePreviousContainer(
			old,
			oldWasRunning,
		)

		if rollbackErr != nil {
			log.Printf(
				"rollback also failed for %s: %v",
				old.App,
				rollbackErr,
			)
		}

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"new live container failed verification: %w; logs: %s",
			err,
			logs,
		)
	}

	newRecord := old
	newRecord.Image = newImage

	if err := store.Save(newRecord); err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			old.Container,
		)

		rollbackErr := restorePreviousContainer(
			old,
			oldWasRunning,
		)

		if rollbackErr != nil {
			log.Printf(
				"rollback also failed for %s: %v",
				old.App,
				rollbackErr,
			)
		}

		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"save new deployment metadata: %w",
			err,
		)
	}

	if err := replaceDeploymentSource(
		currentPath,
		candidatePath,
	); err != nil {
		log.Printf(
			"warning: failed to replace deployment source for %s: %v",
			old.App,
			err,
		)
	}

	if old.Image != "" &&
		old.Image != newImage {
		if output, err := runCommand(
			"",
			"docker",
			"image",
			"rm",
			old.Image,
		); err != nil {
			log.Printf(
				"warning: failed to remove old image %s:\n%s",
				old.Image,
				output,
			)
		}
	}

	log.Printf(
		"Safe redeployment of %s completed",
		old.App,
	)

	return newRecord, nil
}

func startManagedContainer(
	containerName string,
	imageName string,
	port int,
) error {
	output, err := runCommand(
		"",
		"docker",
		"run",
		"-d",
		"--restart",
		"unless-stopped",
		"--name",
		containerName,
		"-p",
		fmt.Sprintf(
			"%d:80",
			port,
		),
		imageName,
	)

	if err != nil {
		log.Printf(
			"docker run failed:\n%s",
			output,
		)

		return fmt.Errorf(
			"docker run: %w",
			err,
		)
	}

	return nil
}

func restorePreviousContainer(
	record DeploymentRecord,
	shouldRun bool,
) error {
	if record.Image == "" {
		return fmt.Errorf(
			"previous image is unknown",
		)
	}

	if err := startManagedContainer(
		record.Container,
		record.Image,
		record.Port,
	); err != nil {
		return err
	}

	if !shouldRun {
		if output, err := runCommand(
			"",
			"docker",
			"stop",
			record.Container,
		); err != nil {
			log.Printf(
				"failed to restore stopped state:\n%s",
				output,
			)

			return err
		}
	}

	return nil
}

func replaceDeploymentSource(
	currentPath string,
	candidatePath string,
) error {
	if err := os.RemoveAll(currentPath); err != nil {
		return err
	}

	if err := os.Rename(
		candidatePath,
		currentPath,
	); err != nil {
		return err
	}

	return nil
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

func verifyContainerStartup(
	containerName string,
) error {
	const checks = 3

	for i := 0; i < checks; i++ {
		time.Sleep(time.Second)

		output, err := runCommand(
			"",
			"docker",
			"inspect",
			"-f",
			"{{.State.Status}}",
			containerName,
		)

		if err != nil {
			return fmt.Errorf(
				"inspect container: %w",
				err,
			)
		}

		status := strings.TrimSpace(output)

		if status != "running" {
			return fmt.Errorf(
				"container entered %q state",
				status,
			)
		}
	}

	return nil
}

func containerLogs(
	containerName string,
	tail int,
) (string, error) {
	return runCommand(
		"",
		"docker",
		"logs",
		"--tail",
		fmt.Sprintf("%d", tail),
		containerName,
	)
}

func getDeployment(
	app string,
) (DeploymentRecord, error) {
	if app == "" {
		return DeploymentRecord{},
			ErrDeploymentNotFound
	}

	return store.Get(app)
}

func deploymentResponse(
	record DeploymentRecord,
) DeploymentResponse {
	return DeploymentResponse{
		App:       record.App,
		RepoURL:   record.RepoURL,
		Container: record.Container,
		Image:     record.Image,
		Port:      record.Port,
		Status:    containerStatus(record.Container),
	}
}

func containerExists(
	containerName string,
) bool {
	_, err := runCommand(
		"",
		"docker",
		"inspect",
		containerName,
	)

	return err == nil
}

func containerStatus(
	containerName string,
) string {
	output, err := runCommand(
		"",
		"docker",
		"inspect",
		"-f",
		"{{.State.Status}}",
		containerName,
	)

	if err != nil {
		return "missing"
	}

	return strings.TrimSpace(output)
}

func findAvailablePort(
	start int,
	end int,
) (int, error) {
	for port := start; port <= end; port++ {
		listener, err := net.Listen(
			"tcp",
			fmt.Sprintf(
				":%d",
				port,
			),
		)

		if err != nil {
			continue
		}

		listener.Close()

		return port, nil
	}

	return 0, fmt.Errorf(
		"no available ports between %d and %d",
		start,
		end,
	)
}

func versionedImageName(
	appName string,
) string {
	return fmt.Sprintf(
		"minideploy-%s:%d",
		appName,
		time.Now().UnixNano(),
	)
}

func repoName(
	repoURL string,
) string {
	repoURL = strings.TrimSuffix(
		repoURL,
		"/",
	)

	name := filepath.Base(repoURL)

	name = strings.TrimSuffix(
		name,
		".git",
	)

	if name == "" || name == "." {
		return ""
	}

	return name
}

func runCommand(
	dir string,
	name string,
	args ...string,
) (string, error) {
	cmd := exec.Command(
		name,
		args...,
	)

	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()

	return string(output), err
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
		"/srv/minideploy/web/index.html",
	)
}

func main() {
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
		"GET /",
		dashboardHandler,
	)

	address := "127.0.0.1:9000"

	log.Printf(
		"MiniDeploy API listening on http://%s",
		address,
	)

	if err := http.ListenAndServe(
		address,
		mux,
	); err != nil {
		log.Fatal(err)
	}
}
