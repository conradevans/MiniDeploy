package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func deployRepository(
	repoURL string,
	containerPort int,
	healthPath string,
) (DeploymentRecord, error) {
	deployMu.Lock()
	defer deployMu.Unlock()

	containerPort = normalizedContainerPort(
		containerPort,
	)

	healthPath = normalizedHealthPath(
		healthPath,
	)

	if err := validateDeploymentConfig(
		containerPort,
		healthPath,
	); err != nil {
		return DeploymentRecord{}, err
	}

	appName := repoName(repoURL)

	if appName == "" {
		return DeploymentRecord{}, fmt.Errorf("invalid repository URL")
	}

	resetDeploymentLog(appName, "deployment")
	deploymentEvent(
		appName,
		"Preparing deployment: container port %d, health path %s",
		containerPort,
		healthPath,
	)

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

	deploymentEvent(appName, "Cloning repository...")

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

	deploymentEvent(appName, "Repository cloned.")
	deploymentEvent(appName, "Building Docker image...")

	if output, err := runCommand(
		deployPath,
		"docker",
		"build",
		"-t",
		imageName,
		".",
	); err != nil {
		log.Printf("docker build failed:\n%s", output)
		deploymentEvent(
			appName,
			"ERROR: Docker build failed:\n%s",
			output,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"docker build failed: %w",
			err,
		)
	}

	deploymentEvent(
		appName,
		"Docker image built successfully: %s",
		imageName,
	)

	port, err := findAvailablePort(
		minDeployPort,
		maxDeployPort,
	)

	if err != nil {
		return DeploymentRecord{}, err
	}

	deploymentEvent(
		appName,
		"Starting container on host port %d...",
		port,
	)

	if err := startManagedContainerWithPort(
		containerName,
		imageName,
		port,
		containerPort,
	); err != nil {
		return DeploymentRecord{}, err
	}

	deploymentEvent(appName, "Container started. Verifying startup...")

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

	deploymentEvent(
		appName,
		"Startup verification passed. Checking HTTP health at %s...",
		healthPath,
	)

	if err := verifyHTTPHealthPath(
		port,
		healthPath,
	); err != nil {
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
			"container failed HTTP health check: %w; logs: %s",
			err,
			logs,
		)
	}

	deploymentEvent(appName, "HTTP health check passed.")

	record := DeploymentRecord{
		App:           appName,
		RepoURL:       repoURL,
		Container:     containerName,
		Image:         imageName,
		Port:          port,
		ContainerPort: containerPort,
		HealthPath:    healthPath,
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

	deploymentEvent(
		appName,
		"Container is healthy and deployment metadata is saved.",
	)

	return record, nil
}

func safeRedeploy(
	old DeploymentRecord,
) (DeploymentRecord, error) {
	old = normalizeDeploymentRecord(old)

	deployMu.Lock()
	defer deployMu.Unlock()

	resetDeploymentLog(old.App, "zero-downtime redeploy")
	deploymentEvent(
		old.App,
		"Current version remains live on port %d.",
		old.Port,
	)

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
		"Building zero-downtime candidate for %s while current version stays live",
		old.App,
	)

	deploymentEvent(
		old.App,
		"Cloning repository for candidate...",
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

	deploymentEvent(
		old.App,
		"Repository cloned. Building candidate Docker image...",
	)

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
		deploymentEvent(
			old.App,
			"ERROR: candidate Docker build failed:\n%s",
			output,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"build candidate image: %w",
			err,
		)
	}

	deploymentEvent(
		old.App,
		"Candidate Docker image built successfully: %s",
		newImage,
	)

	candidatePort, err := findAvailablePort(
		minDeployPort,
		maxDeployPort,
	)
	if err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"image",
			"rm",
			newImage,
		)
		return DeploymentRecord{}, fmt.Errorf(
			"allocate candidate port: %w",
			err,
		)
	}

	deploymentEvent(
		old.App,
		"Starting candidate container on port %d...",
		candidatePort,
	)

	if err := startManagedContainerWithPort(
		candidateContainer,
		newImage,
		candidatePort,
		old.ContainerPort,
	); err != nil {
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

	cleanupCandidate := func(removeImage bool) {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			candidateContainer,
		)

		if removeImage {
			_, _ = runCommand(
				"",
				"docker",
				"image",
				"rm",
				newImage,
			)
		}
	}

	if err := verifyContainerStartup(
		candidateContainer,
	); err != nil {
		logs, _ := containerLogs(
			candidateContainer,
			100,
		)
		cleanupCandidate(true)

		return DeploymentRecord{}, fmt.Errorf(
			"candidate failed startup verification: %w; logs: %s",
			err,
			logs,
		)
	}

	if err := verifyHTTPHealthPath(
		candidatePort,
		old.HealthPath,
	); err != nil {
		logs, _ := containerLogs(
			candidateContainer,
			100,
		)
		cleanupCandidate(true)

		return DeploymentRecord{}, fmt.Errorf(
			"candidate failed HTTP health check: %w; logs: %s",
			err,
			logs,
		)
	}

	log.Printf(
		"Candidate for %s is healthy on port %d; current version is still serving on port %d",
		old.App,
		candidatePort,
		old.Port,
	)

	deploymentEvent(
		old.App,
		"Candidate passed startup and HTTP health checks.",
	)

	newRecord := old
	newRecord.Container = candidateContainer
	newRecord.Image = newImage
	newRecord.Port = candidatePort

	// Persist the candidate as the desired active deployment.
	// The old container is deliberately still running here.
	if err := store.Save(newRecord); err != nil {
		cleanupCandidate(true)

		return DeploymentRecord{}, fmt.Errorf(
			"save candidate deployment metadata: %w",
			err,
		)
	}

	// Atomically reload Caddy so new requests begin going to the
	// already-healthy candidate while the old container remains alive.
	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"proxy cutover failed for %s; restoring old routing: %v",
			old.App,
			err,
		)

		if restoreErr := store.Save(old); restoreErr != nil {
			log.Printf(
				"failed restoring old deployment metadata for %s: %v",
				old.App,
				restoreErr,
			)
		} else if restoreErr := syncProxyRoutes(); restoreErr != nil {
			log.Printf(
				"failed restoring old proxy route for %s: %v",
				old.App,
				restoreErr,
			)
		}

		cleanupCandidate(true)

		return DeploymentRecord{}, fmt.Errorf(
			"switch proxy to candidate: %w",
			err,
		)
	}

	deploymentEvent(
		old.App,
		"Caddy switched traffic from port %d to port %d.",
		old.Port,
		candidatePort,
	)

	log.Printf(
		"Caddy cut over %s from port %d to healthy candidate port %d",
		old.App,
		old.Port,
		candidatePort,
	)

	// At this point new traffic is routed to the candidate.
	// Preserve the old deployment in history before retiring it.
	pruned, historyErr := historyStore.Push(old)
	if historyErr != nil {
		log.Printf(
			"warning: failed to save deployment history for %s: %v",
			old.App,
			historyErr,
		)
	} else {
		removePrunedHistoryImages(
			pruned,
			newImage,
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

	// Only after Caddy is serving the healthy candidate do we retire
	// the old container. Failure here does not interrupt the new app.
	if old.Container != "" &&
		old.Container != candidateContainer &&
		containerExists(old.Container) {

		if output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			old.Container,
		); err != nil {
			log.Printf(
				"warning: new deployment is live, but old container %s could not be removed: %v\n%s",
				old.Container,
				err,
				output,
			)
		} else {
			log.Printf(
				"Retired old container %s after successful proxy cutover",
				old.Container,
			)
		}
	}

	log.Printf(
		"Zero-downtime redeployment of %s completed: %d -> %d",
		old.App,
		old.Port,
		candidatePort,
	)

	deploymentEvent(
		old.App,
		"Zero-downtime redeployment completed successfully.",
	)

	return newRecord, nil
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
