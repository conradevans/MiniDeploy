package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type staticDeploymentStore struct {
	records []DeploymentRecord
	err     error
}

func (s staticDeploymentStore) Save(
	DeploymentRecord,
) error {
	return s.err
}

func (s staticDeploymentStore) Get(
	app string,
) (DeploymentRecord, error) {
	if s.err != nil {
		return DeploymentRecord{}, s.err
	}

	for _, record := range s.records {
		if record.App == app {
			return record, nil
		}
	}

	return DeploymentRecord{}, ErrDeploymentNotFound
}

func (s staticDeploymentStore) List() (
	[]DeploymentRecord,
	error,
) {
	return s.records, s.err
}

func (s staticDeploymentStore) Delete(string) error {
	return s.err
}

func TestGuestDeploymentsWorkWithoutAuthentication(
	t *testing.T,
) {
	restoreStore := replaceStoreForTest(
		t,
		staticDeploymentStore{},
	)
	defer restoreStore()

	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/guest/deployments",
		nil,
	)
	recorder := httptest.NewRecorder()

	publicRoutes(rejectingAccessValidator{}).
		ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d; want %d; body=%s",
			recorder.Code,
			http.StatusOK,
			recorder.Body.String(),
		)
	}

	if recorder.Body.String() != "[]\n" {
		t.Fatalf(
			"unexpected guest response: %s",
			recorder.Body.String(),
		)
	}
}

func TestGuestDeploymentSerializationUsesAllowlist(
	t *testing.T,
) {
	response, err := guestDeploymentResponse(
		DeploymentRecord{
			App:           "portfolio-app",
			RepoURL:       "https://github.com/private/repository.git",
			Container:     "internal-container",
			Image:         "private-image:secret",
			Port:          8123,
			ContainerPort: 3000,
			HealthPath:    "/internal-health",
		},
		"running",
	)
	if err != nil {
		t.Fatalf("guestDeploymentResponse() error: %v", err)
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal guest response: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal guest response: %v", err)
	}

	wantFields := map[string]string{
		"app":    "portfolio-app",
		"url":    "https://portfolio-app.reactorlab.dev",
		"status": "running",
	}

	if len(fields) != len(wantFields) {
		t.Fatalf(
			"guest fields = %v; want only %v",
			fields,
			wantFields,
		)
	}

	for key, want := range wantFields {
		if fields[key] != want {
			t.Fatalf(
				"guest field %q = %#v; want %q",
				key,
				fields[key],
				want,
			)
		}
	}

	forbiddenValues := []string{
		"private/repository",
		"internal-container",
		"private-image",
		"8123",
		"3000",
		"internal-health",
	}

	serialized := string(data)
	for _, forbidden := range forbiddenValues {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf(
				"guest JSON contains sensitive value %q: %s",
				forbidden,
				serialized,
			)
		}
	}
}

func TestPublicRoutesDoNotExposeLegacyManagement(
	t *testing.T,
) {
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodPost, "/deploy"},
		{http.MethodGet, "/deployments"},
		{http.MethodGet, "/deployments/example/logs"},
		{http.MethodGet, "/deployments/example/deploy-logs"},
		{http.MethodGet, "/deployments/example/history"},
		{http.MethodPost, "/deployments/example/restart"},
		{http.MethodPost, "/deployments/example/redeploy"},
		{http.MethodPost, "/deployments/example/rollback"},
		{http.MethodDelete, "/deployments/example"},
		{http.MethodPost, "/webhooks/github"},
	}

	handler := publicRoutes(stubAccessValidator{})

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(
				tt.method,
				"https://minideploy.reactorlab.dev"+tt.path,
				nil,
			)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusNotFound &&
				recorder.Code != http.StatusMethodNotAllowed {

				t.Fatalf(
					"legacy route returned %d; want 404 or 405",
					recorder.Code,
				)
			}
		})
	}
}

func TestExistingManagementRoutesRemainAvailable(
	t *testing.T,
) {
	restoreStore := replaceStoreForTest(
		t,
		staticDeploymentStore{},
	)
	defer restoreStore()

	t.Setenv("MINIDEPLOY_GITHUB_WEBHOOK_SECRET", "")

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "health",
			method:     http.MethodGet,
			path:       "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "deployments",
			method:     http.MethodGet,
			path:       "/deployments",
			wantStatus: http.StatusOK,
		},
		{
			name:       "deploy input validation",
			method:     http.MethodPost,
			path:       "/deploy",
			body:       "not-json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "runtime logs",
			method:     http.MethodGet,
			path:       "/deployments/missing/logs",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "deploy logs",
			method:     http.MethodGet,
			path:       "/deployments/missing/deploy-logs",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "history",
			method:     http.MethodGet,
			path:       "/deployments/missing/history",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "restart",
			method:     http.MethodPost,
			path:       "/deployments/missing/restart",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "redeploy",
			method:     http.MethodPost,
			path:       "/deployments/missing/redeploy",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "rollback",
			method:     http.MethodPost,
			path:       "/deployments/missing/rollback",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete",
			method:     http.MethodDelete,
			path:       "/deployments/missing",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "webhook configuration",
			method:     http.MethodPost,
			path:       "/webhooks/github",
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	handler := securityMiddleware(routes())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				tt.method,
				"http://localhost:9000"+tt.path,
				strings.NewReader(tt.body),
			)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d; want %d; body=%s",
					recorder.Code,
					tt.wantStatus,
					recorder.Body.String(),
				)
			}
		})
	}
}

func TestManagementAndPublicListenerAddresses(
	t *testing.T,
) {
	if managementAddress != "127.0.0.1:9000" {
		t.Fatalf(
			"management address = %q; want 127.0.0.1:9000",
			managementAddress,
		)
	}

	if publicAddress != "127.0.0.1:9003" {
		t.Fatalf(
			"public address = %q; want 127.0.0.1:9003",
			publicAddress,
		)
	}
}

func TestMiniDeployPublicHostnameIsReserved(
	t *testing.T,
) {
	for _, app := range []string{
		"minideploy",
		"MiniDeploy",
		" MINIDEPLOY ",
		"minideploy.",
	} {
		if !isReservedPublicApp(app) {
			t.Fatalf("expected %q to be reserved", app)
		}

		if hostname := publicHostnameForApp(app); hostname != "" {
			t.Fatalf(
				"publicHostnameForApp(%q) = %q; want empty",
				app,
				hostname,
			)
		}
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"http://localhost:9000/deploy",
		strings.NewReader(
			`{"repoUrl":"https://github.com/example/minideploy.git"}`,
		),
	)
	recorder := httptest.NewRecorder()

	deployHandler(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"reserved deployment status = %d; want %d",
			recorder.Code,
			http.StatusBadRequest,
		)
	}
}

func TestGuestStoreFailureReturnsGenericError(
	t *testing.T,
) {
	restoreStore := replaceStoreForTest(
		t,
		staticDeploymentStore{
			err: errors.New("/secret/path: internal failure"),
		},
	)
	defer restoreStore()

	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/guest/deployments",
		nil,
	)
	recorder := httptest.NewRecorder()

	publicRoutes(nil).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"status = %d; want %d",
			recorder.Code,
			http.StatusInternalServerError,
		)
	}

	if strings.Contains(
		recorder.Body.String(),
		"secret/path",
	) {
		t.Fatalf(
			"guest error leaked internal details: %s",
			recorder.Body.String(),
		)
	}
}

func replaceStoreForTest(
	t *testing.T,
	replacement DeploymentStore,
) func() {
	t.Helper()

	previous := store
	store = replacement

	return func() {
		store = previous
	}
}
