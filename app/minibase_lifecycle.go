package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrDatabaseLifecycleUnsupported = errors.New(
		"deployment does not support MiniBase lifecycle",
	)
	ErrDatabaseLifecycleNotAttached = errors.New(
		"deployment has no MiniBase database attached",
	)
	ErrDatabaseLifecycleAttachmentMismatch = errors.New(
		"database attachment does not match deployment",
	)
	ErrDatabaseLifecycleNotDetached = errors.New(
		"deployment is not waiting for database reconnection",
	)
	ErrDatabaseLifecycleDeploymentUnavailable = errors.New(
		"deployment is not available for database attachment",
	)

	miniBaseLifecycleTokenPath = defaultMiniBaseTokenPath

	databaseLifecycleSynchronizeProxy = syncProxyRoutes
	databaseLifecycleStopDeployment   = stopDeploymentForDatabaseDetach
	databaseLifecycleStartDeployment  = startDeploymentAfterDatabaseAttach
)

type miniBaseLifecycleDeploymentResponse struct {
	App              string `json:"app"`
	Supported        bool   `json:"supported"`
	Status           string `json:"status"`
	DatabaseAttached bool   `json:"databaseAttached"`
	DatabaseDetached bool   `json:"databaseDetached"`
	DatabaseID       string `json:"databaseId,omitempty"`
}

type miniBaseLifecycleDetachRequest struct {
	DatabaseID   string `json:"databaseId"`
	AttachmentID string `json:"attachmentId"`
}

type miniBaseLifecycleAttachRequest struct {
	DatabaseID string `json:"databaseId"`
}

func miniBaseLifecycleAuthorized(r *http.Request) bool {
	expected, err := loadMiniBaseToken(miniBaseLifecycleTokenPath)
	if err != nil {
		return false
	}

	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}

	provided := []byte(strings.TrimPrefix(header, prefix))
	if len(provided) != len(expected) {
		return false
	}

	return subtle.ConstantTimeCompare(provided, expected) == 1
}

func requireMiniBaseLifecycleAuthorization(
	w http.ResponseWriter,
	r *http.Request,
) bool {
	if miniBaseLifecycleAuthorized(r) {
		return true
	}

	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func decodeMiniBaseLifecycleRequest(
	w http.ResponseWriter,
	r *http.Request,
	output any,
) bool {
	decoder := json.NewDecoder(
		http.MaxBytesReader(w, r.Body, 4096),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(output); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(
			w,
			"request must contain one JSON value",
			http.StatusBadRequest,
		)
		return false
	}

	return true
}

func miniBaseLifecycleDeploymentsHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !requireMiniBaseLifecycleAuthorization(w, r) {
		return
	}

	records, err := store.List()
	if err != nil {
		http.Error(
			w,
			"failed to load deployments",
			http.StatusInternalServerError,
		)
		return
	}

	result := make(
		[]miniBaseLifecycleDeploymentResponse,
		0,
		len(records),
	)

	for _, record := range records {
		record = normalizeDeploymentRecord(record)

		attachment, attached, attachmentErr :=
			currentDatabaseAttachment(record)
		if attachmentErr != nil {
			http.Error(
				w,
				"invalid deployment database metadata",
				http.StatusInternalServerError,
			)
			return
		}

		item := miniBaseLifecycleDeploymentResponse{
			App:              record.App,
			Supported:        deploymentSupportsMiniBase(record),
			Status:           deploymentProjectStatus(record),
			DatabaseAttached: attached,
			DatabaseDetached: record.DatabaseDetached,
		}
		if attached {
			item.DatabaseID = attachment.DatabaseID
		}

		result = append(result, item)
	}

	writeJSON(w, http.StatusOK, result)
}

func miniBaseLifecycleDetachHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !requireMiniBaseLifecycleAuthorization(w, r) {
		return
	}

	var input miniBaseLifecycleDetachRequest
	if !decodeMiniBaseLifecycleRequest(w, r, &input) {
		return
	}

	if !miniBaseDatabaseIDPattern.MatchString(input.DatabaseID) ||
		!miniBaseAttachmentIDPattern.MatchString(input.AttachmentID) {

		http.Error(w, "invalid attachment", http.StatusBadRequest)
		return
	}

	err := detachDatabaseFromDeployment(
		r.PathValue("app"),
		input.DatabaseID,
		input.AttachmentID,
	)
	if err != nil {
		writeMiniBaseLifecycleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func miniBaseLifecycleAttachHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !requireMiniBaseLifecycleAuthorization(w, r) {
		return
	}

	var input miniBaseLifecycleAttachRequest
	if !decodeMiniBaseLifecycleRequest(w, r, &input) {
		return
	}

	if !miniBaseDatabaseIDPattern.MatchString(input.DatabaseID) {
		http.Error(w, "invalid database", http.StatusBadRequest)
		return
	}

	_, err := attachDatabaseToDeployment(
		r.Context(),
		r.PathValue("app"),
		input.DatabaseID,
	)
	if err != nil {
		writeMiniBaseLifecycleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeMiniBaseLifecycleError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, ErrDeploymentNotFound):
		http.Error(w, "deployment not found", http.StatusNotFound)

	case errors.Is(err, ErrDatabaseLifecycleUnsupported):
		http.Error(
			w,
			"deployment does not support MiniBase",
			http.StatusUnprocessableEntity,
		)

	case errors.Is(err, ErrDatabaseLifecycleNotAttached),
		errors.Is(err, ErrDatabaseLifecycleAttachmentMismatch),
		errors.Is(err, ErrDatabaseLifecycleNotDetached),
		errors.Is(err, ErrDatabaseLifecycleDeploymentUnavailable),
		errors.Is(err, ErrMiniBaseDatabaseUnavailable):

		http.Error(w, "database lifecycle conflict", http.StatusConflict)

	case errors.Is(err, ErrMiniBaseOperation):
		http.Error(
			w,
			"MiniBase operation failed",
			http.StatusBadGateway,
		)

	default:
		http.Error(
			w,
			"database lifecycle operation failed",
			http.StatusInternalServerError,
		)
	}
}

func detachDatabaseFromDeployment(
	app string,
	databaseID string,
	attachmentID string,
) error {
	databaseAttachmentMu.Lock()
	defer databaseAttachmentMu.Unlock()

	deployMu.Lock()
	defer deployMu.Unlock()

	latest, err := getDeployment(app)
	if err != nil {
		return err
	}
	if !deploymentSupportsMiniBase(latest) {
		return ErrDatabaseLifecycleUnsupported
	}

	attachment, attached, err :=
		currentDatabaseAttachment(latest)
	if err != nil {
		return err
	}

	if !attached {
		if latest.DatabaseDetached {
			return nil
		}
		return ErrDatabaseLifecycleNotAttached
	}

	if attachment.DatabaseID != databaseID ||
		attachment.AttachmentID != attachmentID {

		return ErrDatabaseLifecycleAttachmentMismatch
	}

	original := latest

	// First remove traffic from the application while the MiniBase
	// attachment still exists. This makes database deletion impossible
	// while any old process could still be using DATABASE_URL.
	if !latest.DatabaseDetached {
		latest.DatabaseDetached = true

		if err := store.Save(latest); err != nil {
			return fmt.Errorf(
				"save detached deployment state: %w",
				err,
			)
		}

		if err := databaseLifecycleSynchronizeProxy(); err != nil {
			_ = store.Save(original)
			_ = databaseLifecycleSynchronizeProxy()

			return fmt.Errorf(
				"switch deployment to detached route: %w",
				err,
			)
		}
	}

	// Now no new public traffic reaches the deployment. Remove all
	// running containers before releasing the database attachment.
	if err := databaseLifecycleStopDeployment(latest); err != nil {
		return fmt.Errorf(
			"stop database-dependent deployment: %w",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		miniBaseRequestTimeout,
	)
	defer cancel()

	if err := miniBaseClient.DeleteAttachment(
		ctx,
		attachment.AttachmentID,
	); err != nil {

		return fmt.Errorf(
			"%w: detach MiniBase attachment",
			ErrMiniBaseOperation,
		)
	}

	latest.DatabaseAttachments = nil
	latest.DatabaseDetached = true

	if err := store.Save(latest); err != nil {
		return fmt.Errorf(
			"save completed detached deployment state: %w",
			err,
		)
	}

	deploymentEvent(
		latest.App,
		"MiniBase database detached. Deployment is stopped until a database is reattached.",
	)

	return nil
}

func attachDatabaseToDeployment(
	requestContext context.Context,
	app string,
	databaseID string,
) (DeploymentRecord, error) {
	latest, err := getDeployment(app)
	if err != nil {
		return DeploymentRecord{}, err
	}

	if latest.DatabaseDetached {
		return attachDatabaseToDetachedDeployment(
			requestContext,
			app,
			databaseID,
		)
	}

	return attachDatabaseToRunningDeployment(
		requestContext,
		app,
		databaseID,
	)
}

func attachDatabaseToRunningDeployment(
	requestContext context.Context,
	app string,
	databaseID string,
) (DeploymentRecord, error) {
	databaseAttachmentMu.Lock()
	defer databaseAttachmentMu.Unlock()

	latest, err := getDeployment(app)
	if err != nil {
		return DeploymentRecord{}, err
	}
	if !deploymentSupportsMiniBase(latest) {
		return DeploymentRecord{},
			ErrDatabaseLifecycleUnsupported
	}
	if latest.DatabaseDetached {
		return DeploymentRecord{},
			ErrDatabaseLifecycleNotDetached
	}
	if deploymentProjectStatus(latest) != "running" {
		return DeploymentRecord{},
			ErrDatabaseLifecycleDeploymentUnavailable
	}

	if _, attached, err :=
		currentDatabaseAttachment(latest); err != nil {

		return DeploymentRecord{}, err
	} else if attached {
		return DeploymentRecord{},
			ErrDatabaseLifecycleAttachmentMismatch
	}

	environment, err :=
		runtimeEnvironmentStore.Load(latest.App)
	if err != nil {
		return DeploymentRecord{},
			fmt.Errorf(
				"load runtime environment: %w",
				err,
			)
	}
	if _, conflict := environment["DATABASE_URL"]; conflict {
		return DeploymentRecord{},
			fmt.Errorf(
				"managed DATABASE_URL conflicts with stored runtime environment",
			)
	}

	ctx, cancel := context.WithTimeout(
		requestContext,
		5*miniBaseRequestTimeout,
	)
	defer cancel()

	database, err :=
		availableExistingMiniBaseDatabase(ctx, databaseID)
	if err != nil {
		if errors.Is(
			err,
			ErrMiniBaseDatabaseUnavailable,
		) {
			return DeploymentRecord{},
				ErrMiniBaseDatabaseUnavailable
		}

		return DeploymentRecord{},
			ErrMiniBaseOperation
	}

	attachment, err :=
		miniBaseClient.CreateAttachment(
			ctx,
			database.ID,
			latest.App,
		)
	if err != nil {
		return DeploymentRecord{},
			ErrMiniBaseOperation
	}

	original := latest

	latest.DatabaseAttachments =
		[]DatabaseAttachmentRecord{{
			AttachmentID: attachment.ID,
			DatabaseID:   database.ID,
			DisplayName:  database.DisplayName,
			BindingName:  miniBaseBindingPrimary,
		}}

	if err := store.Save(latest); err != nil {
		if detachErr :=
			miniBaseClient.DeleteAttachment(
				ctx,
				attachment.ID,
			); detachErr != nil {

			// Fail closed: keep local attachment metadata
			// aligned with the MiniBase attachment that could
			// not be removed.
			_ = store.Save(latest)

			return DeploymentRecord{},
				fmt.Errorf(
					"%w: attachment compensation failed",
					ErrMiniBaseOperation,
				)
		}

		return DeploymentRecord{},
			fmt.Errorf(
				"save database attachment metadata: %w",
				err,
			)
	}

	updated, err :=
		databaseAttachmentRedeploy(latest, nil)
	if err == nil {
		deploymentEvent(
			latest.App,
			"MiniBase database attached. Safe redeploy completed successfully.",
		)

		return updated, nil
	}

	// The safe redeploy contract keeps the previous release live
	// when candidate deployment fails. Remove the new attachment
	// so MiniBase remains standalone and the user can retry.
	if detachErr :=
		miniBaseClient.DeleteAttachment(
			ctx,
			attachment.ID,
		); detachErr != nil {

		// MiniBase still considers the database attached, so retain
		// the same local metadata. This prevents deletion until the
		// relationship can be cleaned up safely.
		_ = store.Save(latest)

		return DeploymentRecord{},
			fmt.Errorf(
				"%w: deployment update and attachment compensation failed",
				ErrMiniBaseOperation,
			)
	}

	if restoreErr := store.Save(original); restoreErr != nil {
		return DeploymentRecord{},
			fmt.Errorf(
				"deployment update failed and local attachment metadata could not be restored: %w",
				restoreErr,
			)
	}

	return DeploymentRecord{},
		fmt.Errorf(
			"attach database redeploy failed: %w",
			err,
		)
}

func attachDatabaseToDetachedDeployment(
	requestContext context.Context,
	app string,
	databaseID string,
) (DeploymentRecord, error) {
	databaseAttachmentMu.Lock()
	defer databaseAttachmentMu.Unlock()

	deployMu.Lock()
	defer deployMu.Unlock()

	latest, err := getDeployment(app)
	if err != nil {
		return DeploymentRecord{}, err
	}
	if !deploymentSupportsMiniBase(latest) {
		return DeploymentRecord{},
			ErrDatabaseLifecycleUnsupported
	}

	if !latest.DatabaseDetached {
		return DeploymentRecord{},
			ErrDatabaseLifecycleNotDetached
	}

	if _, attached, err := currentDatabaseAttachment(latest); err != nil {
		return DeploymentRecord{}, err
	} else if attached {
		return DeploymentRecord{},
			ErrDatabaseLifecycleAttachmentMismatch
	}

	environment, err := runtimeEnvironmentStore.Load(latest.App)
	if err != nil {
		return DeploymentRecord{},
			fmt.Errorf("load runtime environment: %w", err)
	}
	if _, conflict := environment["DATABASE_URL"]; conflict {
		return DeploymentRecord{},
			fmt.Errorf(
				"managed DATABASE_URL conflicts with stored runtime environment",
			)
	}

	ctx, cancel := context.WithTimeout(
		requestContext,
		5*miniBaseRequestTimeout,
	)
	defer cancel()

	database, err :=
		availableExistingMiniBaseDatabase(ctx, databaseID)
	if err != nil {
		if errors.Is(err, ErrMiniBaseDatabaseUnavailable) {
			return DeploymentRecord{},
				ErrMiniBaseDatabaseUnavailable
		}
		return DeploymentRecord{}, ErrMiniBaseOperation
	}

	attachment, err := miniBaseClient.CreateAttachment(
		ctx,
		database.ID,
		latest.App,
	)
	if err != nil {
		return DeploymentRecord{}, ErrMiniBaseOperation
	}

	candidate := latest
	candidate.DatabaseDetached = false
	candidate.DatabaseAttachments = []DatabaseAttachmentRecord{{
		AttachmentID: attachment.ID,
		DatabaseID:   database.ID,
		DisplayName:  database.DisplayName,
		BindingName:  miniBaseBindingPrimary,
	}}

	started, err :=
		databaseLifecycleStartDeployment(candidate)
	if err != nil {
		_ = miniBaseClient.DeleteAttachment(
			ctx,
			attachment.ID,
		)

		return DeploymentRecord{},
			fmt.Errorf(
				"restart deployment with database: %w",
				err,
			)
	}

	if err := store.Save(started); err != nil {
		_ = databaseLifecycleStopDeployment(started)
		_ = miniBaseClient.DeleteAttachment(
			ctx,
			attachment.ID,
		)

		return DeploymentRecord{},
			fmt.Errorf(
				"save reattached deployment: %w",
				err,
			)
	}

	if err := databaseLifecycleSynchronizeProxy(); err != nil {
		_ = store.Save(latest)
		_ = databaseLifecycleSynchronizeProxy()
		_ = databaseLifecycleStopDeployment(started)
		_ = miniBaseClient.DeleteAttachment(
			ctx,
			attachment.ID,
		)

		return DeploymentRecord{},
			fmt.Errorf(
				"restore live route after database attach: %w",
				err,
			)
	}

	deploymentEvent(
		started.App,
		"MiniBase database reattached. Deployment restarted and health checks passed.",
	)

	return started, nil
}

func stopDeploymentForDatabaseDetach(
	record DeploymentRecord,
) error {
	record = normalizeDeploymentRecord(record)

	switch record.Strategy {
	case deploymentStrategyFullstackViteNode:
		return cleanupFullstackRelease(record, false)

	case deploymentStrategyNodeExpress:
		if record.Container == "" {
			return fmt.Errorf(
				"deployment container metadata is unavailable",
			)
		}

		if !containerExists(record.Container) {
			return nil
		}

		output, err := runCommand(
			"",
			"docker",
			"rm",
			"-f",
			record.Container,
		)
		if err != nil {
			return fmt.Errorf(
				"remove deployment container: %w: %s",
				err,
				output,
			)
		}

		return nil

	default:
		return ErrDatabaseLifecycleUnsupported
	}
}

func startDeploymentAfterDatabaseAttach(
	record DeploymentRecord,
) (DeploymentRecord, error) {
	record = normalizeDeploymentRecord(record)

	environment, err :=
		runtimeEnvironmentStore.Load(record.App)
	if err != nil {
		return DeploymentRecord{},
			fmt.Errorf(
				"load runtime environment: %w",
				err,
			)
	}

	if err := verifyRuntimeEnvironmentMetadata(
		record,
		environment,
	); err != nil {
		return DeploymentRecord{}, err
	}

	switch record.Strategy {
	case deploymentStrategyFullstackViteNode:
		started, err :=
			startAndVerifyFullstackRelease(
				record,
				environment,
			)
		if err != nil {
			_ = cleanupFullstackRelease(
				started,
				false,
			)
			return DeploymentRecord{}, err
		}

		return started, nil

	case deploymentStrategyNodeExpress:
		runtime, err :=
			resolveDatabaseRuntime(
				record,
				environment,
			)
		if err != nil {
			return DeploymentRecord{}, err
		}

		if err := runReactorLabMigration(
			record.App,
			record.Image,
			record.PackageManager,
			record.ReactorLabMigration,
			runtime,
		); err != nil {
			return DeploymentRecord{}, err
		}

		port, err := findAvailablePort(
			minDeployPort,
			maxDeployPort,
		)
		if err != nil {
			return DeploymentRecord{},
				fmt.Errorf(
					"allocate deployment port: %w",
					err,
				)
		}

		record.Port = port

		if err := startManagedDeploymentContainerWithOptions(
			record.App,
			record.Container,
			record.Image,
			record.Port,
			record.ContainerPort,
			record.Strategy,
			runtime.Environment,
			managedContainerOptions{
				DataNetwork: runtime.DataNetwork,
			},
		); err != nil {
			return DeploymentRecord{}, err
		}

		cleanup := func() {
			if containerExists(record.Container) {
				_, _ = runCommand(
					"",
					"docker",
					"rm",
					"-f",
					record.Container,
				)
			}
		}

		if err := verifyContainerStartup(
			record.Container,
		); err != nil {
			cleanup()
			return DeploymentRecord{},
				fmt.Errorf(
					"deployment failed startup verification: %w",
					err,
				)
		}

		if err := verifyHTTPHealthPath(
			record.Port,
			record.HealthPath,
		); err != nil {
			cleanup()
			return DeploymentRecord{},
				fmt.Errorf(
					"deployment failed HTTP health check: %w",
					err,
				)
		}

		return normalizeDeploymentRecord(record), nil

	default:
		return DeploymentRecord{},
			ErrDatabaseLifecycleUnsupported
	}
}
