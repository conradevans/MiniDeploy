package main

import (
	"fmt"
	"log"
	"time"
)

var (
	fullstackBuildDeploymentImage   = buildDeploymentImage
	fullstackVerifyStartup          = verifyContainerStartup
	fullstackVerifyHTTPHealth       = verifyHTTPHealthPath
	fullstackFindAvailablePort      = findAvailablePort
	fullstackSynchronizeProxyRoutes = syncProxyRoutes
)

func newFullstackReleaseRecord(
	app string,
	repoURL string,
	plan deploymentBuildPlan,
	release string,
	kind string,
	environment map[string]string,
) (DeploymentRecord, error) {
	if plan.Strategy != deploymentStrategyFullstackViteNode ||
		len(plan.Services) != 2 {

		return DeploymentRecord{}, fmt.Errorf("invalid full-stack build plan")
	}

	network, err := fullstackReleaseNetworkName(app, release)
	if err != nil {
		return DeploymentRecord{}, err
	}

	record := DeploymentRecord{
		App:                  app,
		RepoURL:              repoURL,
		Strategy:             deploymentStrategyFullstackViteNode,
		Network:              network,
		EnvironmentVariables: runtimeEnvironmentNames(environment),
		Services:             make([]DeploymentServiceRecord, 0, 2),
	}

	for _, name := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		servicePlan, ok := fullstackBuildServiceByName(plan, name)
		if !ok || servicePlan.Path != name {
			return DeploymentRecord{}, fmt.Errorf(
				"full-stack build plan is missing %s",
				name,
			)
		}

		container, err := fullstackServiceContainerName(
			app,
			name,
			kind,
			release,
		)
		if err != nil {
			return DeploymentRecord{}, err
		}
		image, err := fullstackServiceImageName(app, name, release)
		if err != nil {
			return DeploymentRecord{}, err
		}

		record.Services = append(record.Services, DeploymentServiceRecord{
			Name:                name,
			Path:                name,
			Strategy:            servicePlan.Build.Strategy,
			Container:           container,
			Image:               image,
			ContainerPort:       servicePlan.Build.ContainerPort,
			HealthPath:          servicePlan.Build.HealthPath,
			PackageManager:      servicePlan.Build.PackageManager,
			PackageInstallMode:  servicePlan.Build.PackageInstallMode,
			ReactorLabMigration: servicePlan.Build.ReactorLabMigration,
		})
	}

	record = normalizeDeploymentRecord(record)
	if err := validateFullstackActiveResources(record); err != nil {
		return DeploymentRecord{}, err
	}
	return record, nil
}

func buildFullstackRelease(
	record DeploymentRecord,
	plan deploymentBuildPlan,
) error {
	built := make([]DeploymentServiceRecord, 0, 2)
	for _, name := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, _ := deploymentServiceByName(record, name)
		servicePlan, _ := fullstackBuildServiceByName(plan, name)
		deploymentEvent(
			record.App,
			"Building %s service image...",
			name,
		)
		output, err := fullstackBuildDeploymentImage(
			servicePlan.RepositoryPath,
			service.Image,
			servicePlan.Build,
		)
		if err != nil {
			log.Printf("%s Docker build failed:\n%s", name, output)
			deploymentEvent(
				record.App,
				"ERROR: %s Docker build failed:\n%s",
				name,
				output,
			)
			for _, builtService := range built {
				_ = removeFullstackImage(
					record.App,
					builtService.Name,
					builtService.Image,
				)
			}
			return fmt.Errorf("build %s image: %w", name, err)
		}
		built = append(built, service)
		deploymentEvent(
			record.App,
			"%s image built successfully.",
			fullstackServiceDisplayName(name),
		)
	}
	return nil
}

func startAndVerifyFullstackRelease(
	record DeploymentRecord,
	environment map[string]string,
) (DeploymentRecord, error) {
	runtime, err := resolveDatabaseRuntime(record, environment)
	if err != nil {
		return record, err
	}
	backend, ok := deploymentServiceByName(record, fullstackBackendService)
	if !ok {
		return record, fmt.Errorf("missing backend service")
	}
	if err := runReactorLabMigration(record.App, backend.Image, backend.PackageManager, backend.ReactorLabMigration, runtime); err != nil {
		return record, err
	}

	if err := createFullstackNetwork(record.App, record.Network); err != nil {
		return record, err
	}

	// Start the backend first so its loopback port is reserved before the
	// frontend receives a second port. Neither port is routed by Caddy yet.
	for _, name := range []string{
		fullstackBackendService,
		fullstackFrontendService,
	} {
		index := deploymentServiceIndex(record.Services, name)
		if index < 0 {
			return record, fmt.Errorf("missing %s service", name)
		}
		port, err := fullstackFindAvailablePort(minDeployPort, maxDeployPort)
		if err != nil {
			return record, fmt.Errorf(
				"allocate %s host port: %w",
				name,
				err,
			)
		}
		record.Services[index].Port = port
		service := record.Services[index]

		serviceEnvironment := fullstackServiceRuntimeEnvironment(
			name,
			runtime.Environment,
		)
		if name == fullstackBackendService {
			if len(environment) > 0 {
				deploymentEvent(
					record.App,
					"Applying runtime environment to backend securely...",
				)
			}
		}

		deploymentEvent(
			record.App,
			"Starting %s service on loopback host port %d...",
			name,
			port,
		)
		options := managedContainerOptions{
			Network: record.Network,
			Service: name,
			App:     record.App,
		}
		if name == fullstackBackendService {
			options.DataNetwork = runtime.DataNetwork
		}
		if err := startManagedDeploymentContainerWithOptions(
			record.App,
			service.Container,
			service.Image,
			service.Port,
			service.ContainerPort,
			service.Strategy,
			serviceEnvironment,
			options,
		); err != nil {
			return record, fmt.Errorf(
				"start %s service: %w",
				name,
				err,
			)
		}
	}

	for _, name := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, _ := deploymentServiceByName(record, name)
		deploymentEvent(record.App, "Checking %s startup...", name)
		if err := fullstackVerifyStartup(service.Container); err != nil {
			logs, _ := containerLogs(
				service.Container,
				100,
				runtime.Redaction,
			)
			return record, fmt.Errorf(
				"%s failed startup verification: %w; logs: %s",
				name,
				err,
				logs,
			)
		}
		deploymentEvent(
			record.App,
			"Checking %s HTTP health at %s...",
			name,
			service.HealthPath,
		)
		if err := fullstackVerifyHTTPHealth(
			service.Port,
			service.HealthPath,
		); err != nil {
			logs, _ := containerLogs(
				service.Container,
				100,
				runtime.Redaction,
			)
			return record, fmt.Errorf(
				"%s failed HTTP health check: %w; logs: %s",
				name,
				err,
				logs,
			)
		}
	}

	return normalizeDeploymentRecord(record), nil
}

func deploymentServiceIndex(
	services []DeploymentServiceRecord,
	name string,
) int {
	for index, service := range services {
		if service.Name == name {
			return index
		}
	}
	return -1
}

func deployFullstackProject(
	app string,
	repoURL string,
	plan deploymentBuildPlan,
	environmentChange *runtimeEnvironmentChange,
) (DeploymentRecord, error) {
	release := fmt.Sprintf("%d", time.Now().UnixNano())
	record, err := newFullstackReleaseRecord(
		app,
		repoURL,
		plan,
		release,
		"release",
		environmentChange.effective,
	)
	if err != nil {
		return DeploymentRecord{}, err
	}

	if err := buildFullstackRelease(record, plan); err != nil {
		return DeploymentRecord{}, err
	}

	record, err = startAndVerifyFullstackRelease(
		record,
		environmentChange.effective,
	)
	if err != nil {
		_ = cleanupFullstackRelease(record, true)
		return DeploymentRecord{}, err
	}
	deploymentEvent(app, "Frontend and backend health checks passed.")

	if err := environmentChange.Commit(); err != nil {
		_ = cleanupFullstackRelease(record, true)
		return DeploymentRecord{}, fmt.Errorf(
			"save runtime environment: %w",
			err,
		)
	}

	if err := store.Save(record); err != nil {
		_ = environmentChange.Rollback()
		_ = cleanupFullstackRelease(record, true)
		return DeploymentRecord{}, fmt.Errorf(
			"save full-stack deployment metadata: %w",
			err,
		)
	}

	deploymentEvent(app, "Updating public route for the healthy project...")
	if err := fullstackSynchronizeProxyRoutes(); err != nil {
		if deleteErr := store.Delete(app); deleteErr != nil {
			log.Printf("failed removing failed full-stack metadata for %s: %v", app, deleteErr)
		} else if restoreErr := fullstackSynchronizeProxyRoutes(); restoreErr != nil {
			log.Printf("failed restoring proxy routes after %s cutover failure: %v", app, restoreErr)
		}
		_ = environmentChange.Rollback()
		_ = cleanupFullstackRelease(record, true)
		return DeploymentRecord{}, fmt.Errorf(
			"switch proxy to full-stack release: %w",
			err,
		)
	}

	deploymentEvent(app, "Full-stack deployment completed successfully.")
	return record, nil
}

func safeRedeployFullstackLocked(
	old DeploymentRecord,
	environmentReplacement map[string]string,
) (DeploymentRecord, error) {
	if err := validateFullstackActiveResources(old); err != nil {
		return DeploymentRecord{}, err
	}

	environmentChange, err := prepareRuntimeEnvironmentChange(
		old.App,
		environmentReplacement,
		environmentReplacement != nil,
	)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("prepare runtime environment: %w", err)
	}
	if err := validateManagedDatabaseEnvironment(old, environmentChange.effective); err != nil {
		return DeploymentRecord{}, err
	}
	if environmentReplacement == nil {
		if err := verifyRuntimeEnvironmentMetadata(
			old,
			environmentChange.effective,
		); err != nil {
			return DeploymentRecord{}, err
		}
	}

	resetDeploymentLog(old.App, "zero-downtime full-stack redeploy")
	deploymentEvent(old.App, "Current frontend and backend remain live.")
	release := fmt.Sprintf("%d", time.Now().UnixNano())
	candidatePath, err := managedDeploymentPath(
		old.App + "-candidate-" + release,
	)
	if err != nil {
		return DeploymentRecord{}, err
	}
	defer func() {
		if cleanupErr := removeManagedDeploymentPath(candidatePath); cleanupErr != nil {
			log.Printf("warning: clean full-stack candidate source: %v", cleanupErr)
		}
	}()

	deploymentEvent(old.App, "Cloning repository once for candidate project...")
	if output, err := runCommand(
		"",
		"git",
		"clone",
		"--",
		old.RepoURL,
		candidatePath,
	); err != nil {
		log.Printf("candidate git clone failed:\n%s", output)
		return DeploymentRecord{}, fmt.Errorf("git clone candidate: %w", err)
	}

	plan, err := persistedFullstackBuildPlan(old, candidatePath)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"prepare full-stack candidate strategy: %w",
			err,
		)
	}
	describeDeploymentPlan(old.App, plan)

	candidate, err := newFullstackReleaseRecord(
		old.App,
		old.RepoURL,
		plan,
		release,
		"candidate",
		environmentChange.effective,
	)
	if err != nil {
		return DeploymentRecord{}, err
	}
	if err := buildFullstackRelease(candidate, plan); err != nil {
		return DeploymentRecord{}, err
	}
	candidate.DatabaseAttachments = cloneDatabaseAttachments(old.DatabaseAttachments)
	candidate, err = startAndVerifyFullstackRelease(
		candidate,
		environmentChange.effective,
	)
	if err != nil {
		_ = cleanupFullstackRelease(candidate, true)
		return DeploymentRecord{}, err
	}

	if err := environmentChange.Commit(); err != nil {
		_ = cleanupFullstackRelease(candidate, true)
		return DeploymentRecord{}, fmt.Errorf(
			"save candidate runtime environment: %w",
			err,
		)
	}
	if err := store.Save(candidate); err != nil {
		_ = environmentChange.Rollback()
		_ = cleanupFullstackRelease(candidate, true)
		return DeploymentRecord{}, fmt.Errorf(
			"save candidate deployment metadata: %w",
			err,
		)
	}

	deploymentEvent(old.App, "Updating public route for paired candidate...")
	if err := fullstackSynchronizeProxyRoutes(); err != nil {
		if restoreErr := store.Save(old); restoreErr != nil {
			log.Printf("failed restoring old full-stack metadata for %s: %v", old.App, restoreErr)
		} else if restoreErr := fullstackSynchronizeProxyRoutes(); restoreErr != nil {
			log.Printf("failed restoring old full-stack route for %s: %v", old.App, restoreErr)
		}
		_ = environmentChange.Rollback()
		_ = cleanupFullstackRelease(candidate, true)
		return DeploymentRecord{}, fmt.Errorf(
			"switch proxy to full-stack candidate: %w",
			err,
		)
	}

	pruned, historyErr := historyStore.Push(old)
	if historyErr != nil {
		log.Printf("warning: save full-stack history for %s: %v", old.App, historyErr)
	} else {
		removePrunedHistoryImages(
			pruned,
			deploymentImages(candidate)...,
		)
	}

	currentPath, pathErr := managedDeploymentPath(old.App)
	if pathErr == nil {
		if err := replaceDeploymentSource(currentPath, candidatePath); err != nil {
			log.Printf("warning: replace full-stack source for %s: %v", old.App, err)
		}
	}
	if err := cleanupFullstackRelease(old, false); err != nil {
		log.Printf("warning: retire previous full-stack release %s: %v", old.App, err)
	}

	deploymentEvent(old.App, "Full-stack redeployment completed successfully.")
	return candidate, nil
}

func rollbackFullstackLocked(
	current DeploymentRecord,
) (DeploymentRecord, error) {
	if err := validateFullstackActiveResources(current); err != nil {
		return DeploymentRecord{}, err
	}
	environment, err := runtimeEnvironmentStore.Load(current.App)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("load current runtime environment: %w", err)
	}
	if err := verifyRuntimeEnvironmentMetadata(current, environment); err != nil {
		return DeploymentRecord{}, err
	}

	versions, err := historyStore.List(current.App)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf("load deployment history: %w", err)
	}
	if len(versions) == 0 {
		return DeploymentRecord{}, ErrNoRollbackVersion
	}

	previous := versions[0].RecordWithFallback(current)
	if err := validateFullstackServiceMetadata(previous.Services); err != nil {
		return DeploymentRecord{}, fmt.Errorf("invalid full-stack history: %w", err)
	}
	for _, service := range previous.Services {
		if err := validateFullstackImageRecord(
			current.App,
			service.Name,
			service.Image,
		); err != nil {
			return DeploymentRecord{}, err
		}
		if _, err := runCommand(
			"",
			"docker",
			"image",
			"inspect",
			service.Image,
		); err != nil {
			return DeploymentRecord{}, fmt.Errorf(
				"previous %s image is unavailable: %w",
				service.Name,
				err,
			)
		}
	}

	resetDeploymentLog(current.App, "zero-downtime full-stack rollback")
	deploymentEvent(current.App, "Current frontend and backend remain live.")
	release := fmt.Sprintf("%d", time.Now().UnixNano())
	network, err := fullstackReleaseNetworkName(current.App, release)
	if err != nil {
		return DeploymentRecord{}, err
	}
	candidate := previous
	candidate.App = current.App
	candidate.RepoURL = current.RepoURL
	candidate.Network = network
	candidate.EnvironmentVariables = runtimeEnvironmentNames(environment)
	candidate.DatabaseAttachments = cloneDatabaseAttachments(current.DatabaseAttachments)
	for index := range candidate.Services {
		service := &candidate.Services[index]
		service.Container, err = fullstackServiceContainerName(
			current.App,
			service.Name,
			"rollback",
			release,
		)
		if err != nil {
			return DeploymentRecord{}, err
		}
		service.Port = 0
	}
	candidate = normalizeDeploymentRecord(candidate)
	if err := validateFullstackActiveResources(candidate); err != nil {
		return DeploymentRecord{}, err
	}

	candidate, err = startAndVerifyFullstackRelease(candidate, environment)
	if err != nil {
		_ = cleanupFullstackRelease(candidate, false)
		return DeploymentRecord{}, err
	}
	if err := store.Save(candidate); err != nil {
		_ = cleanupFullstackRelease(candidate, false)
		return DeploymentRecord{}, fmt.Errorf(
			"save rollback candidate metadata: %w",
			err,
		)
	}
	if err := fullstackSynchronizeProxyRoutes(); err != nil {
		if restoreErr := store.Save(current); restoreErr != nil {
			log.Printf("failed restoring current full-stack metadata for %s: %v", current.App, restoreErr)
		} else if restoreErr := fullstackSynchronizeProxyRoutes(); restoreErr != nil {
			log.Printf("failed restoring current full-stack route for %s: %v", current.App, restoreErr)
		}
		_ = cleanupFullstackRelease(candidate, false)
		return DeploymentRecord{}, fmt.Errorf(
			"switch proxy to full-stack rollback candidate: %w",
			err,
		)
	}

	updatedHistory := append(
		[]DeploymentVersion{deploymentVersion(current)},
		versions[1:]...,
	)
	pruned, historyErr := historyStore.Set(current.App, updatedHistory)
	if historyErr != nil {
		log.Printf("warning: update full-stack rollback history for %s: %v", current.App, historyErr)
	} else {
		removePrunedHistoryImages(
			pruned,
			deploymentImages(candidate)...,
		)
	}
	if err := cleanupFullstackRelease(current, false); err != nil {
		log.Printf("warning: retire pre-rollback full-stack release %s: %v", current.App, err)
	}

	deploymentEvent(current.App, "Full-stack rollback completed successfully.")
	return candidate, nil
}
