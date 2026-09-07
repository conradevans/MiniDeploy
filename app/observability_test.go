package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestReactorLabObservabilityOnlyInspectsDeploymentContainers(t *testing.T) {
	previousStore := store
	previousRunner := commandRunner
	t.Cleanup(func() {
		store = previousStore
		commandRunner = previousRunner
	})

	testStore := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := testStore.Save(DeploymentRecord{
		App:        "safe-app",
		Container:  "minideploy-safe-app",
		Strategy:   "dockerfile",
		Port:       8081,
		HealthPath: "/",
	}); err != nil {
		t.Fatal(err)
	}
	store = testStore

	commandRunner = func(_ string, name string, args ...string) (string, error) {
		if name != "docker" {
			t.Fatalf("unexpected command: %s", name)
		}

		for _, arg := range args {
			if arg == "minibase-postgres" {
				t.Fatalf("non-deployment container passed to Docker detail command")
			}
		}

		if len(args) >= 2 && args[0] == "ps" && args[1] == "-a" {
			return "minideploy-safe-app\nminibase-postgres\n", nil
		}

		if len(args) > 0 && args[0] == "inspect" {
			return "{\"Status\":\"exited\",\"StartedAt\":\"2026-09-07T00:00:00Z\"}\t/minideploy-safe-app\t0\t1024\n", nil
		}

		if len(args) > 0 && args[0] == "stats" {
			t.Fatalf("stats should not run for an exited container")
		}

		t.Fatalf("unexpected Docker args: %s", strings.Join(args, " "))
		return "", nil
	}

	got, err := collectReactorLabObservability()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Deployments) != 1 {
		t.Fatalf("deployments = %d, want 1", len(got.Deployments))
	}
	if len(got.Deployments[0].Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(got.Deployments[0].Containers))
	}
	if got.Deployments[0].Containers[0].Container != "minideploy-safe-app" {
		t.Fatalf("unexpected container: %q", got.Deployments[0].Containers[0].Container)
	}
}

func TestReactorLabTargetsUseRecordedServices(t *testing.T) {
	record := DeploymentRecord{
		App:      "fullstack",
		Strategy: "fullstack-vite-node",
		Services: []DeploymentServiceRecord{
			{
				Name:       "frontend",
				Strategy:   "vite-static",
				Container:  "minideploy-fullstack-frontend",
				Port:       8088,
				HealthPath: "/",
			},
			{
				Name:       "backend",
				Strategy:   "node-express",
				Container:  "minideploy-fullstack-backend",
				Port:       8087,
				HealthPath: "/health",
			},
		},
	}

	targets := reactorLabTargets(record)
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	if targets[0].Container != "minideploy-fullstack-frontend" ||
		targets[1].Container != "minideploy-fullstack-backend" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestReactorLabStrategyNormalizesLegacyDeployment(t *testing.T) {
	if got := reactorLabStrategy(""); got != deploymentStrategyDockerfile {
		t.Fatalf("legacy strategy = %q, want %q", got, deploymentStrategyDockerfile)
	}

	if got := reactorLabStrategy(deploymentStrategyNodeExpress); got != deploymentStrategyNodeExpress {
		t.Fatalf("node strategy = %q, want %q", got, deploymentStrategyNodeExpress)
	}
}
