package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInitialDatabaseUnsupported  = errors.New("deployment strategy does not support MiniBase")
	ErrMiniBaseDatabaseUnavailable = errors.New("MiniBase database is unavailable for attachment")
	ErrMiniBaseOperation           = errors.New("MiniBase operation failed")
	ErrDatabaseDetached            = errors.New("deployment database is detached")
)

type databaseRuntime struct {
	Environment map[string]string
	Redaction   map[string]string
	DataNetwork string
}

func cloneDatabaseAttachments(attachments []DatabaseAttachmentRecord) []DatabaseAttachmentRecord {
	if len(attachments) == 0 {
		return nil
	}
	cloned := make([]DatabaseAttachmentRecord, len(attachments))
	copy(cloned, attachments)
	return cloned
}

func currentDatabaseAttachment(record DeploymentRecord) (DatabaseAttachmentRecord, bool, error) {
	if len(record.DatabaseAttachments) == 0 {
		return DatabaseAttachmentRecord{}, false, nil
	}
	if len(record.DatabaseAttachments) != 1 {
		return DatabaseAttachmentRecord{}, false, fmt.Errorf("deployment must have at most one primary database attachment")
	}
	attachment := record.DatabaseAttachments[0]
	if !miniBaseAttachmentIDPattern.MatchString(attachment.AttachmentID) ||
		!miniBaseDatabaseIDPattern.MatchString(attachment.DatabaseID) ||
		attachment.BindingName != miniBaseBindingPrimary ||
		strings.TrimSpace(attachment.DisplayName) != attachment.DisplayName ||
		attachment.DisplayName == "" || len(attachment.DisplayName) > 800 {
		return DatabaseAttachmentRecord{}, false, fmt.Errorf("deployment contains invalid database attachment metadata")
	}
	return attachment, true, nil
}

func deploymentSupportsMiniBase(record DeploymentRecord) bool {
	record = normalizeDeploymentRecord(record)
	switch record.Strategy {
	case deploymentStrategyNodeExpress:
		return true
	case deploymentStrategyFullstackViteNode:
		backend, ok := deploymentServiceByName(record, fullstackBackendService)
		return ok && backend.Strategy == deploymentStrategyNodeExpress
	default:
		return false
	}
}

func deploymentPlanSupportsMiniBase(plan deploymentBuildPlan) bool {
	switch plan.Strategy {
	case deploymentStrategyNodeExpress:
		return true
	case deploymentStrategyFullstackViteNode:
		backend, ok := fullstackBuildServiceByName(plan, fullstackBackendService)
		return ok && backend.Build.Strategy == deploymentStrategyNodeExpress
	default:
		return false
	}
}

func availableExistingMiniBaseDatabase(
	ctx context.Context,
	databaseID string,
) (miniBaseDatabase, error) {
	if !miniBaseDatabaseIDPattern.MatchString(databaseID) {
		return miniBaseDatabase{}, ErrMiniBaseDatabaseUnavailable
	}

	databases, err := miniBaseClient.ListDatabases(ctx)
	if err != nil {
		return miniBaseDatabase{}, ErrMiniBaseOperation
	}

	for _, candidate := range databases {
		if candidate.ID == databaseID &&
			candidate.Status == "ready" &&
			!candidate.Attached {

			return candidate, nil
		}
	}

	return miniBaseDatabase{}, ErrMiniBaseDatabaseUnavailable
}

func createExistingMiniBaseAttachment(
	ctx context.Context,
	app string,
	databaseID string,
) (DatabaseAttachmentRecord, error) {
	database, err := availableExistingMiniBaseDatabase(ctx, databaseID)
	if err != nil {
		return DatabaseAttachmentRecord{}, err
	}

	attachment, err := miniBaseClient.CreateAttachment(ctx, database.ID, app)
	if err != nil {
		return DatabaseAttachmentRecord{}, ErrMiniBaseOperation
	}

	return DatabaseAttachmentRecord{
		AttachmentID: attachment.ID,
		DatabaseID:   database.ID,
		DisplayName:  database.DisplayName,
		BindingName:  miniBaseBindingPrimary,
	}, nil
}

func createInitialMiniBaseAttachment(
	app string,
	databaseID string,
	plan deploymentBuildPlan,
) (DatabaseAttachmentRecord, error) {
	if databaseID == "" {
		return DatabaseAttachmentRecord{}, nil
	}
	if !deploymentPlanSupportsMiniBase(plan) {
		return DatabaseAttachmentRecord{}, ErrInitialDatabaseUnsupported
	}

	// deployRepository already holds deployMu. MiniBase atomically rejects a
	// competing attachment, so taking databaseAttachmentMu here would invert
	// the post-deploy attachment lock order.
	ctx, cancel := context.WithTimeout(context.Background(), 5*miniBaseRequestTimeout)
	defer cancel()
	return createExistingMiniBaseAttachment(ctx, app, databaseID)
}

func deleteInitialMiniBaseAttachment(attachment DatabaseAttachmentRecord) error {
	if attachment.AttachmentID == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), miniBaseRequestTimeout)
	defer cancel()
	if err := miniBaseClient.DeleteAttachment(ctx, attachment.AttachmentID); err != nil {
		return fmt.Errorf("automatic MiniBase attachment cleanup failed")
	}
	return nil
}

func validateManagedDatabaseEnvironment(record DeploymentRecord, environment map[string]string) error {
	_, attached, err := currentDatabaseAttachment(record)
	if err != nil {
		return err
	}
	if attached {
		if _, exists := environment["DATABASE_URL"]; exists {
			return fmt.Errorf("DATABASE_URL is managed by ReactorLab while a MiniBase database is attached")
		}
	}
	return nil
}

func resolveDatabaseRuntime(record DeploymentRecord, environment map[string]string) (databaseRuntime, error) {
	runtime := databaseRuntime{
		Environment: cloneRuntimeEnvironment(environment),
		Redaction:   cloneRuntimeEnvironment(environment),
	}
	attachment, attached, err := currentDatabaseAttachment(record)
	if err != nil || !attached {
		return runtime, err
	}
	if !deploymentSupportsMiniBase(record) {
		return databaseRuntime{}, fmt.Errorf("deployment strategy does not support MiniBase")
	}
	if err := validateManagedDatabaseEnvironment(record, environment); err != nil {
		return databaseRuntime{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), miniBaseRequestTimeout)
	defer cancel()
	binding, err := miniBaseClient.ResolveBinding(ctx, attachment.AttachmentID)
	if err != nil {
		return databaseRuntime{}, fmt.Errorf("resolve MiniBase binding: %w", err)
	}
	if binding.DatabaseID != attachment.DatabaseID {
		return databaseRuntime{}, fmt.Errorf("MiniBase binding does not match deployment metadata")
	}
	connection := (&url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(binding.Username, binding.Password),
		Host:     net.JoinHostPort(binding.Host, strconv.Itoa(binding.Port)),
		Path:     "/" + binding.Database,
		RawQuery: url.Values{"sslmode": []string{"disable"}}.Encode(),
	}).String()
	runtime.Environment["DATABASE_URL"] = connection
	runtime.Redaction["DATABASE_URL"] = connection
	runtime.Redaction["MINIBASE_DATABASE_PASSWORD"] = binding.Password
	runtime.DataNetwork = binding.DockerNetwork
	return runtime, nil
}

func validateReactorLabDataNetwork() error {
	output, err := runCommand("", "docker", "network", "inspect", "-f", "{{.Driver}} {{.Internal}}", reactorLabDataNetwork)
	if err != nil {
		return fmt.Errorf("required private data network is unavailable")
	}
	if strings.TrimSpace(output) != "bridge true" {
		return fmt.Errorf("required private data network has invalid configuration")
	}
	return nil
}

func runReactorLabMigration(app, image, packageManager string, declared bool, runtime databaseRuntime) error {
	if !declared {
		return nil
	}
	if runtime.DataNetwork == "" {
		return fmt.Errorf("reactorlab:migrate requires an attached MiniBase database")
	}
	if packageManager != packageManagerNPM {
		return fmt.Errorf("reactorlab:migrate package manager is unsupported")
	}
	if err := validateReactorLabDataNetwork(); err != nil {
		return err
	}
	envFile, cleanup, err := runtimeEnvironmentStore.TemporaryDockerEnvFile(app, runtime.Environment)
	if err != nil {
		return fmt.Errorf("prepare migration environment: %w", err)
	}
	defer cleanup()
	name := fmt.Sprintf("minideploy-%s-migration-%d", app, time.Now().UnixNano())
	args := []string{
		"run", "--rm", "--name", name,
		"--network", reactorLabDataNetwork,
		"--label", "com.minideploy.managed=true",
		"--label", "com.minideploy.app=" + app,
		"--label", "com.minideploy.service=migration",
	}
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	args = append(args, image, "npm", "run", "reactorlab:migrate")
	output, commandErr := runCommand("", "docker", args...)
	redacted := redactRuntimeEnvironmentValues(output, runtime.Redaction)
	if commandErr != nil {
		_, _ = runCommand("", "docker", "rm", "-f", name)
		deploymentEvent(app, "ERROR: reactorlab:migrate failed: %s", redacted)
		return fmt.Errorf("reactorlab:migrate failed")
	}
	if strings.TrimSpace(redacted) != "" {
		deploymentEvent(app, "reactorlab:migrate completed: %s", redacted)
	} else {
		deploymentEvent(app, "reactorlab:migrate completed successfully.")
	}
	return nil
}

func detachMiniBaseAttachment(record DeploymentRecord) error {
	attachment, attached, err := currentDatabaseAttachment(record)
	if err != nil || !attached {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), miniBaseRequestTimeout)
	defer cancel()
	if err := miniBaseClient.DeleteAttachment(ctx, attachment.AttachmentID); err != nil {
		log.Printf("MiniBase attachment detach failed for %s: %v", record.App, err)
		return fmt.Errorf("detach MiniBase database: %w", err)
	}
	return nil
}
