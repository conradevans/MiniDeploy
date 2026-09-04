package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func lifecycleTestStore(
	t *testing.T,
	record DeploymentRecord,
) *JSONStore {
	t.Helper()

	previousStore := store
	testStore := NewJSONStore(
		filepath.Join(
			t.TempDir(),
			"deployments.json",
		),
	)
	store = testStore
	t.Cleanup(func() {
		store = previousStore
	})

	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	return testStore
}

func replaceLifecycleFunctionsForTest(
	t *testing.T,
	syncFn func() error,
	stopFn func(DeploymentRecord) error,
	startFn func(DeploymentRecord) (DeploymentRecord, error),
) {
	t.Helper()

	oldSync := databaseLifecycleSynchronizeProxy
	oldStop := databaseLifecycleStopDeployment
	oldStart := databaseLifecycleStartDeployment

	databaseLifecycleSynchronizeProxy = syncFn
	databaseLifecycleStopDeployment = stopFn
	databaseLifecycleStartDeployment = startFn

	t.Cleanup(func() {
		databaseLifecycleSynchronizeProxy = oldSync
		databaseLifecycleStopDeployment = oldStop
		databaseLifecycleStartDeployment = oldStart
	})
}

func TestMiniBaseLifecycleAuthorizationUsesSharedIntegrationToken(
	t *testing.T,
) {
	tokenPath := filepath.Join(
		t.TempDir(),
		"integration-token",
	)

	token := "0123456789abcdef0123456789abcdef01234567"
	if err := os.WriteFile(
		tokenPath,
		[]byte(token+"\n"),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	oldPath := miniBaseLifecycleTokenPath
	miniBaseLifecycleTokenPath = tokenPath
	t.Cleanup(func() {
		miniBaseLifecycleTokenPath = oldPath
	})

	request := httptest.NewRequest(
		http.MethodGet,
		"/internal/minibase/deployments",
		nil,
	)

	if miniBaseLifecycleAuthorized(request) {
		t.Fatal("request without token was authorized")
	}

	request.Header.Set(
		"Authorization",
		"Bearer wrong-token",
	)
	if miniBaseLifecycleAuthorized(request) {
		t.Fatal("request with wrong token was authorized")
	}

	request.Header.Set(
		"Authorization",
		"Bearer "+token,
	)
	if !miniBaseLifecycleAuthorized(request) {
		t.Fatal("request with integration token was rejected")
	}
}

func TestDetachStopsDeploymentBeforeReleasingMiniBaseAttachment(
	t *testing.T,
) {
	record := attachedNodeRecord()
	testStore := lifecycleTestStore(t, record)

	client := &fakeMiniBaseClient{}
	replaceMiniBaseClientForTest(t, client)

	syncCalls := 0
	stopCalls := 0

	replaceLifecycleFunctionsForTest(
		t,
		func() error {
			syncCalls++
			return nil
		},
		func(candidate DeploymentRecord) error {
			stopCalls++

			if !candidate.DatabaseDetached {
				t.Fatal(
					"deployment was stopped before detached route state was saved",
				)
			}
			if len(candidate.DatabaseAttachments) != 1 {
				t.Fatal(
					"MiniBase attachment disappeared before deployment stopped",
				)
			}

			return nil
		},
		func(record DeploymentRecord) (DeploymentRecord, error) {
			return record, nil
		},
	)

	attachment := record.DatabaseAttachments[0]

	if err := detachDatabaseFromDeployment(
		record.App,
		attachment.DatabaseID,
		attachment.AttachmentID,
	); err != nil {
		t.Fatalf(
			"detachDatabaseFromDeployment() error: %v",
			err,
		)
	}

	if syncCalls != 1 {
		t.Fatalf(
			"proxy sync calls = %d; want 1",
			syncCalls,
		)
	}
	if stopCalls != 1 {
		t.Fatalf(
			"stop calls = %d; want 1",
			stopCalls,
		)
	}

	if len(client.deleted) != 1 ||
		client.deleted[0] != attachment.AttachmentID {

		t.Fatalf(
			"deleted attachments = %#v",
			client.deleted,
		)
	}

	persisted, err := testStore.Get(record.App)
	if err != nil {
		t.Fatal(err)
	}

	if !persisted.DatabaseDetached {
		t.Fatal("deployment was not persisted as detached")
	}
	if len(persisted.DatabaseAttachments) != 0 {
		t.Fatal("completed detach retained attachment metadata")
	}
}

func TestReattachRestartsAndHealthChecksBeforeRestoringRoute(
	t *testing.T,
) {
	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	record.DatabaseDetached = true

	testStore := lifecycleTestStore(t, record)

	databaseID :=
		"database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	attachmentID :=
		"attachment_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          databaseID,
			DisplayName: "Scheduler Production",
			Status:      "ready",
			Attached:    false,
		}},
		attachment: miniBaseAttachment{
			ID:           attachmentID,
			DatabaseID:   databaseID,
			ConsumerType: "minideploy",
			ConsumerRef:  record.App,
			BindingName:  miniBaseBindingPrimary,
		},
	}
	replaceMiniBaseClientForTest(t, client)

	startCalls := 0
	syncCalls := 0

	replaceLifecycleFunctionsForTest(
		t,
		func() error {
			syncCalls++
			return nil
		},
		func(record DeploymentRecord) error {
			return nil
		},
		func(candidate DeploymentRecord) (
			DeploymentRecord,
			error,
		) {
			startCalls++

			if candidate.DatabaseDetached {
				t.Fatal(
					"candidate remained detached while restarting",
				)
			}
			if len(candidate.DatabaseAttachments) != 1 {
				t.Fatal(
					"candidate did not receive MiniBase attachment",
				)
			}
			if candidate.DatabaseAttachments[0].DatabaseID !=
				databaseID {

				t.Fatal(
					"candidate received wrong database",
				)
			}

			candidate.Port = 8500
			return candidate, nil
		},
	)

	updated, err := attachDatabaseToDetachedDeployment(
		context.Background(),
		record.App,
		databaseID,
	)
	if err != nil {
		t.Fatalf(
			"attachDatabaseToDetachedDeployment() error: %v",
			err,
		)
	}

	if startCalls != 1 {
		t.Fatalf(
			"start calls = %d; want 1",
			startCalls,
		)
	}
	if syncCalls != 1 {
		t.Fatalf(
			"proxy sync calls = %d; want 1",
			syncCalls,
		)
	}

	if client.attachmentCalls != 1 ||
		client.attachedDB != databaseID ||
		client.consumerRef != record.App {

		t.Fatalf(
			"MiniBase attachment request = calls:%d db:%q app:%q",
			client.attachmentCalls,
			client.attachedDB,
			client.consumerRef,
		)
	}

	if updated.DatabaseDetached {
		t.Fatal("reattached deployment remained detached")
	}
	if len(updated.DatabaseAttachments) != 1 {
		t.Fatal("reattached deployment has no attachment")
	}

	persisted, err := testStore.Get(record.App)
	if err != nil {
		t.Fatal(err)
	}

	if persisted.DatabaseDetached {
		t.Fatal("persisted deployment remained detached")
	}
	if len(persisted.DatabaseAttachments) != 1 {
		t.Fatal("persisted deployment has no attachment")
	}
}

func TestReattachRefusesRunningDeployment(
	t *testing.T,
) {
	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	record.DatabaseDetached = false

	lifecycleTestStore(t, record)

	_, err := attachDatabaseToDetachedDeployment(
		context.Background(),
		record.App,
		"database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)

	if err != ErrDatabaseLifecycleNotDetached {
		t.Fatalf(
			"error = %v; want ErrDatabaseLifecycleNotDetached",
			err,
		)
	}
}

func TestFirstAttachToRunningDeploymentUsesSafeRedeploy(
	t *testing.T,
) {
	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	record.DatabaseDetached = false

	testStore := lifecycleTestStore(t, record)

	previousRuntimeStore := runtimeEnvironmentStore
	runtimeEnvironmentStore = newRuntimeEnvironmentFileStore(
		filepath.Join(t.TempDir(), "runtime-secrets"),
	)
	t.Cleanup(func() {
		runtimeEnvironmentStore = previousRuntimeStore
	})

	if err := runtimeEnvironmentStore.Replace(
		record.App,
		map[string]string{
			"APP_MODE": "test",
		},
	); err != nil {
		t.Fatal(err)
	}

	databaseID :=
		"database_cccccccccccccccccccccccccccccccc"
	attachmentID :=
		"attachment_dddddddddddddddddddddddddddddddd"

	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          databaseID,
			DisplayName: "Fresh Database",
			Status:      "ready",
			Attached:    false,
		}},
		attachment: miniBaseAttachment{
			ID:           attachmentID,
			DatabaseID:   databaseID,
			ConsumerType: "minideploy",
			ConsumerRef:  record.App,
			BindingName:  miniBaseBindingPrimary,
		},
	}
	replaceMiniBaseClientForTest(t, client)

	previousRedeploy := databaseAttachmentRedeploy
	redeployCalls := 0
	databaseAttachmentRedeploy = func(
		candidate DeploymentRecord,
		_ map[string]string,
	) (DeploymentRecord, error) {
		redeployCalls++

		if len(candidate.DatabaseAttachments) != 1 {
			t.Fatal(
				"safe redeploy did not receive database attachment",
			)
		}
		if candidate.DatabaseAttachments[0].DatabaseID != databaseID {
			t.Fatal(
				"safe redeploy received wrong database",
			)
		}

		candidate.Port = 8501
		return candidate, nil
	}
	t.Cleanup(func() {
		databaseAttachmentRedeploy = previousRedeploy
	})

	previousStatusRunner := commandRunner
	commandRunner = func(
		dir string,
		name string,
		args ...string,
	) (string, error) {
		if name == "docker" &&
			len(args) >= 4 &&
			args[0] == "inspect" &&
			args[1] == "-f" {

			return "running\n", nil
		}

		return previousStatusRunner(
			dir,
			name,
			args...,
		)
	}
	t.Cleanup(func() {
		commandRunner = previousStatusRunner
	})

	updated, err := attachDatabaseToDeployment(
		context.Background(),
		record.App,
		databaseID,
	)
	if err != nil {
		t.Fatalf(
			"attachDatabaseToDeployment() error: %v",
			err,
		)
	}

	if redeployCalls != 1 {
		t.Fatalf(
			"redeploy calls = %d; want 1",
			redeployCalls,
		)
	}

	if client.attachmentCalls != 1 {
		t.Fatalf(
			"attachment calls = %d; want 1",
			client.attachmentCalls,
		)
	}

	if updated.DatabaseDetached {
		t.Fatal(
			"running deployment became detached",
		)
	}

	if len(updated.DatabaseAttachments) != 1 {
		t.Fatal(
			"running deployment did not retain attachment",
		)
	}

	persisted, err :=
		testStore.Get(record.App)
	if err != nil {
		t.Fatal(err)
	}

	if len(persisted.DatabaseAttachments) != 1 {
		t.Fatal(
			"database attachment was not persisted",
		)
	}
}
