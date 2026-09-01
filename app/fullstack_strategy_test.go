package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFullstackFixture(
	t *testing.T,
	root string,
	frontendLock bool,
	backendLock bool,
) {
	t.Helper()
	frontend := filepath.Join(root, fullstackFrontendService)
	backend := filepath.Join(root, fullstackBackendService)
	writeViteManifest(t, frontend, "vite build")
	writeExpressManifest(
		t,
		backend,
		"node server.js",
		`"dependencies": {"express": "5.1.0"}`,
	)
	if frontendLock {
		writePackageLock(t, frontend)
	}
	if backendLock {
		writePackageLock(t, backend)
	}
}

func fullstackServicePlanForTest(
	t *testing.T,
	plan deploymentBuildPlan,
	name string,
) deploymentServiceBuildPlan {
	t.Helper()
	service, ok := fullstackBuildServiceByName(plan, name)
	if !ok {
		t.Fatalf("missing %s service plan: %#v", name, plan)
	}
	return service
}

func TestFullstackDetectionAndBuildPlanning(t *testing.T) {
	repository := t.TempDir()
	writeFullstackFixture(t, repository, true, false)

	plan, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{
			ContainerPort: 4321,
			HealthPath:    "/health",
		},
	)
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}
	if plan.Strategy != deploymentStrategyFullstackViteNode {
		t.Fatalf("strategy = %q; want full-stack", plan.Strategy)
	}
	if len(plan.Services) != 2 {
		t.Fatalf("services = %d; want 2", len(plan.Services))
	}

	frontend := fullstackServicePlanForTest(
		t,
		plan,
		fullstackFrontendService,
	)
	backend := fullstackServicePlanForTest(
		t,
		plan,
		fullstackBackendService,
	)
	if frontend.Path != "frontend" ||
		frontend.RepositoryPath != filepath.Join(repository, "frontend") ||
		frontend.Build.Strategy != deploymentStrategyViteStatic ||
		frontend.Build.PackageInstallMode != packageInstallModeCI ||
		frontend.Build.ContainerPort != 80 ||
		frontend.Build.HealthPath != "/" {

		t.Fatalf("unexpected frontend plan: %#v", frontend)
	}
	if backend.Path != "backend" ||
		backend.RepositoryPath != filepath.Join(repository, "backend") ||
		backend.Build.Strategy != deploymentStrategyNodeExpress ||
		backend.Build.PackageInstallMode != packageInstallModeSetup ||
		backend.Build.ContainerPort != 4321 ||
		backend.Build.HealthPath != "/health" {

		t.Fatalf("unexpected backend plan: %#v", backend)
	}
	if frontend.Build.GeneratedDockerfile == "" ||
		backend.Build.GeneratedDockerfile == "" {

		t.Fatal("full-stack service plan did not reuse generated runtimes")
	}
	for _, path := range []string{
		filepath.Join(repository, "frontend", "Dockerfile"),
		filepath.Join(repository, "backend", "Dockerfile"),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("detector modified source with %s", path)
		}
	}
}

func TestFullstackDetectionRejectsIncompleteOrMisleadingLayouts(
	t *testing.T,
) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "missing frontend",
			setup: func(t *testing.T, root string) {
				backend := filepath.Join(root, "backend")
				writeExpressManifest(t, backend, "node server.js", `"dependencies":{"express":"5"}`)
			},
		},
		{
			name: "missing backend",
			setup: func(t *testing.T, root string) {
				writeViteManifest(t, filepath.Join(root, "frontend"), "vite build")
			},
		},
		{
			name: "invalid Vite frontend",
			setup: func(t *testing.T, root string) {
				writeFullstackFixture(t, root, false, false)
				writeViteManifest(t, filepath.Join(root, "frontend"), "webpack")
			},
		},
		{
			name: "invalid Express backend",
			setup: func(t *testing.T, root string) {
				writeFullstackFixture(t, root, false, false)
				writeExpressManifest(t, filepath.Join(root, "backend"), "nodemon server.js", `"dependencies":{"express":"5"}`)
			},
		},
		{
			name: "react scripts frontend is not Express",
			setup: func(t *testing.T, root string) {
				frontend := filepath.Join(root, "frontend")
				writeExpressManifest(t, filepath.Join(root, "backend"), "node server.js", `"dependencies":{"express":"5"}`)
				writeStrategyTestFile(t, frontend, "package.json", `{"scripts":{"build":"react-scripts build","start":"node server.js"},"dependencies":{"express":"5","react-scripts":"5"}}`)
				writeStrategyTestFile(t, frontend, "server.js", "// misleading\n")
			},
		},
		{
			name: "unsupported Vite build does not become Node",
			setup: func(t *testing.T, root string) {
				writeFullstackFixture(t, root, false, false)
				frontend := filepath.Join(root, "frontend")
				writeStrategyTestFile(t, frontend, "package.json", `{"scripts":{"build":"webpack","start":"node server.js"},"dependencies":{"express":"5"},"devDependencies":{"vite":"8"}}`)
				writeStrategyTestFile(t, frontend, "server.js", "// misleading\n")
			},
		},
		{
			name: "arbitrary nested directories are not guessed",
			setup: func(t *testing.T, root string) {
				writeFullstackFixture(t, filepath.Join(root, "apps", "project"), false, false)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			test.setup(t, repository)
			_, err := detectDeploymentStrategy(repository, deploymentConfig{})
			if !errors.Is(err, ErrNoSupportedDeploymentStrategy) {
				t.Fatalf("error = %v; want unsupported", err)
			}
		})
	}
}

func TestRootDockerfilePrecedesValidFullstackLayout(t *testing.T) {
	repository := t.TempDir()
	writeFullstackFixture(t, repository, true, true)
	writeStrategyTestFile(t, repository, "Dockerfile", "FROM scratch\n")

	plan, err := detectDeploymentStrategy(repository, deploymentConfig{})
	if err != nil {
		t.Fatalf("detect strategy: %v", err)
	}
	if plan.Strategy != deploymentStrategyDockerfile || len(plan.Services) != 0 {
		t.Fatalf("root Dockerfile did not retain priority: %#v", plan)
	}
}

func TestFullstackServiceSymlinkContainment(t *testing.T) {
	t.Run("escape rejected", func(t *testing.T) {
		repository := t.TempDir()
		outside := t.TempDir()
		writeViteManifest(t, outside, "vite build")
		backend := filepath.Join(repository, "backend")
		writeExpressManifest(t, backend, "node server.js", `"dependencies":{"express":"5"}`)
		if err := os.Symlink(outside, filepath.Join(repository, "frontend")); err != nil {
			t.Fatalf("Symlink() error: %v", err)
		}
		_, err := detectDeploymentStrategy(repository, deploymentConfig{})
		if err == nil || !strings.Contains(err.Error(), "escapes repository") {
			t.Fatalf("escape error = %v", err)
		}
	})

	t.Run("contained target accepted", func(t *testing.T) {
		repository := t.TempDir()
		frontendTarget := filepath.Join(repository, "ui")
		writeViteManifest(t, frontendTarget, "vite build")
		backend := filepath.Join(repository, "backend")
		writeExpressManifest(t, backend, "node server.js", `"dependencies":{"express":"5"}`)
		if err := os.Symlink("ui", filepath.Join(repository, "frontend")); err != nil {
			t.Fatalf("Symlink() error: %v", err)
		}
		plan, err := detectDeploymentStrategy(repository, deploymentConfig{})
		if err != nil || plan.Strategy != deploymentStrategyFullstackViteNode {
			t.Fatalf("contained symlink plan = %#v, error=%v", plan, err)
		}
	})

	t.Run("manifest escape rejected", func(t *testing.T) {
		repository := t.TempDir()
		frontend := filepath.Join(repository, "frontend")
		if err := os.MkdirAll(frontend, 0755); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "package.json")
		writeStrategyTestFile(t, filepath.Dir(outside), filepath.Base(outside), `{"scripts":{"build":"vite build"},"devDependencies":{"vite":"8"}}`)
		if err := os.Symlink(outside, filepath.Join(frontend, "package.json")); err != nil {
			t.Fatal(err)
		}
		backend := filepath.Join(repository, "backend")
		writeExpressManifest(t, backend, "node server.js", `"dependencies":{"express":"5"}`)
		_, err := detectDeploymentStrategy(repository, deploymentConfig{})
		if err == nil || !strings.Contains(err.Error(), "escapes checkout") {
			t.Fatalf("manifest escape error = %v", err)
		}
	})
}

func TestFullstackMetadataAndHistoryRoundTrip(t *testing.T) {
	record := fullstackTestRecord("project", "old")
	store := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := store.Save(record); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded, err := store.Get(record.App)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if len(loaded.Services) != 2 || loaded.Network != record.Network {
		t.Fatalf("service metadata did not round-trip: %#v", loaded)
	}

	version := deploymentVersion(record)
	restored := version.Record()
	if len(restored.Services) != 2 {
		t.Fatalf("history services = %d; want 2", len(restored.Services))
	}
	for _, name := range []string{"frontend", "backend"} {
		want, _ := deploymentServiceByName(record, name)
		got, _ := deploymentServiceByName(restored, name)
		if got.Image != want.Image ||
			got.Strategy != want.Strategy ||
			got.PackageInstallMode != want.PackageInstallMode {

			t.Fatalf("restored %s = %#v; want %#v", name, got, want)
		}
	}

	legacy := normalizeDeploymentRecord(DeploymentRecord{App: "legacy"})
	if legacy.Strategy != deploymentStrategyDockerfile || len(legacy.Services) != 0 {
		t.Fatalf("legacy record changed: %#v", legacy)
	}
}

func TestGolfMulletRepositoryProducesCanonicalResources(t *testing.T) {
	const repositoryURL = "https://github.com/conradevans/GolfMullet.git"

	repository := t.TempDir()
	writeFullstackFixture(t, repository, true, true)
	plan, err := detectDeploymentStrategy(repository, deploymentConfig{})
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}
	if plan.Strategy != deploymentStrategyFullstackViteNode {
		t.Fatalf("strategy = %q; want %q", plan.Strategy, deploymentStrategyFullstackViteNode)
	}

	app := repoName(repositoryURL)
	record, err := newFullstackReleaseRecord(
		app,
		repositoryURL,
		plan,
		"production-regression",
		"release",
		nil,
	)
	if err != nil {
		t.Fatalf("newFullstackReleaseRecord() error: %v", err)
	}

	if record.App != "golfmullet" {
		t.Fatalf("record app = %q; want golfmullet", record.App)
	}
	if record.RepoURL != repositoryURL {
		t.Fatalf("record repository URL = %q; want original %q", record.RepoURL, repositoryURL)
	}
	metadataStore := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := metadataStore.Save(record); err != nil {
		t.Fatalf("persist canonical record: %v", err)
	}
	persisted, err := metadataStore.Get("golfmullet")
	if err != nil {
		t.Fatalf("load canonical record: %v", err)
	}
	if persisted.App != "golfmullet" || persisted.RepoURL != repositoryURL {
		t.Fatalf("persisted identity = app %q, repository %q", persisted.App, persisted.RepoURL)
	}
	if version := deploymentVersion(record); version.App != "golfmullet" || version.RepoURL != repositoryURL {
		t.Fatalf("history identity = app %q, repository %q", version.App, version.RepoURL)
	}

	if record.Network != "minideploy-golfmullet-release-production-regression" {
		t.Fatalf("network = %q", record.Network)
	}
	if hostname := publicHostnameForApp(record.App); hostname != "golfmullet.reactorlab.dev" {
		t.Fatalf("public hostname = %q; want golfmullet.reactorlab.dev", hostname)
	}

	previousDeploymentsDir := deploymentsDir
	deploymentsDir = filepath.Join(t.TempDir(), "managed-deployments")
	t.Cleanup(func() { deploymentsDir = previousDeploymentsDir })
	managedPath, err := managedDeploymentPath(record.App)
	if err != nil || filepath.Base(managedPath) != "golfmullet" {
		t.Fatalf("managed path = %q, error=%v", managedPath, err)
	}
	secretStore := newRuntimeEnvironmentFileStore(filepath.Join(t.TempDir(), "secrets"))
	secretPath, err := secretStore.path(record.App)
	if err != nil || filepath.Base(secretPath) != "golfmullet.env" {
		t.Fatalf("secret path = %q, error=%v", secretPath, err)
	}
	logPath, err := deploymentLogPath(record.App)
	if err != nil || filepath.Base(logPath) != "golfmullet.log" {
		t.Fatalf("deployment log path = %q, error=%v", logPath, err)
	}

	expectedResources := map[string]struct {
		image     string
		container string
	}{
		fullstackFrontendService: {
			image:     "minideploy-golfmullet-frontend:production-regression",
			container: "minideploy-golfmullet-frontend-release-production-regression",
		},
		fullstackBackendService: {
			image:     "minideploy-golfmullet-backend:production-regression",
			container: "minideploy-golfmullet-backend-release-production-regression",
		},
	}
	for serviceName, expected := range expectedResources {
		service, ok := deploymentServiceByName(record, serviceName)
		if !ok {
			t.Fatalf("missing %s service", serviceName)
		}
		if service.Image != expected.image || service.Container != expected.container {
			t.Fatalf("%s resources = image %q, container %q", serviceName, service.Image, service.Container)
		}
		if service.Image != strings.ToLower(service.Image) ||
			service.Container != strings.ToLower(service.Container) {

			t.Fatalf("%s resources are not lowercase", serviceName)
		}
	}

	singleServiceImage := versionedImageName(app)
	if !strings.HasPrefix(singleServiceImage, "minideploy-golfmullet:") ||
		singleServiceImage != strings.ToLower(singleServiceImage) {

		t.Fatalf("single-service image = %q", singleServiceImage)
	}
}

func fullstackTestRecord(app string, release string) DeploymentRecord {
	return normalizeDeploymentRecord(DeploymentRecord{
		App:                  app,
		RepoURL:              "https://github.com/example/" + app + ".git",
		Strategy:             deploymentStrategyFullstackViteNode,
		Network:              "minideploy-" + app + "-release-" + release,
		EnvironmentVariables: []string{"ACCEPTANCE_MESSAGE"},
		Services: []DeploymentServiceRecord{
			{
				Name:               "frontend",
				Path:               "frontend",
				Strategy:           deploymentStrategyViteStatic,
				Container:          "minideploy-" + app + "-frontend-release-" + release,
				Image:              "minideploy-" + app + "-frontend:" + release,
				Port:               8081,
				ContainerPort:      80,
				HealthPath:         "/",
				PackageManager:     packageManagerNPM,
				PackageInstallMode: packageInstallModeCI,
			},
			{
				Name:               "backend",
				Path:               "backend",
				Strategy:           deploymentStrategyNodeExpress,
				Container:          "minideploy-" + app + "-backend-release-" + release,
				Image:              "minideploy-" + app + "-backend:" + release,
				Port:               8082,
				ContainerPort:      3000,
				HealthPath:         "/",
				PackageManager:     packageManagerNPM,
				PackageInstallMode: packageInstallModeCI,
			},
		},
	})
}
