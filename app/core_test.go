package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "HTTPS repository",
			url:  "https://github.com/conradevans/MyScheduler.git",
			want: "MyScheduler",
		},
		{
			name: "repository without git suffix",
			url:  "https://github.com/conradevans/GolfMullet",
			want: "GolfMullet",
		},
		{
			name: "trailing slash",
			url:  "https://github.com/conradevans/MiniDeploy.git/",
			want: "MiniDeploy",
		},
		{
			name: "empty URL",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoName(tt.url)

			if got != tt.want {
				t.Fatalf(
					"repoName(%q) = %q; want %q",
					tt.url,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestDeploymentConfigDefaults(t *testing.T) {
	if got := normalizedContainerPort(0); got != 80 {
		t.Fatalf(
			"normalizedContainerPort(0) = %d; want 80",
			got,
		)
	}

	if got := normalizedContainerPort(3000); got != 3000 {
		t.Fatalf(
			"normalizedContainerPort(3000) = %d; want 3000",
			got,
		)
	}

	if got := normalizedHealthPath(""); got != "/" {
		t.Fatalf(
			"normalizedHealthPath(\"\") = %q; want /",
			got,
		)
	}

	if got := normalizedHealthPath("   /health   "); got != "/health" {
		t.Fatalf(
			"normalizedHealthPath trimmed value = %q; want /health",
			got,
		)
	}
}

func TestValidateDeploymentConfig(t *testing.T) {
	tests := []struct {
		name          string
		containerPort int
		healthPath    string
		wantErr       bool
	}{
		{
			name:          "valid default HTTP app",
			containerPort: 80,
			healthPath:    "/",
		},
		{
			name:          "valid Node app",
			containerPort: 3000,
			healthPath:    "/health",
		},
		{
			name:          "port too low",
			containerPort: 0,
			healthPath:    "/",
			wantErr:       true,
		},
		{
			name:          "port too high",
			containerPort: 65536,
			healthPath:    "/",
			wantErr:       true,
		},
		{
			name:          "health path missing slash",
			containerPort: 3000,
			healthPath:    "health",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeploymentConfig(
				tt.containerPort,
				tt.healthPath,
			)

			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf(
					"unexpected validation error: %v",
					err,
				)
			}
		})
	}
}

func TestNormalizeDeploymentRecord(t *testing.T) {
	record := normalizeDeploymentRecord(
		DeploymentRecord{
			App:           "example",
			ContainerPort: 0,
			HealthPath:    "",
		},
	)

	if record.ContainerPort != defaultContainerPort {
		t.Fatalf(
			"ContainerPort = %d; want %d",
			record.ContainerPort,
			defaultContainerPort,
		)
	}

	if record.HealthPath != defaultHealthPath {
		t.Fatalf(
			"HealthPath = %q; want %q",
			record.HealthPath,
			defaultHealthPath,
		)
	}
}

func TestJSONStoreLifecycle(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"deployments.json",
	)

	s := NewJSONStore(path)

	_, err := s.Get("missing")
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf(
			"Get(missing) error = %v; want ErrDeploymentNotFound",
			err,
		)
	}

	record := DeploymentRecord{
		App:           "test-app",
		RepoURL:       "https://github.com/example/test-app.git",
		Container:     "minideploy-test-app",
		Image:         "minideploy-test-app:v1",
		Port:          8081,
		ContainerPort: 3000,
		HealthPath:    "/health",
	}

	if err := s.Save(record); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := s.Get(record.App)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if got != record {
		t.Fatalf(
			"Get() = %#v; want %#v",
			got,
			record,
		)
	}

	record.Port = 8082
	record.Image = "minideploy-test-app:v2"

	if err := s.Save(record); err != nil {
		t.Fatalf("Save(update) error: %v", err)
	}

	records, err := s.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf(
			"List() returned %d records; want 1",
			len(records),
		)
	}

	if records[0].Port != 8082 {
		t.Fatalf(
			"updated Port = %d; want 8082",
			records[0].Port,
		)
	}

	if err := s.Delete(record.App); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = s.Get(record.App)
	if !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf(
			"Get() after Delete error = %v; want ErrDeploymentNotFound",
			err,
		)
	}
}

func TestHistoryStorePrunesOldVersions(t *testing.T) {
	path := filepath.Join(
		t.TempDir(),
		"history.json",
	)

	s := NewJSONHistoryStore(path, 2)

	v1 := DeploymentRecord{
		App:   "example",
		Image: "example:v1",
		Port:  8081,
	}

	v2 := DeploymentRecord{
		App:   "example",
		Image: "example:v2",
		Port:  8082,
	}

	v3 := DeploymentRecord{
		App:   "example",
		Image: "example:v3",
		Port:  8083,
	}

	if _, err := s.Push(v1); err != nil {
		t.Fatalf("Push(v1) error: %v", err)
	}

	if _, err := s.Push(v2); err != nil {
		t.Fatalf("Push(v2) error: %v", err)
	}

	pruned, err := s.Push(v3)
	if err != nil {
		t.Fatalf("Push(v3) error: %v", err)
	}

	if len(pruned) != 1 {
		t.Fatalf(
			"pruned versions = %d; want 1",
			len(pruned),
		)
	}

	if pruned[0].Image != "example:v1" {
		t.Fatalf(
			"pruned image = %q; want example:v1",
			pruned[0].Image,
		)
	}

	versions, err := s.List("example")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf(
			"history length = %d; want 2",
			len(versions),
		)
	}

	if versions[0].Image != "example:v3" {
		t.Fatalf(
			"latest history image = %q; want example:v3",
			versions[0].Image,
		)
	}

	if versions[1].Image != "example:v2" {
		t.Fatalf(
			"second history image = %q; want example:v2",
			versions[1].Image,
		)
	}
}

func githubTestSignature(
	body []byte,
	secret string,
) string {
	mac := hmac.New(
		sha256.New,
		[]byte(secret),
	)

	_, _ = mac.Write(body)

	return "sha256=" + hex.EncodeToString(
		mac.Sum(nil),
	)
}

func TestValidGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "unit-test-secret"

	valid := githubTestSignature(
		body,
		secret,
	)

	if !validGitHubSignature(
		body,
		valid,
		secret,
	) {
		t.Fatal("valid GitHub signature was rejected")
	}

	if validGitHubSignature(
		body,
		"sha256=deadbeef",
		secret,
	) {
		t.Fatal("invalid GitHub signature was accepted")
	}

	if validGitHubSignature(
		body,
		"deadbeef",
		secret,
	) {
		t.Fatal("signature without sha256 prefix was accepted")
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "https://github.com/User/Project.git",
			want:  "https://github.com/user/project",
		},
		{
			input: " HTTPS://GITHUB.COM/User/Project.git/ ",
			want:  "https://github.com/user/project",
		},
		{
			input: "https://github.com/user/project",
			want:  "https://github.com/user/project",
		},
	}

	for _, tt := range tests {
		got := normalizeRepoURL(tt.input)

		if got != tt.want {
			t.Fatalf(
				"normalizeRepoURL(%q) = %q; want %q",
				tt.input,
				got,
				tt.want,
			)
		}
	}
}

func TestGitHubWebhookRejectsInvalidSignature(t *testing.T) {
	t.Setenv(
		"MINIDEPLOY_GITHUB_WEBHOOK_SECRET",
		"unit-test-secret",
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/webhooks/github",
		strings.NewReader(`{"zen":"test"}`),
	)

	req.Header.Set(
		"X-GitHub-Event",
		"ping",
	)

	req.Header.Set(
		"X-Hub-Signature-256",
		"sha256=deadbeef",
	)

	rec := httptest.NewRecorder()

	githubWebhookHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d; want %d",
			rec.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestGitHubWebhookPing(t *testing.T) {
	const secret = "unit-test-secret"

	t.Setenv(
		"MINIDEPLOY_GITHUB_WEBHOOK_SECRET",
		secret,
	)

	body := []byte(`{"zen":"test"}`)

	req := httptest.NewRequest(
		http.MethodPost,
		"/webhooks/github",
		strings.NewReader(string(body)),
	)

	req.Header.Set(
		"X-GitHub-Event",
		"ping",
	)

	req.Header.Set(
		"X-Hub-Signature-256",
		githubTestSignature(
			body,
			secret,
		),
	)

	rec := httptest.NewRecorder()

	githubWebhookHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d; want %d; body=%s",
			rec.Code,
			http.StatusOK,
			rec.Body.String(),
		)
	}

	if !strings.Contains(
		rec.Body.String(),
		`"status":"ok"`,
	) {
		t.Fatalf(
			"unexpected response body: %s",
			rec.Body.String(),
		)
	}
}
