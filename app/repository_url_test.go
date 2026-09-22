package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployRejectsHTTPRepositoryCredentialsBeforeClone(t *testing.T) {
	restoreStore := replaceStoreForTest(t, staticDeploymentStore{})
	defer restoreStore()

	commandCalled := false
	replaceCommandRunnerForTest(
		t,
		func(string, string, ...string) (string, error) {
			commandCalled = true
			return "", nil
		},
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"http://localhost:9000/deploy",
		strings.NewReader(
			`{"repoUrl":"https://user:token@github.com/owner/repo.git"}`,
		),
	)
	response := httptest.NewRecorder()

	deployHandler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf(
			"status = %d; want %d; body=%s",
			response.Code,
			http.StatusBadRequest,
			response.Body.String(),
		)
	}
	if !strings.Contains(response.Body.String(), "must not contain credentials") {
		t.Fatalf("unexpected rejection: %s", response.Body.String())
	}
	if commandCalled {
		t.Fatal("credential-bearing repository URL reached deployment commands")
	}
}

func TestRepositoryURLValidationRejectsHTTPQueryAndFragment(t *testing.T) {
	for _, repositoryURL := range []string{
		"https://github.com/owner/repo.git?token=secret",
		"https://github.com/owner/repo.git#token=secret",
	} {
		t.Run(repositoryURL, func(t *testing.T) {
			if err := validateRepositoryURL(repositoryURL); err == nil {
				t.Fatalf("validateRepositoryURL(%q) unexpectedly succeeded", repositoryURL)
			}
		})
	}
}

func TestRepositoryURLValidationAcceptsNormalGitForms(t *testing.T) {
	for _, repositoryURL := range []string{
		"https://github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
	} {
		t.Run(repositoryURL, func(t *testing.T) {
			if err := validateRepositoryURL(repositoryURL); err != nil {
				t.Fatalf("validateRepositoryURL(%q) error: %v", repositoryURL, err)
			}
		})
	}
}

func TestAcceptedRepositoryURLsPersistWithoutUnsafeComponents(t *testing.T) {
	root := t.TempDir()
	metadata := NewJSONStore(filepath.Join(root, "deployments.json"))
	history := NewJSONHistoryStore(filepath.Join(root, "history.json"), 3)

	for index, repositoryURL := range []string{
		"https://github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
	} {
		app := fmt.Sprintf("safe-repository-%d", index)
		record := DeploymentRecord{
			App:     app,
			RepoURL: repositoryURL,
			Image:   fmt.Sprintf("minideploy-%s:v1", app),
		}

		if err := metadata.Save(record); err != nil {
			t.Fatalf("save accepted repository URL %q: %v", repositoryURL, err)
		}
		persisted, err := metadata.Get(app)
		if err != nil {
			t.Fatalf("get accepted repository URL %q: %v", repositoryURL, err)
		}
		assertPersistedRepositoryURLSafe(t, persisted.RepoURL)

		if _, err := history.Push(record); err != nil {
			t.Fatalf("archive accepted repository URL %q: %v", repositoryURL, err)
		}
		versions, err := history.List(app)
		if err != nil || len(versions) != 1 {
			t.Fatalf(
				"history for accepted repository URL %q = %#v, %v",
				repositoryURL,
				versions,
				err,
			)
		}
		assertPersistedRepositoryURLSafe(t, versions[0].RepoURL)
	}
}

func TestRepositoryPersistenceRejectsUnsafeHTTPComponents(t *testing.T) {
	root := t.TempDir()
	metadata := NewJSONStore(filepath.Join(root, "deployments.json"))
	history := NewJSONHistoryStore(filepath.Join(root, "history.json"), 3)

	for index, repositoryURL := range []string{
		"https://user:token@github.com/owner/repo.git",
		"https://github.com/owner/repo.git?token=secret",
		"https://github.com/owner/repo.git#token=secret",
		"ssh://git:token@github.com/owner/repo.git",
	} {
		record := DeploymentRecord{
			App:     fmt.Sprintf("unsafe-repository-%d", index),
			RepoURL: repositoryURL,
		}
		if err := metadata.Save(record); err == nil {
			t.Fatalf("metadata accepted unsafe repository URL %q", repositoryURL)
		}
		if _, err := history.Push(record); err == nil {
			t.Fatalf("history push accepted unsafe repository URL %q", repositoryURL)
		}
		if _, err := history.Set(
			record.App,
			[]DeploymentVersion{{App: record.App, RepoURL: repositoryURL}},
		); err == nil {
			t.Fatalf("history set accepted unsafe repository URL %q", repositoryURL)
		}
	}
}

func assertPersistedRepositoryURLSafe(t *testing.T, repositoryURL string) {
	t.Helper()

	if strings.HasPrefix(repositoryURL, "git@github.com:") {
		if strings.ContainsAny(repositoryURL, "?#") {
			t.Fatalf(
				"persisted SCP-style repository URL has query or fragment: %q",
				repositoryURL,
			)
		}
		return
	}

	parsed, err := url.Parse(repositoryURL)
	if err != nil {
		t.Fatalf("parse persisted repository URL %q: %v", repositoryURL, err)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || strings.Contains(repositoryURL, "#") {

		t.Fatalf("persisted repository URL has query or fragment: %q", repositoryURL)
	}
	if (strings.EqualFold(parsed.Scheme, "http") ||
		strings.EqualFold(parsed.Scheme, "https")) &&
		parsed.User != nil {

		t.Fatalf("persisted HTTP repository URL has userinfo: %q", repositoryURL)
	}
	if strings.EqualFold(parsed.Scheme, "ssh") && parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			t.Fatalf("persisted SSH repository URL has a password: %q", repositoryURL)
		}
	}
}
