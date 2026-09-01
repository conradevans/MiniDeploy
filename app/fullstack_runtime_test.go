package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func replaceCommandRunnerForTest(
	t *testing.T,
	runner func(string, string, ...string) (string, error),
) {
	t.Helper()
	previous := commandRunner
	commandRunner = runner
	t.Cleanup(func() { commandRunner = previous })
}

func TestFullstackProxyRoutesUseOneHostnameAndAPIPrefix(t *testing.T) {
	record := fullstackTestRecord("project", "route")
	local, public, err := fullstackProxyRouteFragments(
		record,
		7,
		"project.reactorlab.dev",
	)
	if err != nil {
		t.Fatalf("fullstackProxyRouteFragments() error: %v", err)
	}

	for _, want := range []string{
		"handle_path /project/*",
		"path /api /api/*",
		"reverse_proxy 127.0.0.1:8082",
		"reverse_proxy 127.0.0.1:8081",
	} {
		if !strings.Contains(local+public, want) {
			t.Fatalf("routing is missing %q:\nlocal=%s\npublic=%s", want, local, public)
		}
	}
	if strings.Index(public, "127.0.0.1:8082") >
		strings.Index(public, "127.0.0.1:8081") {

		t.Fatal("backend /api route must precede frontend catch-all")
	}
	if strings.Contains(public, "backend.reactorlab.dev") ||
		strings.Count(public, "project.reactorlab.dev") != 2 {

		t.Fatalf("unexpected independent backend hostname: %s", public)
	}
}

func TestFullstackEnvironmentOnlyReachesBackend(t *testing.T) {
	secret := "PHASE3_SECRET_SENTINEL"
	environment := map[string]string{"ACCEPTANCE_MESSAGE": secret}
	frontend := fullstackServiceRuntimeEnvironment(
		fullstackFrontendService,
		environment,
	)
	backend := fullstackServiceRuntimeEnvironment(
		fullstackBackendService,
		environment,
	)
	if len(frontend) != 0 {
		t.Fatalf("frontend received environment: %#v", frontend)
	}
	if backend["ACCEPTANCE_MESSAGE"] != secret {
		t.Fatal("backend did not receive runtime environment")
	}
	backend["ACCEPTANCE_MESSAGE"] = "changed"
	if environment["ACCEPTANCE_MESSAGE"] != secret {
		t.Fatal("backend environment was not cloned")
	}

	dockerEnvironment := dockerRuntimeEnvironment(
		deploymentStrategyNodeExpress,
		3000,
		fullstackServiceRuntimeEnvironment(
			fullstackBackendService,
			environment,
		),
	)
	if dockerEnvironment["PORT"] != "3000" {
		t.Fatalf("managed PORT = %q; want 3000", dockerEnvironment["PORT"])
	}
	args := managedContainerRunArgumentsWithOptions(
		"minideploy-project-backend-release-test",
		"minideploy-project-backend:test",
		8082,
		3000,
		"/secure/runtime.env",
		managedContainerOptions{
			Network: "minideploy-project-release-test",
			Service: fullstackBackendService,
			App:     "project",
		},
	)
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"127.0.0.1:8082:3000",
		"--network minideploy-project-release-test",
		"--network-alias backend",
		"com.minideploy.app=project",
		"com.minideploy.service=backend",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Docker args missing %q: %v", want, args)
		}
	}
	if strings.Contains(joined, secret) || strings.Contains(joined, "/var/run/docker.sock") {
		t.Fatalf("unsafe Docker args: %v", args)
	}
}

func TestFullstackAdminNamesOnlyAndGuestStillThreeFields(t *testing.T) {
	secret := "PHASE3_RESPONSE_SECRET_SENTINEL"
	record := fullstackTestRecord("project", "response")
	admin := deploymentResponse(record)
	adminData, err := json.Marshal(admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(admin.Services) != 2 ||
		!reflect.DeepEqual(admin.EnvironmentVariables, []string{"ACCEPTANCE_MESSAGE"}) {

		t.Fatalf("unexpected Admin response: %#v", admin)
	}
	if strings.Contains(string(adminData), secret) {
		t.Fatal("secret value entered Admin response")
	}

	guest, err := guestDeploymentResponse(record, "running")
	if err != nil {
		t.Fatal(err)
	}
	guestData, err := json.Marshal(guest)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(guestData, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 {
		t.Fatalf("Guest fields = %v; want exactly three", fields)
	}
	for _, forbidden := range []string{
		"services",
		"strategy",
		"frontend",
		"backend",
		"network",
		"environment",
		"8081",
		"8082",
	} {
		if strings.Contains(string(guestData), forbidden) {
			t.Fatalf("Guest response exposed %q: %s", forbidden, guestData)
		}
	}
}

func TestFullstackRestartHandlesBothServicesPredictably(t *testing.T) {
	record := fullstackTestRecord("project", "restart")
	var restarted []string
	replaceCommandRunnerForTest(
		t,
		func(_ string, name string, args ...string) (string, error) {
			if name != "docker" {
				return "", nil
			}
			if len(args) >= 4 && args[0] == "inspect" && args[1] == "-f" {
				service := fullstackFrontendService
				if strings.Contains(args[3], "backend") {
					service = fullstackBackendService
				}
				return "true|project|" + service + "\n", nil
			}
			if len(args) > 0 && args[0] == "restart" {
				restarted = append(restarted, args[1])
			}
			return "", nil
		},
	)

	containers, _, err := restartDeploymentContainers(record)
	if err != nil {
		t.Fatalf("restartDeploymentContainers() error: %v", err)
	}
	backend, _ := deploymentServiceByName(record, fullstackBackendService)
	frontend, _ := deploymentServiceByName(record, fullstackFrontendService)
	want := []string{backend.Container, frontend.Container}
	if !slices.Equal(restarted, want) {
		t.Fatalf("restart order = %v; want %v", restarted, want)
	}
	if containers != strings.Join(want, ", ") {
		t.Fatalf("container summary = %q", containers)
	}
}

func TestFullstackCleanupRefusesAnotherProjectsResources(t *testing.T) {
	record := fullstackTestRecord("project", "ownership")
	var removed bool
	replaceCommandRunnerForTest(
		t,
		func(_ string, _ string, args ...string) (string, error) {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "network inspect") {
				return "true|another-project\n", nil
			}
			if strings.Contains(joined, "inspect -f") {
				return "true|another-project|frontend\n", nil
			}
			if strings.Contains(joined, " rm ") ||
				(len(args) > 1 && args[0] == "rm") {
				removed = true
			}
			return "", nil
		},
	)

	frontend, _ := deploymentServiceByName(record, fullstackFrontendService)
	if err := removeFullstackContainer(
		record.App,
		frontend.Name,
		frontend.Container,
	); err == nil {
		t.Fatal("expected foreign container ownership error")
	}
	if err := removeFullstackNetwork(record.App, record.Network); err == nil {
		t.Fatal("expected foreign network ownership error")
	}
	if removed {
		t.Fatal("cleanup attempted to remove another project's resource")
	}
}

func TestFullstackRestartRefusesAnotherProjectsContainer(t *testing.T) {
	record := fullstackTestRecord("project", "restart-ownership")
	var restarted bool
	replaceCommandRunnerForTest(
		t,
		func(_ string, _ string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "restart" {
				restarted = true
			}
			return "true|another-project|backend\n", nil
		},
	)

	if _, _, err := restartDeploymentContainers(record); err == nil {
		t.Fatal("expected foreign container ownership error")
	}
	if restarted {
		t.Fatal("restart attempted to mutate another project's container")
	}
}

func TestFullstackCleanupDoesNotIgnoreDockerInspectionFailure(t *testing.T) {
	record := fullstackTestRecord("project", "inspect-failure")
	inspectErr := errors.New("cannot connect to Docker daemon")
	replaceCommandRunnerForTest(
		t,
		func(_ string, _ string, _ ...string) (string, error) {
			return "daemon unavailable", inspectErr
		},
	)

	frontend, _ := deploymentServiceByName(record, fullstackFrontendService)
	if err := removeFullstackContainer(
		record.App,
		frontend.Name,
		frontend.Container,
	); !errors.Is(err, inspectErr) {
		t.Fatalf("container cleanup error = %v; want inspection failure", err)
	}
	if err := removeFullstackNetwork(
		record.App,
		record.Network,
	); !errors.Is(err, inspectErr) {
		t.Fatalf("network cleanup error = %v; want inspection failure", err)
	}
}

func TestFullstackAggregateStatusRequiresBothServicesRunning(t *testing.T) {
	record := fullstackTestRecord("project", "status")
	replaceCommandRunnerForTest(
		t,
		func(_ string, _ string, args ...string) (string, error) {
			container := args[len(args)-1]
			if strings.Contains(container, "backend") {
				return "exited\n", nil
			}
			return "running\n", nil
		},
	)
	if status := deploymentProjectStatus(record); status != "exited" {
		t.Fatalf("aggregate status = %q; want exited", status)
	}
}

func TestFullstackResourceNameValidation(t *testing.T) {
	if _, err := fullstackServiceContainerName(
		"project",
		"../backend",
		"release",
		"one",
	); err == nil {
		t.Fatal("accepted traversal-like service name")
	}
	if err := validateFullstackNetworkRecord(
		"project",
		"minideploy-another-release-one",
	); err == nil {
		t.Fatal("accepted another project's network")
	}
	if err := validateFullstackContainerRecord(
		"project",
		"frontend",
		"--all",
	); err == nil {
		t.Fatal("accepted Docker option as container metadata")
	}
}
