package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

func startManagedContainer(
	containerName string,
	imageName string,
	port int,
) error {
	return startManagedContainerWithPort(
		containerName,
		imageName,
		port,
		defaultContainerPort,
	)
}

func startManagedContainerWithPort(
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
) error {
	return startManagedDeploymentContainer(
		containerName,
		containerName,
		imageName,
		hostPort,
		containerPort,
		deploymentStrategyDockerfile,
		nil,
	)
}

func startManagedDeploymentContainer(
	app string,
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
	strategy string,
	environment map[string]string,
) error {
	dockerEnvironment := dockerRuntimeEnvironment(
		strategy,
		containerPort,
		environment,
	)

	envFile, cleanup, err :=
		runtimeEnvironmentStore.TemporaryDockerEnvFile(
			app,
			dockerEnvironment,
		)
	if err != nil {
		return fmt.Errorf(
			"prepare Docker runtime environment: %w",
			err,
		)
	}
	defer cleanup()

	args := managedContainerRunArguments(
		containerName,
		imageName,
		hostPort,
		containerPort,
		envFile,
	)

	output, err := runCommand(
		"",
		"docker",
		args...,
	)
	if err != nil {
		log.Printf(
			"docker run failed:\n%s",
			output,
		)
		return fmt.Errorf(
			"docker run: %w",
			err,
		)
	}
	return nil
}

func dockerRuntimeEnvironment(
	strategy string,
	containerPort int,
	environment map[string]string,
) map[string]string {
	dockerEnvironment := cloneRuntimeEnvironment(
		environment,
	)

	if strategy == deploymentStrategyNodeExpress {
		dockerEnvironment["PORT"] = strconv.Itoa(
			containerPort,
		)
	}

	return dockerEnvironment
}

func managedContainerRunArguments(
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
	envFile string,
) []string {
	args := []string{
		"run",
		"-d",
		"--restart",
		"unless-stopped",
		"--name",
		containerName,
		"-p",
		managedPortBinding(
			hostPort,
			containerPort,
		),
	}

	if envFile != "" {
		args = append(
			args,
			"--env-file",
			envFile,
		)
	}

	return append(args, imageName)
}

func managedPortBinding(
	hostPort int,
	containerPort int,
) string {
	return fmt.Sprintf(
		"127.0.0.1:%d:%d",
		hostPort,
		containerPort,
	)
}

func restartContainerArguments(containerName string) []string {
	return []string{
		"restart",
		containerName,
	}
}

func containerLogs(
	containerName string,
	tail int,
	environment map[string]string,
) (string, error) {
	output, err := runCommand(
		"",
		"docker",
		"logs",
		"--tail",
		fmt.Sprintf("%d", tail),
		containerName,
	)

	return redactRuntimeEnvironmentValues(
		output,
		environment,
	), err
}

func containerExists(
	containerName string,
) bool {
	_, err := runCommand(
		"",
		"docker",
		"inspect",
		containerName,
	)
	return err == nil
}

func containerStatus(
	containerName string,
) string {
	output, err := runCommand(
		"",
		"docker",
		"inspect",
		"-f",
		"{{.State.Status}}",
		containerName,
	)
	if err != nil {
		return "missing"
	}

	return strings.TrimSpace(output)
}

func findAvailablePort(
	start int,
	end int,
) (int, error) {
	for port := start; port <= end; port++ {
		listener, err := net.Listen(
			"tcp",
			fmt.Sprintf(
				"127.0.0.1:%d",
				port,
			),
		)
		if err != nil {
			continue
		}

		listener.Close()
		return port, nil
	}

	return 0, fmt.Errorf(
		"no available ports between %d and %d",
		start,
		end,
	)
}

func runCommand(
	dir string,
	name string,
	args ...string,
) (string, error) {
	cmd := exec.Command(
		name,
		args...,
	)

	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}
