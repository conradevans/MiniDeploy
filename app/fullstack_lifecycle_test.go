package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fullstackCommandRecorder struct {
	app          string
	cloneCount   int
	dockerRuns   [][]string
	dockerEnv    map[string]string
	removed      []string
	networksMade []string
}

func (r *fullstackCommandRecorder) run(
	dir string,
	name string,
	args ...string,
) (string, error) {
	if name == "git" {
		if len(args) > 0 && args[0] == "clone" {
			r.cloneCount++
		}
		return executeCommand(dir, name, args...)
	}
	if name != "docker" || len(args) == 0 {
		return "", nil
	}

	switch args[0] {
	case "run":
		r.dockerRuns = append(r.dockerRuns, slices.Clone(args))
		service := ""
		envFile := ""
		for index, argument := range args {
			if argument == "--network-alias" && index+1 < len(args) {
				service = args[index+1]
			}
			if argument == "--env-file" && index+1 < len(args) {
				envFile = args[index+1]
			}
		}
		if envFile != "" {
			content, err := os.ReadFile(envFile)
			if err != nil {
				return "", err
			}
			if r.dockerEnv == nil {
				r.dockerEnv = make(map[string]string)
			}
			r.dockerEnv[service] = string(content)
		}
		return "container-id\n", nil
	case "logs":
		return "application log\n", nil
	case "restart":
		return args[1] + "\n", nil
	case "rm":
		r.removed = append(r.removed, args[len(args)-1])
		return "", nil
	case "inspect":
		if len(args) >= 4 && args[1] == "-f" {
			template := args[2]
			resource := args[3]
			if strings.Contains(template, ".State.Status") {
				return "running\n", nil
			}
			service := fullstackFrontendService
			if strings.Contains(resource, "backend") {
				service = fullstackBackendService
			}
			return strings.Join([]string{"true", r.app, service}, "|") + "\n", nil
		}
		return "{}\n", nil
	case "network":
		if len(args) < 2 {
			return "", nil
		}
		switch args[1] {
		case "create":
			r.networksMade = append(r.networksMade, args[len(args)-1])
			return args[len(args)-1] + "\n", nil
		case "inspect":
			return "true|" + r.app + "\n", nil
		case "rm":
			r.removed = append(r.removed, args[len(args)-1])
			return "", nil
		}
	case "image":
		if len(args) > 1 && args[1] == "rm" {
			r.removed = append(r.removed, args[len(args)-1])
		}
		return "", nil
	}
	return "", nil
}

type fullstackLifecycleHarness struct {
	app       string
	repo      string
	old       DeploymentRecord
	store     *JSONStore
	commands  *fullstackCommandRecorder
	buildDirs []string
	logsDir   string
	syncCount int
}

func newFullstackLifecycleHarness(
	t *testing.T,
) *fullstackLifecycleHarness {
	t.Helper()
	root := t.TempDir()
	app := "phase3-project"
	repo := filepath.Join(root, app)
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	writeFullstackFixture(t, repo, true, true)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "MiniDeploy Test"},
		{"add", "."},
		{"commit", "-m", "fixture"},
	} {
		if output, err := executeCommand(repo, "git", args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}

	previousDeploymentsDir := deploymentsDir
	deploymentsDir = filepath.Join(root, "managed-deployments")
	t.Cleanup(func() { deploymentsDir = previousDeploymentsDir })
	if err := os.MkdirAll(deploymentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	currentPath, err := managedDeploymentPath(app)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := executeCommand(
		"",
		"git",
		"clone",
		"--",
		repo,
		currentPath,
	); err != nil {
		t.Fatalf("prepare current checkout: %v: %s", err, output)
	}

	previousDeploymentLogsDir := deploymentLogsDir
	deploymentLogsDir = filepath.Join(root, "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousDeploymentLogsDir })

	previousRuntimeStore := runtimeEnvironmentStore
	runtimeEnvironmentStore = newRuntimeEnvironmentFileStore(
		filepath.Join(root, "secrets"),
	)
	t.Cleanup(func() { runtimeEnvironmentStore = previousRuntimeStore })
	if err := runtimeEnvironmentStore.Replace(
		app,
		map[string]string{"ACCEPTANCE_MESSAGE": "CURRENT_SECRET_SENTINEL"},
	); err != nil {
		t.Fatal(err)
	}

	metadataStore := NewJSONStore(filepath.Join(root, "deployments.json"))
	previousStore := store
	store = metadataStore
	t.Cleanup(func() { store = previousStore })

	previousHistory := historyStore
	historyStore = NewJSONHistoryStore(filepath.Join(root, "history.json"), 3)
	t.Cleanup(func() { historyStore = previousHistory })

	old := fullstackTestRecord(app, "old")
	old.RepoURL = repo
	if err := metadataStore.Save(old); err != nil {
		t.Fatal(err)
	}

	commands := &fullstackCommandRecorder{app: app}
	replaceCommandRunnerForTest(t, commands.run)

	previousBuild := fullstackBuildDeploymentImage
	previousStartup := fullstackVerifyStartup
	previousHealth := fullstackVerifyHTTPHealth
	previousPort := fullstackFindAvailablePort
	previousSync := fullstackSynchronizeProxyRoutes
	t.Cleanup(func() {
		fullstackBuildDeploymentImage = previousBuild
		fullstackVerifyStartup = previousStartup
		fullstackVerifyHTTPHealth = previousHealth
		fullstackFindAvailablePort = previousPort
		fullstackSynchronizeProxyRoutes = previousSync
	})

	harness := &fullstackLifecycleHarness{
		app:      app,
		repo:     repo,
		old:      old,
		store:    metadataStore,
		commands: commands,
		logsDir:  deploymentLogsDir,
	}
	fullstackBuildDeploymentImage = func(
		repositoryPath string,
		_ string,
		_ deploymentBuildPlan,
	) (string, error) {
		harness.buildDirs = append(harness.buildDirs, repositoryPath)
		return "built\n", nil
	}
	fullstackVerifyStartup = func(string) error { return nil }
	fullstackVerifyHTTPHealth = func(int, string) error { return nil }
	nextPort := 8500
	fullstackFindAvailablePort = func(int, int) (int, error) {
		port := nextPort
		nextPort++
		return port, nil
	}
	fullstackSynchronizeProxyRoutes = func() error {
		harness.syncCount++
		return nil
	}
	return harness
}

func (h *fullstackLifecycleHarness) requireOldStillActive(t *testing.T) {
	t.Helper()
	got, err := h.store.Get(h.app)
	if err != nil {
		t.Fatalf("Get(old) error: %v", err)
	}
	if got.Image != h.old.Image || got.Network != h.old.Network {
		t.Fatalf("old release was replaced: %#v", got)
	}
	for _, service := range h.old.Services {
		if slices.Contains(h.commands.removed, service.Container) ||
			slices.Contains(h.commands.removed, service.Image) {

			t.Fatalf("old resource was removed on failure: %s", service.Name)
		}
	}
	if slices.Contains(h.commands.removed, h.old.Network) {
		t.Fatal("old network was removed on failure")
	}
}

func TestFullstackRedeployClonesOnceBuildsBothAndCutsOver(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	record, err := safeRedeploy(h.old, nil)
	if err != nil {
		t.Fatalf("safeRedeploy() error: %v", err)
	}
	if h.commands.cloneCount != 1 {
		t.Fatalf("clone count = %d; want 1", h.commands.cloneCount)
	}
	if len(h.buildDirs) != 2 ||
		filepath.Base(h.buildDirs[0]) != "frontend" ||
		filepath.Base(h.buildDirs[1]) != "backend" {

		t.Fatalf("build directories = %v", h.buildDirs)
	}
	if len(h.commands.dockerRuns) != 2 {
		t.Fatalf("docker runs = %d; want 2", len(h.commands.dockerRuns))
	}
	if _, ok := h.commands.dockerEnv[fullstackFrontendService]; ok {
		t.Fatal("frontend received a runtime environment file")
	}
	backendEnvironment := h.commands.dockerEnv[fullstackBackendService]
	if !strings.Contains(backendEnvironment, "ACCEPTANCE_MESSAGE=") ||
		!strings.Contains(backendEnvironment, "PORT=3000") {

		t.Fatal("backend runtime environment was not injected")
	}
	for _, args := range h.commands.dockerRuns {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--network-alias frontend") &&
			strings.Contains(joined, "--env-file") {

			t.Fatal("frontend received a runtime environment file")
		}
		if strings.Contains(joined, "CURRENT_SECRET_SENTINEL") {
			t.Fatal("secret value leaked into Docker command")
		}
	}
	if h.syncCount != 1 {
		t.Fatalf("Caddy sync count = %d; want 1", h.syncCount)
	}
	if record.Network == h.old.Network || len(record.Services) != 2 {
		t.Fatalf("candidate release is invalid: %#v", record)
	}
	for _, service := range h.old.Services {
		if !slices.Contains(h.commands.removed, service.Container) {
			t.Fatalf("old %s container was not retired", service.Name)
		}
	}
	if !slices.Contains(h.commands.removed, h.old.Network) {
		t.Fatal("old network was not retired")
	}
	versions, err := historyStore.List(h.app)
	if err != nil || len(versions) != 1 || len(versions[0].Services) != 2 {
		t.Fatalf("paired history = %#v, error=%v", versions, err)
	}
	environment, err := runtimeEnvironmentStore.Load(h.app)
	if err != nil || environment["ACCEPTANCE_MESSAGE"] != "CURRENT_SECRET_SENTINEL" {
		t.Fatalf("runtime environment was not preserved: %#v, %v", environment, err)
	}

	secretPath, err := runtimeEnvironmentStore.path(h.app)
	if err != nil {
		t.Fatal(err)
	}
	secretInfo, err := os.Stat(secretPath)
	if err != nil || secretInfo.Mode().Perm() != 0600 {
		t.Fatalf("secret permissions = %v, error=%v", secretInfo, err)
	}
	logPath, err := deploymentLogPath(h.app)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := detectDeploymentStrategy(h.repo, deploymentConfig{})
	if err != nil {
		t.Fatal(err)
	}
	frontendPlan := fullstackServicePlanForTest(t, plan, fullstackFrontendService)
	backendPlan := fullstackServicePlanForTest(t, plan, fullstackBackendService)
	localRoutes, publicRoutes, err := fullstackProxyRouteFragments(
		record,
		0,
		publicHostnameForApp(record.App),
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"deployment metadata": string(mustReadFile(t, h.store.path)),
		"history metadata":    string(mustReadFile(t, historyStore.path)),
		"deployment logs":     string(mustReadFile(t, logPath)),
		"frontend Dockerfile": frontendPlan.Build.GeneratedDockerfile,
		"backend Dockerfile":  backendPlan.Build.GeneratedDockerfile,
		"local Caddy routes":  localRoutes,
		"public Caddy routes": publicRoutes,
	} {
		if strings.Contains(content, "CURRENT_SECRET_SENTINEL") {
			t.Fatalf("secret value leaked into %s", name)
		}
	}
}

func TestFullstackRedeployHealthFailureKeepsOldRelease(t *testing.T) {
	for _, test := range []struct {
		name        string
		failurePort int
	}{
		{name: "frontend", failurePort: 8501},
		{name: "backend", failurePort: 8500},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newFullstackLifecycleHarness(t)
			fullstackVerifyHTTPHealth = func(port int, _ string) error {
				if port == test.failurePort {
					return fmt.Errorf("injected %s failure", test.name)
				}
				return nil
			}
			_, err := safeRedeploy(h.old, nil)
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("health failure error = %v", err)
			}
			if h.syncCount != 0 {
				t.Fatalf("Caddy changed before both healthy: %d", h.syncCount)
			}
			h.requireOldStillActive(t)
		})
	}
}

func TestFullstackRedeployBuildFailureKeepsOldRelease(t *testing.T) {
	for _, failedService := range []string{"frontend", "backend"} {
		t.Run(failedService, func(t *testing.T) {
			h := newFullstackLifecycleHarness(t)
			fullstackBuildDeploymentImage = func(
				repositoryPath string,
				_ string,
				_ deploymentBuildPlan,
			) (string, error) {
				if filepath.Base(repositoryPath) == failedService {
					return "failure", errors.New("injected build failure")
				}
				return "built", nil
			}
			if _, err := safeRedeploy(h.old, nil); err == nil {
				t.Fatal("expected build failure")
			}
			if h.syncCount != 0 {
				t.Fatal("Caddy changed after build failure")
			}
			h.requireOldStillActive(t)
		})
	}
}

func TestFullstackRedeployCaddyFailureRestoresOldRelease(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	fullstackSynchronizeProxyRoutes = func() error {
		h.syncCount++
		if h.syncCount == 1 {
			return errors.New("injected Caddy failure")
		}
		return nil
	}
	if _, err := safeRedeploy(h.old, map[string]string{
		"ACCEPTANCE_MESSAGE": "REPLACEMENT_SECRET_SENTINEL",
	}); err == nil {
		t.Fatal("expected Caddy failure")
	}
	if h.syncCount != 2 {
		t.Fatalf("Caddy sync count = %d; want cutover plus restore", h.syncCount)
	}
	h.requireOldStillActive(t)
	environment, err := runtimeEnvironmentStore.Load(h.app)
	if err != nil || environment["ACCEPTANCE_MESSAGE"] != "CURRENT_SECRET_SENTINEL" {
		t.Fatalf("old environment was not restored: %#v, %v", environment, err)
	}
}

type rejectingCandidateStore struct {
	old DeploymentRecord
}

func (s *rejectingCandidateStore) Save(record DeploymentRecord) error {
	if record.Image != s.old.Image {
		return errors.New("injected metadata save failure")
	}
	s.old = record
	return nil
}

func (s *rejectingCandidateStore) Get(app string) (DeploymentRecord, error) {
	if app != s.old.App {
		return DeploymentRecord{}, ErrDeploymentNotFound
	}
	return s.old, nil
}

func (s *rejectingCandidateStore) List() ([]DeploymentRecord, error) {
	return []DeploymentRecord{s.old}, nil
}

func (s *rejectingCandidateStore) Delete(string) error { return nil }

func TestFullstackRedeployMetadataFailureRestoresOldRelease(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	rejecting := &rejectingCandidateStore{old: h.old}
	store = rejecting
	if _, err := safeRedeploy(h.old, nil); err == nil {
		t.Fatal("expected metadata save failure")
	}
	if h.syncCount != 0 {
		t.Fatal("Caddy changed after metadata save failure")
	}
	if rejecting.old.Image != h.old.Image {
		t.Fatal("candidate metadata replaced old release")
	}
	h.requireOldStillActiveWithRecord(t, rejecting.old)
}

func (h *fullstackLifecycleHarness) requireOldStillActiveWithRecord(
	t *testing.T,
	record DeploymentRecord,
) {
	t.Helper()
	if record.Image != h.old.Image || record.Network != h.old.Network {
		t.Fatalf("old release was replaced: %#v", record)
	}
	for _, service := range h.old.Services {
		if slices.Contains(h.commands.removed, service.Container) {
			t.Fatalf("old %s container removed", service.Name)
		}
	}
}

func TestFullstackRedeployReplacesAndClearsBackendEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		replacement map[string]string
		wantNames   []string
		wantKey     string
	}{
		{
			name: "replace",
			replacement: map[string]string{
				"JWT_SECRET": "REPLACEMENT_SECRET_SENTINEL",
			},
			wantNames: []string{"JWT_SECRET"},
			wantKey:   "JWT_SECRET",
		},
		{
			name:        "clear",
			replacement: map[string]string{},
			wantNames:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newFullstackLifecycleHarness(t)
			record, err := safeRedeploy(h.old, test.replacement)
			if err != nil {
				t.Fatalf("safeRedeploy() error: %v", err)
			}
			if !slices.Equal(record.EnvironmentVariables, test.wantNames) {
				t.Fatalf(
					"environment names = %v; want %v",
					record.EnvironmentVariables,
					test.wantNames,
				)
			}
			loaded, err := runtimeEnvironmentStore.Load(h.app)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded) != len(test.replacement) {
				t.Fatalf("stored environment count = %d; want %d", len(loaded), len(test.replacement))
			}
			backendEnvironment := h.commands.dockerEnv[fullstackBackendService]
			if !strings.Contains(backendEnvironment, "PORT=3000") ||
				strings.Contains(backendEnvironment, "CURRENT_SECRET_SENTINEL") {

				t.Fatal("backend did not receive only the current runtime environment")
			}
			if test.wantKey != "" &&
				!strings.Contains(backendEnvironment, test.wantKey+"=") {

				t.Fatal("replacement variable was not injected into backend")
			}
			if _, ok := h.commands.dockerEnv[fullstackFrontendService]; ok {
				t.Fatal("frontend received a runtime environment file")
			}
		})
	}
}

func TestFullstackRollbackRestoresPairedImagesWithCurrentEnvironment(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	previous := fullstackTestRecord(h.app, "previous")
	previous.RepoURL = h.repo
	if _, err := historyStore.Push(previous); err != nil {
		t.Fatal(err)
	}

	record, err := rollbackDeployment(h.old)
	if err != nil {
		t.Fatalf("rollbackDeployment() error: %v", err)
	}
	if h.syncCount != 1 {
		t.Fatalf("Caddy sync count = %d; want 1", h.syncCount)
	}
	for _, name := range []string{"frontend", "backend"} {
		want, _ := deploymentServiceByName(previous, name)
		got, _ := deploymentServiceByName(record, name)
		if got.Image != want.Image || !strings.Contains(got.Container, "-rollback-") {
			t.Fatalf("rollback %s = %#v; want image %s", name, got, want.Image)
		}
	}
	if !reflectStringSlicesEqual(
		record.EnvironmentVariables,
		[]string{"ACCEPTANCE_MESSAGE"},
	) {
		t.Fatalf("rollback environment names = %v", record.EnvironmentVariables)
	}
	if _, ok := h.commands.dockerEnv[fullstackFrontendService]; ok {
		t.Fatal("rollback injected environment into frontend")
	}
	if !strings.Contains(
		h.commands.dockerEnv[fullstackBackendService],
		"ACCEPTANCE_MESSAGE=CURRENT_SECRET_SENTINEL",
	) {
		t.Fatal("rollback did not use the current backend environment")
	}
}

func reflectStringSlicesEqual(left []string, right []string) bool {
	return slices.Equal(left, right)
}

func TestFullstackDeleteRemovesOnlyProjectResources(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	if _, err := historyStore.Push(
		fullstackTestRecord(h.app, "historical"),
	); err != nil {
		t.Fatal(err)
	}
	if err := deleteFullstackProject(h.old); err != nil {
		t.Fatalf("deleteFullstackProject() error: %v", err)
	}
	if _, err := h.store.Get(h.app); !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatalf("metadata remains after delete: %v", err)
	}
	deployPath, err := managedDeploymentPath(h.app)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(deployPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkout remains after delete: %v", err)
	}
	if _, err := runtimeEnvironmentStore.Load(h.app); err != nil {
		t.Fatalf("load deleted environment: %v", err)
	}
	secretPath, err := runtimeEnvironmentStore.path(h.app)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secretPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("secret file remains after delete: %v", err)
	}
	for _, service := range h.old.Services {
		if !slices.Contains(h.commands.removed, service.Container) {
			t.Fatalf("%s container was not deleted", service.Name)
		}
	}
	if !slices.Contains(h.commands.removed, h.old.Network) {
		t.Fatal("project network was not deleted")
	}
	if h.syncCount != 1 {
		t.Fatalf("delete Caddy sync count = %d; want 1", h.syncCount)
	}
}

func TestWebhookRedeploysWholeFullstackProjectAndPreservesEnvironment(t *testing.T) {
	h := newFullstackLifecycleHarness(t)
	selected, ok := deploymentForWebhook(
		[]DeploymentRecord{h.old},
		h.repo,
	)
	if !ok || selected.Strategy != deploymentStrategyFullstackViteNode ||
		len(selected.Services) != 2 {

		t.Fatalf("webhook selection lost project services: %#v", selected)
	}

	redeployed, err := safeRedeploy(selected, nil)
	if err != nil {
		t.Fatalf("webhook-style safeRedeploy() error: %v", err)
	}
	if h.commands.cloneCount != 1 || len(redeployed.Services) != 2 {
		t.Fatalf(
			"webhook redeploy clone count=%d services=%d",
			h.commands.cloneCount,
			len(redeployed.Services),
		)
	}
	environment, err := runtimeEnvironmentStore.Load(h.app)
	if err != nil || environment["ACCEPTANCE_MESSAGE"] != "CURRENT_SECRET_SENTINEL" {
		t.Fatal("webhook redeploy did not preserve the backend environment")
	}
}
