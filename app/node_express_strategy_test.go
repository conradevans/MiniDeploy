package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeExpressManifest(
	t *testing.T,
	repository string,
	startScript string,
	expressSection string,
) {
	t.Helper()

	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		`{
  "scripts": {"start": "`+startScript+`"},
  `+expressSection+`
}`,
	)

	if startScript == "node server.js" {
		writeStrategyTestFile(
			t,
			repository,
			"server.js",
			"require('express')()\n",
		)
	}
}

func writeExpressStartFixture(
	t *testing.T,
	repository string,
	startScript string,
) {
	t.Helper()

	manifest := map[string]any{
		"scripts": map[string]string{
			"start": startScript,
		},
		"dependencies": map[string]string{
			"express": "5.1.0",
		},
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal(package.json) error: %v", err)
	}

	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		string(data),
	)
}

func requireUnsupportedDeployment(t *testing.T, repository string) {
	t.Helper()

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

func TestNodeExpressAcceptsSupportedDirectNodeStartScripts(
	t *testing.T,
) {
	for _, test := range []struct {
		startScript string
		entrypoint  string
	}{
		{startScript: "node server.js", entrypoint: "server.js"},
		{startScript: "node ./server.js", entrypoint: "server.js"},
		{startScript: "node src/server.js", entrypoint: "src/server.js"},
		{startScript: "node app.cjs", entrypoint: "app.cjs"},
		{startScript: "node src/app.mjs", entrypoint: "src/app.mjs"},
	} {
		t.Run(test.startScript, func(t *testing.T) {
			repository := t.TempDir()
			writeExpressStartFixture(t, repository, test.startScript)
			writeStrategyTestFile(
				t,
				repository,
				test.entrypoint,
				"// application entrypoint\n",
			)

			plan, err := detectDeploymentStrategy(
				repository,
				deploymentConfig{},
			)
			if err != nil {
				t.Fatalf("detectDeploymentStrategy() error: %v", err)
			}

			if plan.Strategy != deploymentStrategyNodeExpress {
				t.Fatalf(
					"strategy = %q; want %q",
					plan.Strategy,
					deploymentStrategyNodeExpress,
				)
			}
		})
	}
}

func TestNodeExpressRejectsUnsupportedStartScripts(t *testing.T) {
	for _, startScript := range []string{
		"react-scripts start",
		"next start",
		"nodemon server.js",
		"npm run server",
		"node ../server.js",
		"node /absolute/server.js",
		"node server.ts",
		"node server.js --flag",
		"NODE_ENV=production node server.js",
		"node server.js && echo done",
		"echo node server.js",
		"node server.js | tee output",
		"node server.js > output",
		"node $(printf server.js)",
		"node\nserver.js",
		"node server.js;echo",
		`node "server.js"`,
		"yarn start",
		"pnpm start",
		"ts-node server.js",
		"tsx server.js",
	} {
		t.Run(startScript, func(t *testing.T) {
			parent := t.TempDir()
			repository := filepath.Join(parent, "repository")
			if err := os.Mkdir(repository, 0755); err != nil {
				t.Fatalf("Mkdir(repository) error: %v", err)
			}

			writeExpressStartFixture(t, repository, startScript)
			writeStrategyTestFile(
				t,
				repository,
				"server.js",
				"// local entrypoint\n",
			)
			writeStrategyTestFile(
				t,
				parent,
				"server.js",
				"// escaped entrypoint\n",
			)

			requireUnsupportedDeployment(t, repository)
		})
	}
}

func TestNodeExpressEntrypointMustBeContainedRegularFile(
	t *testing.T,
) {
	t.Run("missing entrypoint", func(t *testing.T) {
		repository := t.TempDir()
		writeExpressStartFixture(t, repository, "node server.js")

		requireUnsupportedDeployment(t, repository)
	})

	t.Run("directory entrypoint", func(t *testing.T) {
		repository := t.TempDir()
		writeExpressStartFixture(t, repository, "node server.js")
		if err := os.Mkdir(
			filepath.Join(repository, "server.js"),
			0755,
		); err != nil {
			t.Fatalf("Mkdir(server.js) error: %v", err)
		}

		requireUnsupportedDeployment(t, repository)
	})

	t.Run("symlink escapes repository", func(t *testing.T) {
		parent := t.TempDir()
		repository := filepath.Join(parent, "repository")
		if err := os.Mkdir(repository, 0755); err != nil {
			t.Fatalf("Mkdir(repository) error: %v", err)
		}

		outside := filepath.Join(parent, "outside.js")
		writeStrategyTestFile(
			t,
			parent,
			"outside.js",
			"// outside repository\n",
		)
		writeExpressStartFixture(t, repository, "node server.js")
		if err := os.Symlink(
			outside,
			filepath.Join(repository, "server.js"),
		); err != nil {
			t.Fatalf("Symlink(server.js) error: %v", err)
		}

		requireUnsupportedDeployment(t, repository)
	})

	t.Run("absolute existing entrypoint", func(t *testing.T) {
		parent := t.TempDir()
		repository := filepath.Join(parent, "repository")
		if err := os.Mkdir(repository, 0755); err != nil {
			t.Fatalf("Mkdir(repository) error: %v", err)
		}

		outside := filepath.Join(parent, "outside.js")
		writeStrategyTestFile(
			t,
			parent,
			"outside.js",
			"// outside repository\n",
		)
		writeExpressStartFixture(
			t,
			repository,
			"node "+outside,
		)

		requireUnsupportedDeployment(t, repository)
	})
}

func TestGolfMulletFrontendLikeFixtureIsNotNodeExpress(
	t *testing.T,
) {
	repository := t.TempDir()
	writeExpressStartFixture(t, repository, "react-scripts start")
	writeStrategyTestFile(
		t,
		repository,
		"src/index.js",
		"// React application\n",
	)

	requireUnsupportedDeployment(t, repository)
}

func TestConventionalExpressSelectsNodeExpress(t *testing.T) {
	repository := t.TempDir()

	writeExpressManifest(
		t,
		repository,
		"node server.js",
		`"dependencies": {"express": "5.1.0"}`,
	)
	writePackageLock(t, repository)

	plan, err := detectDeploymentStrategy(
		repository,
		deploymentConfig{},
	)
	if err != nil {
		t.Fatalf("detectDeploymentStrategy() error: %v", err)
	}

	if plan.Strategy != deploymentStrategyNodeExpress {
		t.Fatalf(
			"strategy = %q; want %q",
			plan.Strategy,
			deploymentStrategyNodeExpress,
		)
	}

	if plan.PackageManager != packageManagerNPM ||
		plan.PackageInstallMode != packageInstallModeCI {

		t.Fatalf("unexpected npm plan: %#v", plan)
	}

	if plan.ContainerPort != defaultNodeContainerPort {
		t.Fatalf(
			"container port = %d; want %d",
			plan.ContainerPort,
			defaultNodeContainerPort,
		)
	}

	if plan.HealthPath != "/" {
		t.Fatalf("health path = %q; want /", plan.HealthPath)
	}

	for _, required := range []string{
		"FROM node:24-alpine",
		"RUN npm ci --no-audit --no-fund",
		`CMD ["npm", "start"]`,
		"EXPOSE 3000",
	} {
		if !strings.Contains(plan.GeneratedDockerfile, required) {
			t.Fatalf(
				"generated Dockerfile missing %q:\n%s",
				required,
				plan.GeneratedDockerfile,
			)
		}
	}

	if strings.Contains(
		plan.GeneratedDockerfile,
		"node server.js",
	) {
		t.Fatal("repository start script leaked into generated Dockerfile")
	}
}

func TestNodeExpressRequiresRuntimeExpressAndStart(
	t *testing.T,
) {
	tests := []struct {
		name     string
		manifest string
	}{
		{
			name: "missing start",
			manifest: `{
  "dependencies": {"express": "5.1.0"}
}`,
		},
		{
			name: "empty start",
			manifest: `{
  "scripts": {"start": "  "},
  "dependencies": {"express": "5.1.0"}
}`,
		},
		{
			name: "missing express",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"dotenv": "17.0.0"}
}`,
		},
		{
			name: "express only in dev dependencies",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "devDependencies": {"express": "5.1.0"}
}`,
		},
		{
			name: "empty express version",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": " "}
}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			writeStrategyTestFile(
				t,
				repository,
				"package.json",
				test.manifest,
			)

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

func TestNodeExpressRejectsUnrelatedAndUnsupportedNodeProjects(
	t *testing.T,
) {
	tests := []struct {
		name       string
		manifest   string
		extraFiles map[string]string
	}{
		{
			name: "unrelated Node package",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"dotenv": "17.0.0"}
}`,
		},
		{
			name: "Next with Express",
			manifest: `{
  "scripts": {"start": "next start"},
  "dependencies": {"express": "5.1.0", "next": "16.0.0"}
}`,
		},
		{
			name: "Nest with Express",
			manifest: `{
  "scripts": {"start": "node dist/main.js"},
  "dependencies": {"express": "5.1.0", "@nestjs/core": "11.0.0"}
}`,
		},
		{
			name: "Fastify with Express",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0", "fastify": "5.0.0"}
}`,
		},
		{
			name: "Koa with Express",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0", "koa": "3.0.0"}
}`,
		},
		{
			name: "TypeScript dependency",
			manifest: `{
  "scripts": {"start": "node dist/server.js"},
  "dependencies": {"express": "5.1.0"},
  "devDependencies": {"typescript": "5.9.0"}
}`,
		},
		{
			name: "TypeScript start entry",
			manifest: `{
  "scripts": {"start": "node --experimental-strip-types server.ts"},
  "dependencies": {"express": "5.1.0"}
}`,
		},
		{
			name: "TypeScript configuration",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
			extraFiles: map[string]string{
				"tsconfig.json": `{}`,
			},
		},
		{
			name: "Deno start command",
			manifest: `{
  "scripts": {"start": "deno run server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
		},
		{
			name: "Deno configuration",
			manifest: `{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
			extraFiles: map[string]string{
				"deno.json": `{}`,
			},
		},
		{
			name: "npm workspace",
			manifest: `{
  "workspaces": ["packages/*"],
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			writeStrategyTestFile(
				t,
				repository,
				"package.json",
				test.manifest,
			)

			for name, content := range test.extraFiles {
				writeStrategyTestFile(
					t,
					repository,
					name,
					content,
				)
			}

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

func TestNodeExpressRejectsNonNPMRepositories(t *testing.T) {
	for _, lockFile := range []string{
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lock",
		"bun.lockb",
	} {
		t.Run(lockFile, func(t *testing.T) {
			repository := t.TempDir()
			writeExpressManifest(
				t,
				repository,
				"node server.js",
				`"dependencies": {"express": "5.1.0"}`,
			)
			writeStrategyTestFile(
				t,
				repository,
				lockFile,
				"",
			)

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

func TestNodeExpressRejectsNonNPMPackageManager(t *testing.T) {
	for _, packageManager := range []string{
		"yarn@4.0.0",
		"pnpm@10.0.0",
		"bun@1.2.0",
	} {
		t.Run(packageManager, func(t *testing.T) {
			repository := t.TempDir()
			writeStrategyTestFile(
				t,
				repository,
				"package.json",
				`{
  "packageManager": "`+packageManager+`",
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
			)

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

func TestNodeExpressWithoutLockUsesNPMInstall(t *testing.T) {
	repository := t.TempDir()
	writeExpressManifest(
		t,
		repository,
		"node server.js",
		`"dependencies": {"express": "5.1.0"}`,
	)

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

func TestNodeExpressAdvancedPortAndHealthOverrides(
	t *testing.T,
) {
	repository := t.TempDir()
	writeExpressManifest(
		t,
		repository,
		"node server.js",
		`"dependencies": {"express": "5.1.0"}`,
	)

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

	if plan.ContainerPort != 4321 ||
		plan.HealthPath != "/health" {

		t.Fatalf("unexpected advanced config: %#v", plan)
	}

	if !strings.Contains(
		plan.GeneratedDockerfile,
		"EXPOSE 4321",
	) {
		t.Fatal("generated Dockerfile missing resolved port")
	}
}

func TestNodeExpressStrategyPersistsForLifecycleReuse(t *testing.T) {
	record := DeploymentRecord{
		App:                "express-app",
		RepoURL:            "https://github.com/example/express-app.git",
		Container:          "minideploy-express-app",
		Image:              "minideploy-express-app:v1",
		Port:               8081,
		ContainerPort:      3000,
		HealthPath:         "/health",
		Strategy:           deploymentStrategyNodeExpress,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: packageInstallModeCI,
		EnvironmentVariables: []string{
			"JWT_SECRET",
			"MONGODB_URI",
		},
	}

	metadataStore := NewJSONStore(
		filepath.Join(t.TempDir(), "deployments.json"),
	)
	if err := metadataStore.Save(record); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	reloaded, err := metadataStore.Get(record.App)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if !reflect.DeepEqual(reloaded, record) {
		t.Fatalf("reloaded record = %#v; want %#v", reloaded, record)
	}

	version := deploymentVersion(record)
	restored := version.RecordWithFallback(record)
	if restored.Strategy != deploymentStrategyNodeExpress ||
		restored.PackageManager != packageManagerNPM ||
		restored.PackageInstallMode != packageInstallModeCI ||
		restored.ContainerPort != 3000 ||
		restored.HealthPath != "/health" {

		t.Fatalf("historical Node strategy was not preserved: %#v", restored)
	}

	if len(restored.EnvironmentVariables) != 0 {
		t.Fatalf(
			"history unexpectedly restored secret metadata: %v",
			restored.EnvironmentVariables,
		)
	}

	repository := t.TempDir()
	writeExpressManifest(
		t,
		repository,
		"node server.js",
		`"dependencies": {"express": "5.1.0"}`,
	)
	writePackageLock(t, repository)

	plan, err := deploymentStrategyForRecord(reloaded, repository)
	if err != nil {
		t.Fatalf("deploymentStrategyForRecord() error: %v", err)
	}

	if plan.Strategy != deploymentStrategyNodeExpress ||
		plan.PackageInstallMode != packageInstallModeCI ||
		plan.ContainerPort != 3000 ||
		plan.HealthPath != "/health" {

		t.Fatalf("unexpected persisted Node plan: %#v", plan)
	}
}

func TestPersistedNodeExpressRejectsUnsupportedFrameworkChange(
	t *testing.T,
) {
	repository := t.TempDir()
	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		`{
  "scripts": {"start": "next start"},
  "dependencies": {"express": "5.1.0", "next": "16.0.0"}
}`,
	)

	_, err := deploymentStrategyForRecord(
		DeploymentRecord{
			Strategy:           deploymentStrategyNodeExpress,
			PackageManager:     packageManagerNPM,
			PackageInstallMode: packageInstallModeSetup,
			ContainerPort:      3000,
			HealthPath:         "/",
		},
		repository,
	)
	if err == nil {
		t.Fatal("persisted Node strategy accepted a Next.js repository")
	}
}

func TestPersistedNodeExpressRejectsUnsupportedStartScript(
	t *testing.T,
) {
	repository := t.TempDir()
	writeExpressStartFixture(t, repository, "react-scripts start")

	_, err := deploymentStrategyForRecord(
		DeploymentRecord{
			Strategy:           deploymentStrategyNodeExpress,
			PackageManager:     packageManagerNPM,
			PackageInstallMode: packageInstallModeSetup,
			ContainerPort:      3000,
			HealthPath:         "/",
		},
		repository,
	)
	if err == nil {
		t.Fatal("persisted Node strategy accepted an unsupported start script")
	}
}

func TestViteTakesPrecedenceOverExpress(t *testing.T) {
	repository := t.TempDir()
	writeStrategyTestFile(
		t,
		repository,
		"package.json",
		`{
  "scripts": {
    "build": "vite build",
    "start": "node server.js"
  },
  "dependencies": {"express": "5.1.0"},
  "devDependencies": {"vite": "8.0.0"}
}`,
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
}

func TestViteEvidenceNeverFallsThroughToNodeExpress(t *testing.T) {
	for _, test := range []struct {
		name       string
		manifest   string
		extraFiles map[string]string
	}{
		{
			name: "Vite dependency with unsupported build script",
			manifest: `{
  "scripts": {"build": "webpack", "start": "node server.js"},
  "dependencies": {"express": "5.1.0"},
  "devDependencies": {"vite": "8.0.0"}
}`,
		},
		{
			name: "Vite config with unsupported build script",
			manifest: `{
  "scripts": {"build": "webpack", "start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
			extraFiles: map[string]string{
				"vite.config.js": "export default {}\n",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := t.TempDir()
			writeStrategyTestFile(
				t,
				repository,
				"package.json",
				test.manifest,
			)
			writeStrategyTestFile(
				t,
				repository,
				"server.js",
				"// Node entrypoint\n",
			)

			for name, content := range test.extraFiles {
				writeStrategyTestFile(
					t,
					repository,
					name,
					content,
				)
			}

			requireUnsupportedDeployment(t, repository)
		})
	}
}

func TestDockerfileTakesPrecedenceOverNodeMetadata(
	t *testing.T,
) {
	for _, manifest := range []string{
		`{
  "scripts": {"start": "node server.js"},
  "dependencies": {"express": "5.1.0"}
}`,
		`{
  "scripts": {"build": "vite build", "start": "node server.js"},
  "dependencies": {"express": "5.1.0"},
  "devDependencies": {"vite": "8.0.0"}
}`,
	} {
		repository := t.TempDir()
		writeStrategyTestFile(
			t,
			repository,
			"Dockerfile",
			"FROM scratch\n",
		)
		writeStrategyTestFile(
			t,
			repository,
			"package.json",
			manifest,
		)

		plan, err := detectDeploymentStrategy(
			repository,
			deploymentConfig{
				ContainerPort: 8080,
				HealthPath:    "/ready",
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

		if plan.ContainerPort != 8080 ||
			plan.HealthPath != "/ready" {

			t.Fatalf(
				"Dockerfile config changed: %#v",
				plan,
			)
		}
	}
}
