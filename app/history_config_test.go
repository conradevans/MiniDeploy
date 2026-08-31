package main

import "testing"

func TestDeploymentVersionPreservesConfig(t *testing.T) {
	record := DeploymentRecord{
		App:           "example",
		RepoURL:       "https://github.com/example/app.git",
		Container:     "example-container",
		Image:         "example-image:v1",
		Port:          8081,
		ContainerPort: 3000,
		HealthPath:    "/health",
	}

	version := deploymentVersion(record)

	if version.ContainerPort != 3000 {
		t.Fatalf(
			"expected historical container port 3000, got %d",
			version.ContainerPort,
		)
	}

	if version.HealthPath != "/health" {
		t.Fatalf(
			"expected historical health path /health, got %q",
			version.HealthPath,
		)
	}

	restored := version.Record()

	if restored.ContainerPort != 3000 {
		t.Fatalf(
			"expected restored container port 3000, got %d",
			restored.ContainerPort,
		)
	}

	if restored.HealthPath != "/health" {
		t.Fatalf(
			"expected restored health path /health, got %q",
			restored.HealthPath,
		)
	}
}

func TestLegacyDeploymentVersionUsesFallbackConfig(t *testing.T) {
	legacy := DeploymentVersion{
		App:     "legacy",
		RepoURL: "https://github.com/example/legacy.git",
		Image:   "legacy-image:v1",
		Port:    8081,
	}

	current := DeploymentRecord{
		App:                "legacy",
		ContainerPort:      3000,
		HealthPath:         "/health",
		Strategy:           deploymentStrategyViteStatic,
		PackageManager:     packageManagerNPM,
		PackageInstallMode: packageInstallModeCI,
	}

	restored := legacy.RecordWithFallback(current)

	if restored.ContainerPort != 3000 {
		t.Fatalf(
			"expected legacy fallback container port 3000, got %d",
			restored.ContainerPort,
		)
	}

	if restored.HealthPath != "/health" {
		t.Fatalf(
			"expected legacy fallback health path /health, got %q",
			restored.HealthPath,
		)
	}

	if restored.Strategy != deploymentStrategyViteStatic ||
		restored.PackageManager != packageManagerNPM ||
		restored.PackageInstallMode != packageInstallModeCI {
		t.Fatalf(
			"expected legacy fallback strategy fields, got %#v",
			restored,
		)
	}
}

func TestHistoricalConfigOverridesFallback(t *testing.T) {
	version := DeploymentVersion{
		App:           "example",
		ContainerPort: 8080,
		HealthPath:    "/ready",
	}

	current := DeploymentRecord{
		App:           "example",
		ContainerPort: 3000,
		HealthPath:    "/health",
	}

	restored := version.RecordWithFallback(current)

	if restored.ContainerPort != 8080 {
		t.Fatalf(
			"expected historical container port 8080, got %d",
			restored.ContainerPort,
		)
	}

	if restored.HealthPath != "/ready" {
		t.Fatalf(
			"expected historical health path /ready, got %q",
			restored.HealthPath,
		)
	}
}
