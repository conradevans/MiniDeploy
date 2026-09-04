package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	initialBuildDeploymentImage = buildDeploymentImage
	initialFindAvailablePort    = findAvailablePort
	initialVerifyStartup        = verifyContainerStartup
	initialVerifyHTTPHealth     = verifyHTTPHealthPath
	initialSynchronizeProxy     = syncProxyRoutes
)

func initialAttachmentMetadataRequiresRecovery(
	app string,
	attachment DatabaseAttachmentRecord,
) bool {
	_, err := store.Get(app)
	if errors.Is(err, ErrDeploymentNotFound) {
		return false
	}

	if err != nil {
		log.Printf(
			"failed checking deployment metadata after initial database deployment failure for %s; attachment %s for database %s was retained for recovery",
			app,
			attachment.AttachmentID,
			attachment.DatabaseID,
		)
	} else {
		log.Printf(
			"failed initial database deployment metadata remains for %s; attachment %s for database %s was retained for recovery",
			app,
			attachment.AttachmentID,
			attachment.DatabaseID,
		)
	}
	deploymentEvent(
		app,
		"ERROR: deployment rollback could not prove metadata removal; attachment %s for database %s was retained for recovery.",
		attachment.AttachmentID,
		attachment.DatabaseID,
	)
	return true
}

func deployRepository(
	repoURL string,
	containerPort int,
	healthPath string,
	environment map[string]string,
	databaseID string,
) (deployed DeploymentRecord, deployErr error) {
	deployMu.Lock()
	defer deployMu.Unlock()

	if err := validateRequestedDeploymentConfig(
		containerPort,
		healthPath,
	); err != nil {
		return DeploymentRecord{}, err
	}

	if databaseID != "" {
		if !miniBaseDatabaseIDPattern.MatchString(databaseID) {
			return DeploymentRecord{}, fmt.Errorf("invalid MiniBase database ID")
		}
		if _, conflict := environment["DATABASE_URL"]; conflict {
			return DeploymentRecord{}, fmt.Errorf(
				"DATABASE_URL cannot be supplied with a managed MiniBase database",
			)
		}
	}

	appName := repoName(repoURL)

	if appName == "" {
		return DeploymentRecord{}, fmt.Errorf("invalid repository URL")
	}

	environmentChange, err := prepareRuntimeEnvironmentChange(
		appName,
		environment,
		true,
	)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"prepare runtime environment: %w",
			err,
		)
	}

	resetDeploymentLog(appName, "deployment")
	deploymentEvent(
		appName,
		"Preparing deployment.",
	)

	deployPath, err := managedDeploymentPath(appName)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"resolve deployment path: %w",
			err,
		)
	}

	imageName := versionedImageName(appName)
	containerName := "minideploy-" + appName

	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"create deployment directory: %w",
			err,
		)
	}

	if err := removeManagedDeploymentPath(deployPath); err != nil {
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
		"--",
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
	deploymentEvent(appName, "Inspecting repository...")

	plan, err := detectDeploymentStrategy(
		deployPath,
		deploymentConfig{
			ContainerPort: containerPort,
			HealthPath:    healthPath,
		},
	)
	if err != nil {
		deploymentEvent(
			appName,
			"ERROR: deployment strategy detection failed: %v",
			err,
		)

		return DeploymentRecord{}, err
	}

	initialAttachment, err := createInitialMiniBaseAttachment(
		appName,
		databaseID,
		plan,
	)
	if err != nil {
		return DeploymentRecord{}, err
	}
	cleanupInitialAttachment := initialAttachment.AttachmentID != ""
	if initialAttachment.AttachmentID != "" {
		defer func() {
			if deployErr == nil || !cleanupInitialAttachment {
				return
			}
			if cleanupErr := deleteInitialMiniBaseAttachment(initialAttachment); cleanupErr != nil {
				log.Printf(
					"automatic initial MiniBase attachment cleanup failed for %s (attachment=%s database=%s)",
					appName,
					initialAttachment.AttachmentID,
					initialAttachment.DatabaseID,
				)
				deploymentEvent(
					appName,
					"ERROR: automatic database attachment cleanup failed; attachment %s for database %s requires manual detachment.",
					initialAttachment.AttachmentID,
					initialAttachment.DatabaseID,
				)
				deployErr = fmt.Errorf("%w; automatic database attachment cleanup failed", deployErr)
			}
		}()
	}

	var databaseAttachments []DatabaseAttachmentRecord
	if initialAttachment.AttachmentID != "" {
		databaseAttachments = []DatabaseAttachmentRecord{initialAttachment}
	}
	if plan.Strategy == deploymentStrategyFullstackViteNode {
		describeDeploymentPlan(appName, plan)
		record, err := deployFullstackProject(
			appName,
			repoURL,
			plan,
			environmentChange,
			databaseAttachments,
		)
		if err != nil {
			if initialAttachment.AttachmentID != "" &&
				initialAttachmentMetadataRequiresRecovery(appName, initialAttachment) {

				cleanupInitialAttachment = false
			}
			if cleanupErr := removeManagedDeploymentPath(deployPath); cleanupErr != nil {
				log.Printf("warning: clean failed full-stack checkout: %v", cleanupErr)
			}
			return DeploymentRecord{}, err
		}
		return record, nil
	}

	containerPort = plan.ContainerPort
	healthPath = plan.HealthPath

	if err := validateDeploymentConfig(
		containerPort,
		healthPath,
	); err != nil {
		return DeploymentRecord{}, err
	}

	describeDeploymentPlan(appName, plan)
	deploymentEvent(
		appName,
		"Selected strategy %s: container port %d, health path %s",
		plan.Strategy,
		containerPort,
		healthPath,
	)
	deploymentEvent(appName, "Building Docker image...")

	if output, err := initialBuildDeploymentImage(
		deployPath,
		imageName,
		plan,
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

	port, err := initialFindAvailablePort(
		minDeployPort,
		maxDeployPort,
	)

	if err != nil {
		return DeploymentRecord{}, err
	}

	record := DeploymentRecord{
		App:                 appName,
		RepoURL:             repoURL,
		Container:           containerName,
		Image:               imageName,
		Port:                port,
		ContainerPort:       containerPort,
		HealthPath:          healthPath,
		Strategy:            plan.Strategy,
		PackageManager:      plan.PackageManager,
		PackageInstallMode:  plan.PackageInstallMode,
		ReactorLabMigration: plan.ReactorLabMigration,
		EnvironmentVariables: runtimeEnvironmentNames(
			environmentChange.effective,
		),
	}
	if initialAttachment.AttachmentID != "" {
		record.DatabaseAttachments = []DatabaseAttachmentRecord{initialAttachment}
	}

	runtime := databaseRuntime{
		Environment: cloneRuntimeEnvironment(environmentChange.effective),
		Redaction:   cloneRuntimeEnvironment(environmentChange.effective),
	}
	if initialAttachment.AttachmentID != "" {
		runtime, err = resolveDatabaseRuntime(record, environmentChange.effective)
		if err != nil {
			return DeploymentRecord{}, err
		}
		if err := runReactorLabMigration(
			appName,
			imageName,
			plan.PackageManager,
			plan.ReactorLabMigration,
			runtime,
		); err != nil {
			return DeploymentRecord{}, err
		}
	}

	deploymentEvent(
		appName,
		"Starting container on host port %d...",
		port,
	)

	if len(runtime.Environment) > 0 ||
		plan.Strategy == deploymentStrategyNodeExpress {

		deploymentEvent(
			appName,
			"Applying runtime environment securely...",
		)
	}

	if err := startManagedDeploymentContainerWithOptions(
		appName,
		containerName,
		imageName,
		port,
		containerPort,
		plan.Strategy,
		runtime.Environment,
		managedContainerOptions{DataNetwork: runtime.DataNetwork},
	); err != nil {
		return DeploymentRecord{}, err
	}

	deploymentEvent(appName, "Container started. Verifying startup...")

	if err := initialVerifyStartup(containerName); err != nil {
		logs, _ := containerLogs(
			containerName,
			100,
			runtime.Redaction,
		)

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

	if err := initialVerifyHTTPHealth(
		port,
		healthPath,
	); err != nil {
		logs, _ := containerLogs(
			containerName,
			100,
			runtime.Redaction,
		)

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

	if err := environmentChange.Commit(); err != nil {
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
			"save runtime environment: %w",
			err,
		)
	}

	if err := store.Save(record); err != nil {
		if rollbackErr := environmentChange.Rollback(); rollbackErr != nil {
			log.Printf(
				"failed restoring runtime environment for %s: %v",
				appName,
				rollbackErr,
			)
		}

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

	if initialAttachment.AttachmentID != "" {
		deploymentEvent(appName, "Updating public route for the healthy deployment...")
		if err := initialSynchronizeProxy(); err != nil {
			if deleteErr := store.Delete(appName); deleteErr != nil {
				cleanupInitialAttachment = false
				log.Printf(
					"initial proxy sync failed for %s and deployment metadata could not be removed; attachment %s for database %s was retained for recovery",
					appName,
					initialAttachment.AttachmentID,
					initialAttachment.DatabaseID,
				)
				deploymentEvent(
					appName,
					"ERROR: public route update and automatic rollback failed; attachment %s for database %s was retained for recovery.",
					initialAttachment.AttachmentID,
					initialAttachment.DatabaseID,
				)
				return DeploymentRecord{}, fmt.Errorf(
					"synchronize proxy for initial database deployment; rollback metadata removal failed",
				)
			}

			if restoreErr := initialSynchronizeProxy(); restoreErr != nil {
				log.Printf(
					"warning: failed restoring proxy routes after initial database deployment failure for %s",
					appName,
				)
			}
			if rollbackErr := environmentChange.Rollback(); rollbackErr != nil {
				log.Printf(
					"warning: failed restoring runtime environment after initial database deployment failure for %s",
					appName,
				)
			}
			_, _ = runCommand("", "docker", "rm", "-f", containerName)
			_, _ = runCommand("", "docker", "image", "rm", imageName)
			if cleanupErr := removeManagedDeploymentPath(deployPath); cleanupErr != nil {
				log.Printf(
					"warning: failed cleaning source after initial database deployment failure for %s",
					appName,
				)
			}
			return DeploymentRecord{}, fmt.Errorf(
				"synchronize proxy for initial database deployment",
			)
		}
	}

	deploymentEvent(
		appName,
		"Container is healthy and deployment metadata is saved.",
	)

	return record, nil
}

func safeRedeploy(
	old DeploymentRecord,
	environmentReplacement map[string]string,
) (DeploymentRecord, error) {
	old = normalizeDeploymentRecord(old)

	if old.DatabaseDetached {
		return DeploymentRecord{}, ErrDatabaseDetached
	}

	deployMu.Lock()
	defer deployMu.Unlock()

	if old.Strategy == deploymentStrategyFullstackViteNode {
		return safeRedeployFullstackLocked(old, environmentReplacement)
	}

	environmentChange, err := prepareRuntimeEnvironmentChange(
		old.App,
		environmentReplacement,
		environmentReplacement != nil,
	)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"prepare runtime environment: %w",
			err,
		)
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

	currentPath, err := managedDeploymentPath(old.App)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"resolve current deployment path: %w",
			err,
		)
	}

	candidatePath, err := managedDeploymentPath(
		old.App + "-candidate-" + version,
	)
	if err != nil {
		return DeploymentRecord{}, fmt.Errorf(
			"resolve candidate deployment path: %w",
			err,
		)
	}

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

	defer func() {
		if err := removeManagedDeploymentPath(
			candidatePath,
		); err != nil {
			log.Printf(
				"warning: failed to clean candidate source path %s: %v",
				candidatePath,
				err,
			)
		}
	}()

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
		"--",
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
		"Repository cloned. Reusing persisted %s strategy...",
		old.Strategy,
	)

	plan, err := deploymentStrategyForRecord(
		old,
		candidatePath,
	)
	if err != nil {
		deploymentEvent(
			old.App,
			"ERROR: persisted deployment strategy is unusable: %v",
			err,
		)

		return DeploymentRecord{}, fmt.Errorf(
			"prepare candidate strategy: %w",
			err,
		)
	}

	describeDeploymentPlan(old.App, plan)
	deploymentEvent(
		old.App,
		"Building candidate Docker image...",
	)

	if output, err := buildDeploymentImage(
		candidatePath,
		newImage,
		plan,
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

	runtimeRecord := old
	runtimeRecord.Strategy = plan.Strategy
	runtimeRecord.ContainerPort = plan.ContainerPort
	runtimeRecord.HealthPath = plan.HealthPath
	runtimeRecord.PackageManager = plan.PackageManager
	runtimeRecord.ReactorLabMigration = plan.ReactorLabMigration
	runtime, err := resolveDatabaseRuntime(runtimeRecord, environmentChange.effective)
	if err != nil {
		_, _ = runCommand("", "docker", "image", "rm", newImage)
		return DeploymentRecord{}, err
	}
	if err := runReactorLabMigration(
		old.App,
		newImage,
		plan.PackageManager,
		plan.ReactorLabMigration,
		runtime,
	); err != nil {
		_, _ = runCommand("", "docker", "image", "rm", newImage)
		return DeploymentRecord{}, err
	}

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

	if len(environmentChange.effective) > 0 ||
		plan.Strategy == deploymentStrategyNodeExpress {

		deploymentEvent(
			old.App,
			"Applying runtime environment to candidate securely...",
		)
	}

	if err := startManagedDeploymentContainerWithOptions(
		old.App,
		candidateContainer,
		newImage,
		candidatePort,
		plan.ContainerPort,
		plan.Strategy,
		runtime.Environment,
		managedContainerOptions{DataNetwork: runtime.DataNetwork},
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
			runtime.Redaction,
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
		plan.HealthPath,
	); err != nil {
		logs, _ := containerLogs(
			candidateContainer,
			100,
			runtime.Redaction,
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
	newRecord.ContainerPort = plan.ContainerPort
	newRecord.HealthPath = plan.HealthPath
	newRecord.Strategy = plan.Strategy
	newRecord.PackageManager = plan.PackageManager
	newRecord.PackageInstallMode = plan.PackageInstallMode
	newRecord.ReactorLabMigration = plan.ReactorLabMigration
	newRecord.EnvironmentVariables = runtimeEnvironmentNames(
		environmentChange.effective,
	)

	if err := environmentChange.Commit(); err != nil {
		cleanupCandidate(true)

		return DeploymentRecord{}, fmt.Errorf(
			"save candidate runtime environment: %w",
			err,
		)
	}

	// Persist the candidate as the desired active deployment.
	// The old container is deliberately still running here.
	if err := store.Save(newRecord); err != nil {
		if rollbackErr := environmentChange.Rollback(); rollbackErr != nil {
			log.Printf(
				"failed restoring runtime environment for %s: %v",
				old.App,
				rollbackErr,
			)
		}

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

		if rollbackErr := environmentChange.Rollback(); rollbackErr != nil {
			log.Printf(
				"failed restoring runtime environment for %s: %v",
				old.App,
				rollbackErr,
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
	currentPath, err := strictChildPath(
		deploymentsDir,
		currentPath,
	)
	if err != nil {
		return fmt.Errorf(
			"validate current deployment path: %w",
			err,
		)
	}

	candidatePath, err = strictChildPath(
		deploymentsDir,
		candidatePath,
	)
	if err != nil {
		return fmt.Errorf(
			"validate candidate deployment path: %w",
			err,
		)
	}

	if err := removeManagedDeploymentPath(currentPath); err != nil {
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

	// Repository names are user-controlled input and become identifiers for
	// Docker, Caddy, managed files, and persisted deployment metadata. Keep
	// the submitted repository URL unchanged, but derive one canonical app
	// identifier before validation or resource construction.
	name = strings.ToLower(name)

	if validateApplicationName(name) != nil {
		return ""
	}

	return name
}
