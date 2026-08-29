package main

import (
	"fmt"
	"strings"
)

const (
	defaultContainerPort = 80
	defaultHealthPath    = "/"
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

func normalizeDeploymentRecord(
	record DeploymentRecord,
) DeploymentRecord {
	record.ContainerPort = normalizedContainerPort(
		record.ContainerPort,
	)

	record.HealthPath = normalizedHealthPath(
		record.HealthPath,
	)

	return record
}
