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

	if err := startManagedContainerWithPort(
		containerName,
		imageName,
		port,
		containerPort,
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

	return record, nil
}

func safeRedeploy(
	old DeploymentRecord,
) (DeploymentRecord, error) {
	old = normalizeDeploymentRecord(old)

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
			"%d:%d",
			candidatePort,
			old.ContainerPort,
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

	if err := verifyHTTPHealthPath(
		candidatePort,
		old.HealthPath,
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
			"candidate failed HTTP health check: %w; logs: %s",
			err,
			logs,
		)
	}

	log.Printf(
		"Candidate for %s passed startup and HTTP health verification",
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

	if err := startManagedContainerWithPort(
		old.Container,
		newImage,
		old.Port,
		old.ContainerPort,
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

	if err := verifyHTTPHealthPath(
		old.Port,
		old.HealthPath,
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
			"new live container failed HTTP health check: %w; logs: %s",
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

	log.Printf(
		"Safe redeployment of %s completed",
		old.App,
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
