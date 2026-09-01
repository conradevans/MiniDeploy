package main

import (
	"fmt"
	"strings"
)

const (
	defaultContainerPort     = 80
	defaultNodeContainerPort = 3000
	defaultHealthPath        = "/"
)

func normalizedContainerPort(port int) int {
	if port == 0 {
		return defaultContainerPort
	}

	return port
}

func normalizedHealthPath(path string) string {
	path = strings.TrimSpace(path)

	if path == "" {
		return defaultHealthPath
	}

	return path
}

func validateDeploymentConfig(
	containerPort int,
	healthPath string,
) error {
	if containerPort < 1 || containerPort > 65535 {
		return fmt.Errorf(
			"container port must be between 1 and 65535",
		)
	}

	if !strings.HasPrefix(healthPath, "/") {
		return fmt.Errorf(
			"health path must begin with /",
		)
	}

	return nil
}

func validateRequestedDeploymentConfig(
	containerPort int,
	healthPath string,
) error {
	if containerPort != 0 &&
		(containerPort < 1 || containerPort > 65535) {

		return fmt.Errorf(
			"container port must be between 1 and 65535",
		)
	}

	healthPath = strings.TrimSpace(healthPath)
	if healthPath != "" &&
		!strings.HasPrefix(healthPath, "/") {

		return fmt.Errorf(
			"health path must begin with /",
		)
	}

	return nil
}

func normalizeDeploymentRecord(
	record DeploymentRecord,
) DeploymentRecord {
	if strings.TrimSpace(record.Strategy) == "" {
		record.Strategy = deploymentStrategyDockerfile
	}

	record.ContainerPort = normalizedContainerPort(
		record.ContainerPort,
	)

	record.HealthPath = normalizedHealthPath(
		record.HealthPath,
	)

	record.EnvironmentVariables =
		normalizeRuntimeEnvironmentNames(
			record.EnvironmentVariables,
		)

	for index := range record.Services {
		service := &record.Services[index]
		service.ContainerPort = normalizedServiceContainerPort(
			service.Strategy,
			service.ContainerPort,
		)
		service.HealthPath = normalizedHealthPath(service.HealthPath)
	}

	if record.Strategy == deploymentStrategyFullstackViteNode {
		if frontend, ok := deploymentServiceByName(record, fullstackFrontendService); ok {
			record.Container = frontend.Container
			record.Image = frontend.Image
			record.Port = frontend.Port
			record.ContainerPort = frontend.ContainerPort
			record.HealthPath = frontend.HealthPath
			record.PackageManager = ""
			record.PackageInstallMode = ""
		}
	}

	return record
}

func normalizedServiceContainerPort(strategy string, port int) int {
	if port != 0 {
		return port
	}

	if strategy == deploymentStrategyNodeExpress {
		return defaultNodeContainerPort
	}

	return defaultContainerPort
}

func cloneDeploymentServices(
	services []DeploymentServiceRecord,
) []DeploymentServiceRecord {
	if len(services) == 0 {
		return nil
	}

	cloned := make([]DeploymentServiceRecord, len(services))
	copy(cloned, services)
	return cloned
}

func deploymentServiceByName(
	record DeploymentRecord,
	name string,
) (DeploymentServiceRecord, bool) {
	for _, service := range record.Services {
		if service.Name == name {
			return service, true
		}
	}

	return DeploymentServiceRecord{}, false
}
