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

	log.Printf(
		"Starting zero-downtime rollback candidate for %s while current version stays live",
		current.App,
	)

	if err := startManagedContainerWithPort(
		candidateName,
		previous.Image,
		candidatePort,
		current.ContainerPort,
	); err != nil {
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

	log.Printf(
		"Rollback candidate for %s is healthy on port %d; current version remains live on port %d",
		current.App,
		candidatePort,
		current.Port,
	)

	newRecord := DeploymentRecord{
		App:           current.App,
		RepoURL:       previous.RepoURL,
		Container:     candidateName,
		Image:         previous.Image,
		Port:          candidatePort,
		ContainerPort: current.ContainerPort,
		HealthPath:    current.HealthPath,
	}

	// Make the healthy rollback candidate the desired active deployment.
	// The current container is deliberately still running.
	if err := store.Save(newRecord); err != nil {
		cleanupCandidate()

		return DeploymentRecord{}, fmt.Errorf(
			"save rollback candidate metadata: %w",
			err,
		)
	}

	// Atomically switch Caddy to the healthy rollback candidate.
	if err := syncProxyRoutes(); err != nil {
		log.Printf(
			"rollback proxy cutover failed for %s; restoring current route: %v",
			current.App,
			err,
		)

		if restoreErr := store.Save(current); restoreErr != nil {
			log.Printf(
				"failed restoring current metadata for %s: %v",
				current.App,
				restoreErr,
			)
		} else if restoreErr := syncProxyRoutes(); restoreErr != nil {
			log.Printf(
				"failed restoring current proxy route for %s: %v",
				current.App,
				restoreErr,
			)
		}

		cleanupCandidate()

		return DeploymentRecord{}, fmt.Errorf(
			"switch proxy to rollback candidate: %w",
			err,
		)
	}

	log.Printf(
		"Caddy cut over rollback of %s from port %d to healthy candidate port %d",
		current.App,
		current.Port,
		candidatePort,
	)

	// Put the version we just left at the front of history.
	// This also means another rollback can move back to it.
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

	// New traffic is already reaching the rollback candidate,
	// so it is now safe to retire the formerly active container.
	if current.Container != "" &&
		current.Container != candidateName &&
		containerExists(current.Container) {

		if output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			current.Container,
		); err != nil {
			log.Printf(
				"warning: rollback is live, but old container %s could not be removed: %v\n%s",
				current.Container,
				err,
				output,
			)
		} else {
			log.Printf(
				"Retired pre-rollback container %s after successful proxy cutover",
				current.Container,
			)
		}
	}

	log.Printf(
		"Zero-downtime rollback of %s completed: port %d -> %d, image %s",
		current.App,
		current.Port,
		candidatePort,
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
