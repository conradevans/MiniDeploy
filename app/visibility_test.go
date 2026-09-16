package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJSONStoreMigratesExistingDeploymentsToGuestVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployments.json")
	legacy := `[{
  "app": "legacy-app",
  "repoUrl": "https://github.com/example/legacy-app.git",
  "container": "minideploy-legacy-app",
  "image": "minideploy-legacy-app:v1",
  "port": 8081,
  "containerPort": 3000,
  "healthPath": "/health"
}]`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy deployment metadata: %v", err)
	}

	records, err := NewJSONStore(path).List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(records) != 1 || !records[0].GuestVisible {
		t.Fatalf("legacy visibility = %#v; want visible", records)
	}

	var persisted []map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated metadata: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode migrated metadata: %v", err)
	}
	if visible, ok := persisted[0]["guestVisible"].(bool); !ok || !visible {
		t.Fatalf("persisted guestVisible = %#v; want true", persisted[0]["guestVisible"])
	}
}

func TestNewDeploymentMetadataDefaultsHidden(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployments.json")
	metadata := NewJSONStore(path)
	record := DeploymentRecord{
		App:           "new-app",
		ContainerPort: 3000,
		HealthPath:    "/health",
		GuestVisible:  false,
	}

	if err := metadata.Save(record); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	stored, err := metadata.Get(record.App)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if stored.GuestVisible {
		t.Fatal("new deployment metadata unexpectedly defaults visible")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read new metadata: %v", err)
	}
	if !strings.Contains(string(data), `"guestVisible": false`) {
		t.Fatalf("new visibility was not persisted explicitly: %s", data)
	}
}

func TestDeploymentVisibilityHandlerPersistsOnlyMetadata(t *testing.T) {
	metadata := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	record := DeploymentRecord{
		App:           "visibility-app",
		RepoURL:       "https://github.com/example/visibility-app.git",
		Container:     "minideploy-visibility-app",
		Image:         "minideploy-visibility-app:v1",
		Port:          8081,
		ContainerPort: 3000,
		HealthPath:    "/health",
		Strategy:      deploymentStrategyNodeExpress,
		GuestVisible:  false,
	}
	if err := metadata.Save(record); err != nil {
		t.Fatal(err)
	}
	restoreStore := replaceStoreForTest(t, metadata)
	defer restoreStore()

	for _, visible := range []bool{true, false} {
		body := `{"guestVisible":false}`
		if visible {
			body = `{"guestVisible":true}`
		}
		req := httptest.NewRequest(
			http.MethodPatch,
			"http://localhost:9000/deployments/visibility-app/visibility",
			strings.NewReader(body),
		)
		recorder := httptest.NewRecorder()
		routes().ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("visibility %t status = %d; body=%s", visible, recorder.Code, recorder.Body.String())
		}
		stored, err := metadata.Get(record.App)
		if err != nil {
			t.Fatal(err)
		}
		expected := record
		expected.GuestVisible = visible
		if !reflect.DeepEqual(stored, expected) {
			t.Fatalf("stored metadata = %#v; want %#v", stored, expected)
		}
		record = stored
	}
}

func TestDeploymentVisibilityHandlerRejectsInvalidRequests(t *testing.T) {
	metadata := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := metadata.Save(DeploymentRecord{
		App:           "known-app",
		ContainerPort: 80,
		HealthPath:    "/",
	}); err != nil {
		t.Fatal(err)
	}
	restoreStore := replaceStoreForTest(t, metadata)
	defer restoreStore()

	for _, body := range []string{
		``,
		`{}`,
		`{"guestVisible":"yes"}`,
		`{"guestVisible":true,"unexpected":true}`,
		`{"guestVisible":true}{"guestVisible":false}`,
	} {
		req := httptest.NewRequest(
			http.MethodPatch,
			"http://localhost:9000/deployments/known-app/visibility",
			strings.NewReader(body),
		)
		recorder := httptest.NewRecorder()
		routes().ServeHTTP(recorder, req)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d; want 400", body, recorder.Code)
		}
	}

	req := httptest.NewRequest(
		http.MethodPatch,
		"http://localhost:9000/deployments/missing/visibility",
		strings.NewReader(`{"guestVisible":true}`),
	)
	recorder := httptest.NewRecorder()
	routes().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown deployment status = %d; want 404", recorder.Code)
	}
}

func TestGuestDeploymentsFilterSummarizeAndDoNotLeakHiddenDetails(t *testing.T) {
	records := []DeploymentRecord{
		{
			App:           "shared-app",
			Container:     "shared-container",
			ContainerPort: 3000,
			HealthPath:    "/health",
			GuestVisible:  true,
		},
		{
			App:           "hidden-secret-app",
			RepoURL:       "https://github.com/private/hidden-secret-repository.git",
			Container:     "hidden-secret-container",
			Image:         "hidden-secret-image:v1",
			Port:          8999,
			ContainerPort: 4321,
			HealthPath:    "/hidden-secret-health",
			GuestVisible:  false,
		},
	}
	restoreStore := replaceStoreForTest(t, staticDeploymentStore{records: records})
	defer restoreStore()

	statusCalls := 0
	replaceCommandRunnerForTest(
		t,
		func(_ string, _ string, _ ...string) (string, error) {
			statusCalls++
			return "running\n", nil
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"https://minideploy.reactorlab.dev/api/guest/deployments",
		nil,
	)
	recorder := httptest.NewRecorder()
	publicRoutes(nil).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var response GuestDeploymentsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode guest response: %v", err)
	}
	if response.Summary != (GuestDeploymentSummary{Total: 2, Showing: 1, Hidden: 1}) {
		t.Fatalf("summary = %#v", response.Summary)
	}
	if len(response.Deployments) != 1 || response.Deployments[0].App != "shared-app" {
		t.Fatalf("guest deployments = %#v", response.Deployments)
	}
	if statusCalls != 1 {
		t.Fatalf("status calls = %d; hidden deployment status was queried", statusCalls)
	}

	serialized := recorder.Body.String()
	for _, forbidden := range []string{
		"hidden-secret-app",
		"hidden-secret-repository",
		"hidden-secret-container",
		"hidden-secret-image",
		"8999",
		"4321",
		"hidden-secret-health",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("guest response leaked %q: %s", forbidden, serialized)
		}
	}
}

func TestGuestDeploymentCountsForZeroAndAllVisible(t *testing.T) {
	tests := []struct {
		name        string
		records     []DeploymentRecord
		wantSummary GuestDeploymentSummary
		wantApps    int
	}{
		{
			name: "zero visible",
			records: []DeploymentRecord{
				{App: "hidden-one", GuestVisible: false},
				{App: "hidden-two", GuestVisible: false},
			},
			wantSummary: GuestDeploymentSummary{Total: 2, Showing: 0, Hidden: 2},
			wantApps:    0,
		},
		{
			name: "all visible",
			records: []DeploymentRecord{
				{App: "visible-one", Container: "one", GuestVisible: true},
				{App: "visible-two", Container: "two", GuestVisible: true},
			},
			wantSummary: GuestDeploymentSummary{Total: 2, Showing: 2, Hidden: 0},
			wantApps:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreStore := replaceStoreForTest(t, staticDeploymentStore{records: tt.records})
			defer restoreStore()
			replaceCommandRunnerForTest(
				t,
				func(_ string, _ string, _ ...string) (string, error) {
					return "running\n", nil
				},
			)

			req := httptest.NewRequest(
				http.MethodGet,
				"https://minideploy.reactorlab.dev/api/guest/deployments",
				nil,
			)
			recorder := httptest.NewRecorder()
			publicRoutes(nil).ServeHTTP(recorder, req)

			var response GuestDeploymentsResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Summary != tt.wantSummary || len(response.Deployments) != tt.wantApps {
				t.Fatalf("response = %#v; want summary %#v and %d apps", response, tt.wantSummary, tt.wantApps)
			}
		})
	}
}

func TestGuestVisibilityDoesNotChangeApplicationProxyRoutes(t *testing.T) {
	hidden := fullstackTestRecord("direct-app", "visibility")
	hidden.GuestVisible = false
	visible := hidden
	visible.GuestVisible = true

	hiddenLocal, hiddenPublic, err := fullstackProxyRouteFragments(
		hidden,
		0,
		"direct-app.reactorlab.dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	visibleLocal, visiblePublic, err := fullstackProxyRouteFragments(
		visible,
		0,
		"direct-app.reactorlab.dev",
	)
	if err != nil {
		t.Fatal(err)
	}
	if hiddenLocal != visibleLocal || hiddenPublic != visiblePublic {
		t.Fatal("Guest View visibility changed application proxy routes")
	}
	if !strings.Contains(hiddenPublic, "direct-app.reactorlab.dev") ||
		!strings.Contains(hiddenPublic, "reverse_proxy") {

		t.Fatalf("hidden application lost its direct public route: %s", hiddenPublic)
	}
}
