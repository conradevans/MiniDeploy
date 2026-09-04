package main

import (
	"errors"
	"fmt"
	"strings"
)

func fullstackReleaseNetworkName(app string, release string) (string, error) {
	if err := validateApplicationName(app); err != nil {
		return "", err
	}
	if !isTrustedDockerIdentifier(release) {
		return "", fmt.Errorf("invalid full-stack release identifier")
	}

	return fmt.Sprintf("minideploy-%s-release-%s", app, release), nil
}

func fullstackServiceContainerName(
	app string,
	service string,
	kind string,
	release string,
) (string, error) {
	if err := validateApplicationName(app); err != nil {
		return "", err
	}
	if !isFullstackServiceName(service) ||
		!isFullstackContainerKind(kind) ||
		!isTrustedDockerIdentifier(release) {

		return "", fmt.Errorf("invalid full-stack container identity")
	}

	return fmt.Sprintf(
		"minideploy-%s-%s-%s-%s",
		app,
		service,
		kind,
		release,
	), nil
}

func fullstackServiceImageName(
	app string,
	service string,
	release string,
) (string, error) {
	if err := validateApplicationName(app); err != nil {
		return "", err
	}
	if !isFullstackServiceName(service) ||
		!isTrustedDockerIdentifier(release) {

		return "", fmt.Errorf("invalid full-stack image identity")
	}

	return fmt.Sprintf(
		"minideploy-%s-%s:%s",
		app,
		service,
		release,
	), nil
}

func isTrustedDockerIdentifier(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}

	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '.' || character == '-' {

			continue
		}
		return false
	}

	return !strings.Contains(value, "..")
}

func isFullstackServiceName(name string) bool {
	return name == fullstackFrontendService ||
		name == fullstackBackendService
}

func isFullstackContainerKind(kind string) bool {
	return kind == "release" || kind == "candidate" || kind == "rollback"
}

func validateFullstackActiveResources(record DeploymentRecord) error {
	record = normalizeDeploymentRecord(record)
	if err := validateApplicationName(record.App); err != nil {
		return err
	}
	if record.Strategy != deploymentStrategyFullstackViteNode {
		return fmt.Errorf("deployment is not a full-stack project")
	}
	if err := validateFullstackServiceMetadata(record.Services); err != nil {
		return err
	}
	if err := validateFullstackNetworkRecord(record.App, record.Network); err != nil {
		return err
	}

	for _, service := range record.Services {
		if err := validateFullstackContainerRecord(
			record.App,
			service.Name,
			service.Container,
		); err != nil {
			return err
		}
		if err := validateFullstackImageRecord(
			record.App,
			service.Name,
			service.Image,
		); err != nil {
			return err
		}
	}

	return nil
}

func validateFullstackNetworkRecord(app string, network string) error {
	if err := validateApplicationName(app); err != nil {
		return err
	}
	prefix := "minideploy-" + app + "-release-"
	if !strings.HasPrefix(network, prefix) ||
		!isTrustedDockerIdentifier(strings.TrimPrefix(network, prefix)) {

		return fmt.Errorf("invalid full-stack network metadata")
	}
	return nil
}

func validateFullstackContainerRecord(
	app string,
	service string,
	container string,
) error {
	if err := validateApplicationName(app); err != nil {
		return err
	}
	if !isFullstackServiceName(service) {
		return fmt.Errorf("invalid full-stack service metadata")
	}
	prefix := fmt.Sprintf("minideploy-%s-%s-", app, service)
	if !strings.HasPrefix(container, prefix) ||
		!isTrustedDockerIdentifier(strings.TrimPrefix(container, prefix)) {

		return fmt.Errorf("invalid full-stack container metadata")
	}
	return nil
}

func validateFullstackImageRecord(
	app string,
	service string,
	image string,
) error {
	if err := validateApplicationName(app); err != nil {
		return err
	}
	prefix := fmt.Sprintf("minideploy-%s-%s:", app, service)
	if !isFullstackServiceName(service) ||
		!strings.HasPrefix(image, prefix) ||
		!isTrustedDockerIdentifier(strings.TrimPrefix(image, prefix)) {

		return fmt.Errorf("invalid full-stack image metadata")
	}
	return nil
}

func createFullstackNetwork(app string, network string) error {
	if err := validateFullstackNetworkRecord(app, network); err != nil {
		return err
	}

	output, err := runCommand(
		"",
		"docker",
		"network",
		"create",
		"--driver",
		"bridge",
		"--label",
		"com.minideploy.managed=true",
		"--label",
		"com.minideploy.app="+app,
		network,
	)
	if err != nil {
		return fmt.Errorf("create full-stack network: %w: %s", err, output)
	}
	return nil
}

func removeFullstackNetwork(app string, network string) error {
	if network == "" {
		return nil
	}
	if err := validateFullstackNetworkRecord(app, network); err != nil {
		return err
	}

	labels, err := runCommand(
		"",
		"docker",
		"network",
		"inspect",
		"-f",
		"{{ index .Labels \"com.minideploy.managed\" }}|{{ index .Labels \"com.minideploy.app\" }}",
		network,
	)
	if err != nil {
		if dockerResourceIsMissing(labels) {
			return nil
		}
		return fmt.Errorf("inspect full-stack network: %w: %s", err, labels)
	}
	if strings.TrimSpace(labels) != "true|"+app {
		return fmt.Errorf("refusing to remove unowned Docker network %q", network)
	}

	output, err := runCommand("", "docker", "network", "rm", network)
	if err != nil {
		return fmt.Errorf("remove full-stack network: %w: %s", err, output)
	}
	return nil
}

func verifyFullstackContainerOwnership(
	app string,
	service string,
	container string,
) error {
	if err := validateFullstackContainerRecord(app, service, container); err != nil {
		return err
	}
	labels, err := runCommand(
		"",
		"docker",
		"inspect",
		"-f",
		"{{ index .Config.Labels \"com.minideploy.managed\" }}|{{ index .Config.Labels \"com.minideploy.app\" }}|{{ index .Config.Labels \"com.minideploy.service\" }}",
		container,
	)
	if err != nil {
		return fmt.Errorf("inspect full-stack container: %w: %s", err, labels)
	}
	want := strings.Join([]string{"true", app, service}, "|")
	if strings.TrimSpace(labels) != want {
		return fmt.Errorf("refusing unowned Docker container %q", container)
	}
	return nil
}

func removeFullstackContainer(
	app string,
	service string,
	container string,
) error {
	if container == "" {
		return nil
	}
	if err := verifyFullstackContainerOwnership(
		app,
		service,
		container,
	); err != nil {
		if dockerResourceIsMissing(err.Error()) {
			return nil
		}
		return err
	}

	output, err := runCommand("", "docker", "rm", "-f", container)
	if err != nil {
		return fmt.Errorf("remove full-stack container: %w: %s", err, output)
	}
	return nil
}

func dockerResourceIsMissing(output string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "no such") ||
		strings.Contains(normalized, "not found")
}

func removeFullstackImage(
	app string,
	service string,
	image string,
) error {
	if image == "" {
		return nil
	}
	if err := validateFullstackImageRecord(app, service, image); err != nil {
		return err
	}

	output, err := runCommand("", "docker", "image", "rm", image)
	if err != nil {
		return fmt.Errorf("remove full-stack image: %w: %s", err, output)
	}
	return nil
}

func cleanupFullstackRelease(
	record DeploymentRecord,
	removeImages bool,
) error {
	if err := validateFullstackActiveResources(record); err != nil {
		return err
	}

	var cleanupErrors []error
	for _, service := range record.Services {
		if err := removeFullstackContainer(
			record.App,
			service.Name,
			service.Container,
		); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if err := removeFullstackNetwork(record.App, record.Network); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	if removeImages {
		for _, service := range record.Services {
			if err := removeFullstackImage(
				record.App,
				service.Name,
				service.Image,
			); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
	}

	return errors.Join(cleanupErrors...)
}

func fullstackServiceRuntimeEnvironment(
	service string,
	environment map[string]string,
) map[string]string {
	if service != fullstackBackendService {
		return nil
	}
	return cloneRuntimeEnvironment(environment)
}

func deploymentProjectStatus(record DeploymentRecord) string {
	record = normalizeDeploymentRecord(record)
	if record.DatabaseDetached {
		return "database-detached"
	}
	if record.Strategy != deploymentStrategyFullstackViteNode {
		return containerStatus(record.Container)
	}
	if validateFullstackServiceMetadata(record.Services) != nil {
		return "missing"
	}

	for _, serviceName := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, _ := deploymentServiceByName(record, serviceName)
		status := containerStatus(service.Container)
		if status != "running" {
			return status
		}
	}
	return "running"
}

func deploymentServiceResponses(
	record DeploymentRecord,
) []DeploymentServiceResponse {
	if record.Strategy != deploymentStrategyFullstackViteNode {
		return nil
	}

	responses := make([]DeploymentServiceResponse, 0, len(record.Services))
	for _, name := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, ok := deploymentServiceByName(record, name)
		if !ok {
			continue
		}
		responses = append(responses, DeploymentServiceResponse{
			Name:               service.Name,
			Path:               service.Path,
			Strategy:           service.Strategy,
			Container:          service.Container,
			Image:              service.Image,
			Port:               service.Port,
			ContainerPort:      service.ContainerPort,
			HealthPath:         service.HealthPath,
			PackageManager:     service.PackageManager,
			PackageInstallMode: service.PackageInstallMode,
			Status:             containerStatus(service.Container),
		})
	}
	return responses
}

func deploymentRuntimeLogs(
	record DeploymentRecord,
	tail int,
	environment map[string]string,
) (string, string, error) {
	record = normalizeDeploymentRecord(record)
	if record.Strategy != deploymentStrategyFullstackViteNode {
		if !containerExists(record.Container) {
			return "", "", fmt.Errorf("deployment container not found")
		}
		logs, err := containerLogs(record.Container, tail, environment)
		return record.Container, logs, err
	}

	if err := validateFullstackServiceMetadata(record.Services); err != nil {
		return "", "", err
	}
	var combined strings.Builder
	containers := make([]string, 0, 2)
	for _, name := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, _ := deploymentServiceByName(record, name)
		if err := verifyFullstackContainerOwnership(
			record.App,
			name,
			service.Container,
		); err != nil {
			return "", "", fmt.Errorf("verify %s service: %w", name, err)
		}
		logs, err := containerLogs(service.Container, tail, environment)
		if err != nil {
			return "", logs, fmt.Errorf("retrieve %s logs: %w", name, err)
		}
		containers = append(containers, service.Container)
		fmt.Fprintf(
			&combined,
			"== %s (%s) ==\n%s",
			fullstackServiceDisplayName(name),
			service.Container,
			logs,
		)
		if !strings.HasSuffix(logs, "\n") {
			combined.WriteByte('\n')
		}
	}
	return strings.Join(containers, ", "), combined.String(), nil
}

func restartDeploymentContainers(
	record DeploymentRecord,
) (string, string, error) {
	record = normalizeDeploymentRecord(record)
	if record.Strategy != deploymentStrategyFullstackViteNode {
		if !containerExists(record.Container) {
			return "", "", fmt.Errorf("deployment container not found")
		}
		output, err := runCommand(
			"",
			"docker",
			restartContainerArguments(record.Container)...,
		)
		return record.Container, output, err
	}

	if err := validateFullstackActiveResources(record); err != nil {
		return "", "", err
	}
	for _, name := range []string{
		fullstackBackendService,
		fullstackFrontendService,
	} {
		service, _ := deploymentServiceByName(record, name)
		if err := verifyFullstackContainerOwnership(
			record.App,
			name,
			service.Container,
		); err != nil {
			return "", "", fmt.Errorf(
				"verify %s service container: %w", name, err)
		}
	}

	containers := make([]string, 0, 2)
	var output strings.Builder
	for _, name := range []string{
		fullstackBackendService,
		fullstackFrontendService,
	} {
		service, _ := deploymentServiceByName(record, name)
		commandOutput, err := runCommand(
			"",
			"docker",
			restartContainerArguments(service.Container)...,
		)
		output.WriteString(commandOutput)
		if err != nil {
			return strings.Join(containers, ", "), output.String(),
				fmt.Errorf("restart %s service: %w", name, err)
		}
		containers = append(containers, service.Container)
	}

	return strings.Join(containers, ", "), output.String(), nil
}

func deploymentImages(record DeploymentRecord) []string {
	if record.Strategy != deploymentStrategyFullstackViteNode {
		if record.Image == "" {
			return nil
		}
		return []string{record.Image}
	}

	images := make([]string, 0, len(record.Services))
	for _, service := range record.Services {
		if service.Image != "" {
			images = append(images, service.Image)
		}
	}
	return images
}
