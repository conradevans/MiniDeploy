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

	return record
}
