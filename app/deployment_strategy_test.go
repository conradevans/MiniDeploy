package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStrategyTestFile(
	t *testing.T,
	root string,
	name string,
	content string,
) {
	t.Helper()

	path := filepath.Join(root, name)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error: %v", name, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error: %v", name, err)
	}
}

func writeViteManifest(
	t *testing.T,
	root string,
	buildScript string,
) {
	t.Helper()

	writeStrategyTestFile(
		t,
		root,
		"package.json",
		`{
  "scripts": {"build": "`+buildScript+`"},
  "dependencies": {"react": "19.0.0"},
  "devDependencies": {"vite": "8.0.0"}
}`,
	)
}

func writePackageLock(t *testing.T, root string) {
	t.Helper()

	writeStrategyTestFile(
		t,
		root,
		"package-lock.json",
		`{"lockfileVersion":3,"packages":{}}`,
	)
}

func TestDockerfileStrategyTakesPrecedence(t *testing.T) {
	repository := t.TempDir()

	writeStrategyTestFile(
		t,
		repository,
		"Dockerfile",
		"FROM scratch\n",
	)
	writeViteManifest(t, repository, "vite build")
	writePackageLock(t, repository)

	plan, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{
			ContainerPort: 3000,
			HealthPath:    "/health",
		},
	)
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}

	if plan.Strategy != deploymentStrategyDockerfile {
		t.Fatalf(
			"strategy = %q; want %q",
			plan.Strategy,
			deploymentStrategyDockerfile,
		)
	}

	if plan.ContainerPort != 3000 || plan.HealthPath != "/health" {
		t.Fatalf(
			"Dockerfile config = %d %q; want 3000 /health",
			plan.ContainerPort,
			plan.HealthPath,
		)
	}

	if plan.GeneratedDockerfile != "" {
		t.Fatal("Dockerfile strategy unexpectedly generated a Dockerfile")
	}
}

func TestStandardViteStrategyUsesNPMCI(t *testing.T) {
	repository := t.TempDir()

	writeViteManifest(t, repository, "vite build")
	writePackageLock(t, repository)
	writeStrategyTestFile(
		t,
		repository,
		"vite.config.js",
		"export default {}\n",
	)

	plan, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}

	if plan.Strategy != deploymentStrategyViteStatic {
		t.Fatalf(
			"strategy = %q; want %q",
			plan.Strategy,
			deploymentStrategyViteStatic,
		)
	}

	if plan.PackageManager != packageManagerNPM ||
		plan.PackageInstallMode != packageInstallModeCI {
		t.Fatalf(
			"package plan = %q %q; want npm ci",
			plan.PackageManager,
			plan.PackageInstallMode,
		)
	}

	if plan.ContainerPort != 80 {
		t.Fatalf("container port = %d; want 80", plan.ContainerPort)
	}

	if plan.HealthPath != "/" {
		t.Fatalf("health path = %q; want /", plan.HealthPath)
	}
}

func TestViteWithoutLockUsesNPMInstall(t *testing.T) {
	repository := t.TempDir()

	writeViteManifest(t, repository, "vite build")

	plan, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}

	if plan.PackageInstallMode != packageInstallModeSetup {
		t.Fatalf(
			"install mode = %q; want %q",
			plan.PackageInstallMode,
			packageInstallModeSetup,
		)
	}

	if !strings.Contains(
		plan.GeneratedDockerfile,
		"RUN npm install --no-audit --no-fund",
	) {
		t.Fatal("generated Dockerfile does not use npm install")
	}
}

func TestUnrelatedNodeRepositoryIsUnsupported(t *testing.T) {
	repository := t.TempDir()

	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		`{
  "scripts": {"build": "node build.js"},
  "dependencies": {"express": "5.0.0", "vite": "8.0.0"}
}`,
	)

	_, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)

	if !errors.Is(err, ErrNoSupportedDeploymentStrategy) {
		t.Fatalf(
			"error = %v; want ErrNoSupportedDeploymentStrategy",
			err,
		)
	}

	if err.Error() !=
		"No supported deployment strategy detected. Add a Dockerfile or use a supported project type." {
		t.Fatalf("unexpected unsupported error: %q", err.Error())
	}
}

func TestSupportedViteBuildScripts(t *testing.T) {
	for _, test := range []struct {
		buildScript  string
		buildCommand string
	}{
		{
			buildScript:  "vite build",
			buildCommand: "RUN npm run build -- --base=/ --outDir=dist",
		},
		{
			buildScript:  "vite",
			buildCommand: "RUN npm run build -- build --base=/ --outDir=dist",
		},
		{
			buildScript:  "tsc -b && vite build",
			buildCommand: "RUN npm run build -- --base=/ --outDir=dist",
		},
		{
			buildScript:  "tsc -b && vite",
			buildCommand: "RUN npm run build -- build --base=/ --outDir=dist",
		},
	} {
		t.Run(test.buildScript, func(t *testing.T) {
			repository := t.TempDir()
			writeViteManifest(
				t,
				repository,
				test.buildScript,
			)

			plan, err := detectDeploymentStrategy(
				repository,
				deploymentConfig{},
			)
			if err != nil {
				t.Fatalf(
					"detectDeploymentStrategy() error: %v",
					err,
				)
			}

			if plan.Strategy != deploymentStrategyViteStatic {
				t.Fatalf(
					"strategy = %q; want %q",
					plan.Strategy,
					deploymentStrategyViteStatic,
				)
			}

			if !strings.Contains(
				plan.GeneratedDockerfile,
				test.buildCommand,
			) {
				t.Fatalf(
					"generated Dockerfile missing %q:\n%s",
					test.buildCommand,
					plan.GeneratedDockerfile,
				)
			}
		})
	}
}

func TestUnsupportedViteBuildScriptShapes(t *testing.T) {
	for _, buildScript := range []string{
		"node invite-build.js",
		"node private-build.js",
		"echo vite",
		"webpack",
	} {
		t.Run(buildScript, func(t *testing.T) {
			repository := t.TempDir()
			writeViteManifest(t, repository, buildScript)

			_, err := detectDeploymentStrategy(
				repository,
				deploymentConfig{},
			)
			if !errors.Is(
				err,
				ErrNoSupportedDeploymentStrategy,
			) {
				t.Fatalf(
					"error = %v; want ErrNoSupportedDeploymentStrategy",
					err,
				)
			}
		})
	}
}

func TestStaleViteConfigWithWebpackBuildIsUnsupported(
	t *testing.T,
) {
	repository := t.TempDir()

	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		"{\n  \"scripts\": {\"build\": \"webpack\"},\n  \"devDependencies\": {\"webpack\": \"5.0.0\"}\n}",
	)
	writeStrategyTestFile(
		t,
		repository,
		"vite.config.js",
		"export default {}\n",
	)

	_, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)
	if !errors.Is(err, ErrNoSupportedDeploymentStrategy) {
		t.Fatalf(
			"error = %v; want ErrNoSupportedDeploymentStrategy",
			err,
		)
	}
}

func TestViteCommandWithoutViteEvidenceIsUnsupported(
	t *testing.T,
) {
	repository := t.TempDir()

	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		"{\n  \"scripts\": {\"build\": \"vite build\"},\n  \"devDependencies\": {\"react\": \"19.0.0\"}\n}",
	)

	_, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)
	if !errors.Is(err, ErrNoSupportedDeploymentStrategy) {
		t.Fatalf(
			"error = %v; want ErrNoSupportedDeploymentStrategy",
			err,
		)
	}
}

func TestEmptyRepositoryIsUnsupported(t *testing.T) {
	_, err := detectDeploymentStrategy(
		t.TempDir(),
		deploymentConfig{},
	)

	if !errors.Is(err, ErrNoSupportedDeploymentStrategy) {
		t.Fatalf(
			"error = %v; want ErrNoSupportedDeploymentStrategy",
			err,
		)
	}
}

func TestGeneratedViteRuntimeSupportsRootHostingAndSPAFallback(
	t *testing.T,
) {
	dockerfile := generatedViteDockerfile(
		packageInstallModeCI,
		false,
	)

	for _, required := range []string{
		"FROM node:24-alpine AS build",
		"RUN npm ci --no-audit --no-fund",
		"RUN npm run build -- --base=/ --outDir=dist",
		"FROM nginx:stable-alpine AS runtime",
		"COPY --from=build /app/dist/ /usr/share/nginx/html/",
		"try_files $uri $uri/ /index.html;",
		"EXPOSE 80",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf(
				"generated Dockerfile missing %q:\n%s",
				required,
				dockerfile,
			)
		}
	}

	for _, forbidden := range []string{
		"/var/run/docker.sock",
		"/srv/minideploy",
		"MINIDEPLOY_",
	} {
		if strings.Contains(dockerfile, forbidden) {
			t.Fatalf(
				"generated Dockerfile exposes forbidden value %q",
				forbidden,
			)
		}
	}
}

func TestPersistedViteStrategyDoesNotChangeInstallMode(
	t *testing.T,
) {
	repository := t.TempDir()

	writeViteManifest(t, repository, "vite build")
	writePackageLock(t, repository)

	plan, err := deploymentStrategyForRecord(
		DeploymentRecord{
			Strategy:           deploymentStrategyViteStatic,
			PackageManager:     packageManagerNPM,
			PackageInstallMode: packageInstallModeSetup,
			ContainerPort:      80,
			HealthPath:         "/",
		},
		repository,
	)
	if err != nil {
		t.Fatalf("deploymentStrategyForRecord() error: %v", err)
	}

	if plan.PackageInstallMode != packageInstallModeSetup {
		t.Fatalf(
			"install mode = %q; want persisted %q",
			plan.PackageInstallMode,
			packageInstallModeSetup,
		)
	}
}

func TestLegacyRecordUsesDockerfileStrategy(t *testing.T) {
	repository := t.TempDir()

	writeStrategyTestFile(
		t,
		repository,
		"Dockerfile",
		"FROM scratch\n",
	)

	record := normalizeDeploymentRecord(
		DeploymentRecord{
			ContainerPort: 3000,
			HealthPath:    "/health",
		},
	)

	if record.Strategy != deploymentStrategyDockerfile {
		t.Fatalf(
			"legacy strategy = %q; want %q",
			record.Strategy,
			deploymentStrategyDockerfile,
		)
	}

	plan, err := deploymentStrategyForRecord(
		record,
		repository,
	)
	if err != nil {
		t.Fatalf("deploymentStrategyForRecord() error: %v", err)
	}

	if plan.Strategy != deploymentStrategyDockerfile ||
		plan.ContainerPort != 3000 ||
		plan.HealthPath != "/health" {
		t.Fatalf("unexpected legacy plan: %#v", plan)
	}
}

func TestStrategyPersistsInDeploymentAndHistory(t *testing.T) {
	record := DeploymentRecord{
		App:                "vite-app",
		RepoURL:            "https://github.com/example/vite-app.git",
		Container:          "minideploy-vite-app",
		Image:              "minideploy-vite-app:v1",
		Port:               8081,
		ContainerPort:      80,
		HealthPath:         "/",
		Strategy:           deploymentStrategyViteStatic,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: packageInstallModeCI,
	}

	storePath := filepath.Join(t.TempDir(), "deployments.json")
	strategyStore := NewJSONStore(storePath)

	if err := strategyStore.Save(record); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	stored, err := strategyStore.Get(record.App)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if stored != record {
		t.Fatalf("stored record = %#v; want %#v", stored, record)
	}

	version := deploymentVersion(record)
	restored := version.Record()

	if restored.Strategy != deploymentStrategyViteStatic ||
		restored.PackageManager != packageManagerNPM ||
		restored.PackageInstallMode != packageInstallModeCI {
		t.Fatalf("history strategy was not preserved: %#v", restored)
	}
}

func TestWebhookSelectionPreservesDeploymentStrategy(t *testing.T) {
	want := DeploymentRecord{
		App:                "vite-app",
		RepoURL:            "https://github.com/example/vite-app.git",
		Strategy:           deploymentStrategyViteStatic,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: packageInstallModeCI,
		ContainerPort:      80,
		HealthPath:         "/",
	}

	got, found := deploymentForWebhook(
		[]DeploymentRecord{want},
		"https://github.com/EXAMPLE/vite-app",
	)

	if !found {
		t.Fatal("webhook deployment was not found")
	}

	if got.Strategy != want.Strategy ||
		got.PackageManager != want.PackageManager ||
		got.PackageInstallMode != want.PackageInstallMode {
		t.Fatalf("webhook strategy = %#v; want %#v", got, want)
	}
}
