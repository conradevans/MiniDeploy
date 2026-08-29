package main

import (
	"errors"
	"fmt"
	"log"
	"time"
)

var ErrNoRollbackVersion = errors.New(
	"no rollback version available",
)

func restorePreviousContainer(
	record DeploymentRecord,
	shouldRun bool,
) error {
	record = normalizeDeploymentRecord(record)
	if record.Image == "" {
		return fmt.Errorf(
			"previous image is unknown",
		)
	}

	if err := startManagedContainerWithPort(
		record.Container,
		record.Image,
		record.Port,
		record.ContainerPort,
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

func rollbackDeployment(
	current DeploymentRecord,
) (DeploymentRecord, error) {
	current = normalizeDeploymentRecord(current)

	deployMu.Lock()
	defer deployMu.Unlock()

	versions, err := historyStore.List(current.App)

	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"load deployment history: %w",
			err,
		)
	}

	if len(versions) == 0 {
		return DeploymentRecord{}, ErrNoRollbackVersion
	}

	previous := versions[0]

	if _, err := runCommand(
		"",
		"docker",
		"image",
		"inspect",
		previous.Image,
	); err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"previous Docker image is unavailable: %w",
			err,
		)
	}

	versionID := fmt.Sprintf(
		"%d",
		time.Now().UnixNano(),
	)

	candidateName := fmt.Sprintf(
		"minideploy-%s-rollback-%s",
		current.App,
		versionID,
	)

	candidatePort, err := findAvailablePort(
		minDeployPort,
		maxDeployPort,
	)

	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"allocate rollback candidate port: %w",
			err,
		)
	}

	output, err := runCommand(
		"",
		"docker",
		"run",
		"-d",
		"--name",
		candidateName,
		"-p",
		fmt.Sprintf(
			"%d:%d",
			candidatePort,
			current.ContainerPort,
		),
		previous.Image,
	)

	if err != nil {
		log.Printf(
			"rollback candidate failed to start:\n%s",
			output,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"start rollback candidate: %w",
			err,
		)
	}

	cleanupCandidate := func() {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			candidateName,
		)
	}

	if err := verifyContainerStartup(
		candidateName,
	); err != nil {
		logs, _ := containerLogs(
			candidateName,
			100,
		)

		cleanupCandidate()

		return DeploymentRecord{}, fmt.Errorf(
			"rollback candidate failed startup verification: %w; logs: %s",
			err,
			logs,
		)
	}

	if err := verifyHTTPHealthPath(
		candidatePort,
		current.HealthPath,
	); err != nil {
		logs, _ := containerLogs(
			candidateName,
			100,
		)

		cleanupCandidate()

		return DeploymentRecord{}, fmt.Errorf(
			"rollback candidate failed HTTP verification: %w; logs: %s",
			err,
			logs,
		)
	}

	cleanupCandidate()

	currentWasRunning :=
		containerStatus(current.Container) == "running"

	if containerExists(current.Container) {
		if output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			current.Container,
		); err != nil {
			log.Printf(
				"failed to remove current container:\n%s",
				output,
			)

			return DeploymentRecord{}, fmt.Errorf(
				"remove current container: %w",
				err,
			)
		}
	}

	if err := startManagedContainerWithPort(
		current.Container,
		previous.Image,
		current.Port,
		current.ContainerPort,
	); err != nil {
		restoreErr := restorePreviousContainer(
			current,
			currentWasRunning,
		)

		if restoreErr != nil {
			log.Printf(
				"failed to restore current deployment: %v",
				restoreErr,
			)
		}

		return DeploymentRecord{}, fmt.Errorf(
			"start rollback version: %w",
			err,
		)
	}

	if err := verifyContainerStartup(
		current.Container,
	); err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			current.Container,
		)

		restoreErr := restorePreviousContainer(
			current,
			currentWasRunning,
		)

		if restoreErr != nil {
			log.Printf(
				"failed to restore current deployment: %v",
				restoreErr,
			)
		}

		return DeploymentRecord{}, fmt.Errorf(
			"rolled-back container failed startup verification: %w",
			err,
		)
	}

	if err := verifyHTTPHealthPath(
		current.Port,
		current.HealthPath,
	); err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			current.Container,
		)

		restoreErr := restorePreviousContainer(
			current,
			currentWasRunning,
		)

		if restoreErr != nil {
			log.Printf(
				"failed to restore current deployment: %v",
				restoreErr,
			)
		}

		return DeploymentRecord{}, fmt.Errorf(
			"rolled-back container failed HTTP verification: %w",
			err,
		)
	}

	newRecord := DeploymentRecord{
		App:           current.App,
		RepoURL:       previous.RepoURL,
		Container:     current.Container,
		Image:         previous.Image,
		Port:          current.Port,
		ContainerPort: current.ContainerPort,
		HealthPath:    current.HealthPath,
	}

	if err := store.Save(newRecord); err != nil {
		_, _ = runCommand(
			"",
			"docker",
			"rm",
			"-f",
			current.Container,
		)

		restoreErr := restorePreviousContainer(
			current,
			currentWasRunning,
		)

		if restoreErr != nil {
			log.Printf(
				"failed to restore current deployment: %v",
				restoreErr,
			)
		}

		return DeploymentRecord{}, fmt.Errorf(
			"save rollback metadata: %w",
			err,
		)
	}

	updatedHistory := append(
		[]DeploymentVersion{
			deploymentVersion(current),
		},
		versions[1:]...,
	)

	pruned, historyErr := historyStore.Set(
		current.App,
		updatedHistory,
	)

	if historyErr != nil {
		log.Printf(
			"warning: rollback succeeded but history update failed for %s: %v",
			current.App,
			historyErr,
		)
	} else {
		removePrunedHistoryImages(
			pruned,
			newRecord.Image,
		)
	}

	log.Printf(
		"Rolled back %s to image %s",
		current.App,
		previous.Image,
	)

	return newRecord, nil
}

func removePrunedHistoryImages(
	versions []DeploymentVersion,
	keepImages ...string,
) {
	keep := make(map[string]bool)

	for _, image := range keepImages {
		keep[image] = true
	}

	for _, version := range versions {
		if version.Image == "" ||
			keep[version.Image] {
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
				"warning: failed to prune old image %s:\n%s",
				version.Image,
				output,
			)
		}
	}
}
