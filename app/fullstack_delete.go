package main

import (
	"fmt"
	"log"
)

func deleteFullstackProject(record DeploymentRecord) error {
	record = normalizeDeploymentRecord(record)
	if err := validateFullstackActiveResources(record); err != nil {
		return err
	}

	deployPath, err := managedDeploymentPath(record.App)
	if err != nil {
		return fmt.Errorf("resolve deployment source path: %w", err)
	}
	if _, err := runtimeEnvironmentStore.path(record.App); err != nil {
		return fmt.Errorf("resolve runtime environment path: %w", err)
	}

	versions, err := historyStore.List(record.App)
	if err != nil {
		return fmt.Errorf("load full-stack history for delete: %w", err)
	}
	for _, version := range versions {
		historical := version.RecordWithFallback(record)
		if err := validateFullstackServiceMetadata(historical.Services); err != nil {
			return fmt.Errorf("validate historical full-stack release: %w", err)
		}
		for _, service := range historical.Services {
			if err := validateFullstackImageRecord(
				record.App,
				service.Name,
				service.Image,
			); err != nil {
				return err
			}
		}
	}

	if err := cleanupFullstackRelease(record, false); err != nil {
		return fmt.Errorf("remove full-stack containers/network: %w", err)
	}
	if err := removeManagedDeploymentPath(deployPath); err != nil {
		return fmt.Errorf("remove deployment source: %w", err)
	}

	activeImages := make(map[string]bool)
	for _, image := range deploymentImages(record) {
		activeImages[image] = true
	}
	for _, version := range versions {
		historical := version.RecordWithFallback(record)
		for _, service := range historical.Services {
			if activeImages[service.Image] {
				continue
			}
			if err := removeFullstackImage(
				record.App,
				service.Name,
				service.Image,
			); err != nil {
				log.Printf(
					"warning: remove historical %s image %s: %v",
					service.Name,
					service.Image,
					err,
				)
			}
		}
	}
	if err := historyStore.Clear(record.App); err != nil {
		log.Printf("warning: clear full-stack history for %s: %v", record.App, err)
	}

	for _, service := range record.Services {
		if err := removeFullstackImage(
			record.App,
			service.Name,
			service.Image,
		); err != nil {
			log.Printf(
				"warning: remove active %s image %s: %v",
				service.Name,
				service.Image,
				err,
			)
		}
	}
	if err := detachMiniBaseAttachment(record); err != nil {
		return fmt.Errorf("application resources removed but MiniBase attachment cleanup failed: %w", err)
	}

	if err := runtimeEnvironmentStore.Delete(record.App); err != nil {
		return fmt.Errorf("remove runtime environment: %w", err)
	}
	if err := store.Delete(record.App); err != nil {
		return fmt.Errorf("remove deployment metadata: %w", err)
	}
	if err := fullstackSynchronizeProxyRoutes(); err != nil {
		return fmt.Errorf("synchronize proxy after delete: %w", err)
	}
	return nil
}
