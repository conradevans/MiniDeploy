package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func replaceDatabaseAttachmentRedeployForTest(t *testing.T, replacement func(DeploymentRecord, map[string]string) (DeploymentRecord, error)) {
	t.Helper()
	previous := databaseAttachmentRedeploy
	databaseAttachmentRedeploy = replacement
	t.Cleanup(func() { databaseAttachmentRedeploy = previous })
}

func databaseHandlerRequest(method, body string) *http.Request {
	request := httptest.NewRequest(method, "/deployments/scheduler/database", strings.NewReader(body))
	request.SetPathValue("app", "scheduler")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func setupDatabaseHandlerTest(t *testing.T, record DeploymentRecord, client miniBaseAPI) *JSONStore {
	t.Helper()
	metadata := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := metadata.Save(record); err != nil {
		t.Fatal(err)
	}
	restoreStore := replaceStoreForTest(t, metadata)
	t.Cleanup(restoreStore)
	replaceRuntimeEnvironmentStoreForTest(t)
	replaceMiniBaseClientForTest(t, client)
	replaceCommandRunnerForTest(t, func(_ string, _ string, _ ...string) (string, error) { return "running", nil })
	return metadata
}

func TestDatabaseAttachmentAdminCreateAndExistingWorkflows(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		client func() *fakeMiniBaseClient
	}{
		{
			name: "create new",
			body: `{"mode":"create","displayName":" Scheduler Production "}`,
			client: func() *fakeMiniBaseClient {
				return &fakeMiniBaseClient{
					createdDatabase: miniBaseDatabase{ID: "database_0123456789abcdef0123456789abcdef", DisplayName: "Scheduler Production", Status: "ready"},
					attachment:      miniBaseAttachment{ID: "attachment_0123456789abcdef0123456789abcdef", DatabaseID: "database_0123456789abcdef0123456789abcdef", ConsumerType: "minideploy", ConsumerRef: "scheduler", BindingName: "primary"},
				}
			},
		},
		{
			name: "attach existing",
			body: `{"mode":"attach","databaseId":"database_0123456789abcdef0123456789abcdef"}`,
			client: func() *fakeMiniBaseClient {
				return &fakeMiniBaseClient{
					databases:  []miniBaseDatabase{{ID: "database_0123456789abcdef0123456789abcdef", DisplayName: "Scheduler Production", Status: "ready"}},
					attachment: miniBaseAttachment{ID: "attachment_0123456789abcdef0123456789abcdef", DatabaseID: "database_0123456789abcdef0123456789abcdef", ConsumerType: "minideploy", ConsumerRef: "scheduler", BindingName: "primary"},
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := test.client()
			record := attachedNodeRecord()
			record.DatabaseAttachments = nil
			metadata := setupDatabaseHandlerTest(t, record, client)
			replaceDatabaseAttachmentRedeployForTest(t, func(updated DeploymentRecord, replacement map[string]string) (DeploymentRecord, error) {
				if replacement != nil || len(updated.DatabaseAttachments) != 1 {
					t.Fatal("attachment redeploy did not preserve the existing environment or safe attachment")
				}
				return updated, nil
			})

			response := httptest.NewRecorder()
			deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, test.body))
			if response.Code != http.StatusOK {
				t.Fatalf("attachment status = %d", response.Code)
			}
			persisted, err := metadata.Get("scheduler")
			if err != nil || len(persisted.DatabaseAttachments) != 1 || persisted.DatabaseAttachments[0].AttachmentID != client.attachment.ID {
				t.Fatalf("safe attachment metadata was not persisted: err=%v", err)
			}
			if client.attachedDB != client.attachment.DatabaseID || client.consumerRef != "scheduler" {
				t.Fatal("MiniBase attachment request used the wrong stable application/database identity")
			}
			if test.name == "create new" && client.createdName != "Scheduler Production" {
				t.Fatalf("create display name = %q", client.createdName)
			}
			for _, forbidden := range []string{"password", "postgresql://", "mb_role_", "credentialPath"} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("Admin response contains forbidden binding material marker %q", forbidden)
				}
			}
		})
	}
}

func TestDatabaseAttachmentRedeployFailurePreservesAttachmentForRetry(t *testing.T) {
	client := &fakeMiniBaseClient{
		databases:  []miniBaseDatabase{{ID: "database_0123456789abcdef0123456789abcdef", DisplayName: "Scheduler Production", Status: "ready"}},
		attachment: miniBaseAttachment{ID: "attachment_0123456789abcdef0123456789abcdef", DatabaseID: "database_0123456789abcdef0123456789abcdef", ConsumerType: "minideploy", ConsumerRef: "scheduler", BindingName: "primary"},
	}
	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	metadata := setupDatabaseHandlerTest(t, record, client)
	previousLogs := deploymentLogsDir
	deploymentLogsDir = filepath.Join(t.TempDir(), "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousLogs })
	replaceDatabaseAttachmentRedeployForTest(t, func(DeploymentRecord, map[string]string) (DeploymentRecord, error) {
		return DeploymentRecord{}, errors.New("candidate failed")
	})

	response := httptest.NewRecorder()
	deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, `{"mode":"attach","databaseId":"database_0123456789abcdef0123456789abcdef"}`))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("redeploy failure status = %d", response.Code)
	}
	persisted, err := metadata.Get("scheduler")
	if err != nil || len(persisted.DatabaseAttachments) != 1 {
		t.Fatal("redeploy failure discarded the recoverable attachment metadata")
	}
	if len(client.deleted) != 0 {
		t.Fatal("redeploy failure detached or deleted the independently managed database")
	}
}

func TestDatabaseAttachmentMetadataAndDetachFailureIsReportedSafely(t *testing.T) {
	client := &fakeMiniBaseClient{
		createdDatabase: miniBaseDatabase{ID: "database_0123456789abcdef0123456789abcdef", DisplayName: "Scheduler Production", Status: "ready"},
		attachment:      miniBaseAttachment{ID: "attachment_0123456789abcdef0123456789abcdef", DatabaseID: "database_0123456789abcdef0123456789abcdef", ConsumerType: "minideploy", ConsumerRef: "scheduler", BindingName: "primary"},
		deleteErr:       errors.New("MiniBase unavailable with secret-that-must-not-leak"),
	}
	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	metadata := setupDatabaseHandlerTest(t, record, client)
	badPath := filepath.Join(t.TempDir(), "metadata-target-directory")
	if err := os.Mkdir(badPath, 0o700); err != nil {
		t.Fatal(err)
	}
	client.onCreateAttach = func() { metadata.path = badPath }

	response := httptest.NewRecorder()
	deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, `{"mode":"create","displayName":"Scheduler Production"}`))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("metadata/detach failure status = %d", response.Code)
	}
	if len(client.deleted) != 1 || !strings.Contains(response.Body.String(), "automatic detachment failed") || !strings.Contains(response.Body.String(), "database was preserved") {
		t.Fatal("metadata/detach failure did not clearly report the recoverable preserved-database state")
	}
	if strings.Contains(response.Body.String(), "secret-that-must-not-leak") {
		t.Fatal("metadata/detach failure exposed an internal error")
	}
}

func TestDatabaseAttachmentHandlerRejectsUnsupportedConflictAndInvalidBodies(t *testing.T) {
	client := &fakeMiniBaseClient{}
	static := attachedNodeRecord()
	static.Strategy = deploymentStrategyViteStatic
	static.DatabaseAttachments = nil
	setupDatabaseHandlerTest(t, static, client)
	response := httptest.NewRecorder()
	deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, `{"mode":"create","displayName":"Static"}`))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("static attachment status = %d", response.Code)
	}

	record := attachedNodeRecord()
	record.DatabaseAttachments = nil
	setupDatabaseHandlerTest(t, record, client)
	if err := runtimeEnvironmentStore.Replace("scheduler", map[string]string{"DATABASE_URL": "external"}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, `{"mode":"create","displayName":"Conflict"}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("DATABASE_URL conflict status = %d", response.Code)
	}

	if err := runtimeEnvironmentStore.Replace("scheduler", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	deploymentDatabaseHandler(response, databaseHandlerRequest(http.MethodPost, `{"mode":"create","displayName":"One"} {}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("multiple JSON values status = %d", response.Code)
	}
}

func TestDatabaseAttachmentStatusAndMiniBaseUnavailableAreSafe(t *testing.T) {
	record := attachedNodeRecord()
	client := &fakeMiniBaseClient{err: errors.New("response included password=must-not-leak")}
	setupDatabaseHandlerTest(t, record, client)

	statusResponse := httptest.NewRecorder()
	deploymentDatabaseHandler(statusResponse, databaseHandlerRequest(http.MethodGet, ""))
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("database status code = %d", statusResponse.Code)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["supported"] == nil || fields["attachment"] == nil {
		t.Fatalf("database status response fields = %v", fields)
	}

	availableResponse := httptest.NewRecorder()
	availableMiniBaseDatabasesHandler(availableResponse, httptest.NewRequest(http.MethodGet, "/minibase/databases", nil))
	if availableResponse.Code != http.StatusBadGateway || strings.Contains(availableResponse.Body.String(), "must-not-leak") {
		t.Fatal("MiniBase-unavailable Admin response was not safe")
	}
}

func TestAttachedDeploymentLogsFailClosedWhenBindingIsUnavailable(t *testing.T) {
	record := attachedNodeRecord()
	client := &fakeMiniBaseClient{err: errors.New("binding password must-not-leak")}
	setupDatabaseHandlerTest(t, record, client)
	previousLogs := deploymentLogsDir
	deploymentLogsDir = filepath.Join(t.TempDir(), "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousLogs })
	resetDeploymentLog(record.App, "test")
	deploymentEvent(record.App, "raw-content-that-must-not-be-returned")

	for name, handler := range map[string]http.HandlerFunc{
		"runtime":    logsHandler,
		"deployment": deploymentLogsHandler,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/deployments/scheduler/logs", nil)
			request.SetPathValue("app", "scheduler")
			response := httptest.NewRecorder()
			handler(response, request)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d", response.Code)
			}
			for _, forbidden := range []string{"must-not-leak", "raw-content-that-must-not-be-returned"} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("fail-closed log response exposed %q", forbidden)
				}
			}
		})
	}
}
