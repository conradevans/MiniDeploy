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

type managedContainerOptions struct {
	Network     string
	DataNetwork string
	Service     string
	App         string
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
	return startManagedDeploymentContainerWithOptions(
		app,
		containerName,
		imageName,
		hostPort,
		containerPort,
		strategy,
		environment,
		managedContainerOptions{},
	)
}

func startManagedProjectServiceContainer(
	app string,
	service string,
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
	strategy string,
	environment map[string]string,
	network string,
) error {
	return startManagedDeploymentContainerWithOptions(
		app,
		containerName,
		imageName,
		hostPort,
		containerPort,
		strategy,
		environment,
		managedContainerOptions{
			Network: network,
			Service: service,
			App:     app,
		},
	)
}

func startManagedDeploymentContainerWithOptions(
	app string,
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
	strategy string,
	environment map[string]string,
	options managedContainerOptions,
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

	if options.DataNetwork != "" {
		if options.DataNetwork != reactorLabDataNetwork {
			return fmt.Errorf("unsupported managed data network")
		}
		if err := validateReactorLabDataNetwork(); err != nil {
			return err
		}
		args := managedContainerRunArgumentsWithOptions(
			containerName, imageName, hostPort, containerPort, envFile, options,
		)
		args[0] = "create"
		args = append(args[:1], args[2:]...)
		output, err := runCommand("", "docker", args...)
		if err != nil {
			log.Printf("docker create failed:\n%s", output)
			return fmt.Errorf("docker create: %w", err)
		}
		cleanupContainer := func() {
			_, _ = runCommand("", "docker", "rm", "-f", containerName)
		}
		if output, err := runCommand("", "docker", "network", "connect", options.DataNetwork, containerName); err != nil {
			log.Printf("docker data network connect failed:\n%s", output)
			cleanupContainer()
			return fmt.Errorf("connect private data network: %w", err)
		}
		if output, err := runCommand("", "docker", "start", containerName); err != nil {
			log.Printf("docker start failed:\n%s", output)
			cleanupContainer()
			return fmt.Errorf("docker start: %w", err)
		}
		return nil
	}

	args := managedContainerRunArgumentsWithOptions(
		containerName,
		imageName,
		hostPort,
		containerPort,
		envFile,
		options,
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
	return managedContainerRunArgumentsWithOptions(
		containerName,
		imageName,
		hostPort,
		containerPort,
		envFile,
		managedContainerOptions{},
	)
}

func managedContainerRunArgumentsWithOptions(
	containerName string,
	imageName string,
	hostPort int,
	containerPort int,
	envFile string,
	options managedContainerOptions,
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

	if options.Network != "" {
		args = append(
			args,
			"--network",
			options.Network,
			"--network-alias",
			options.Service,
			"--label",
			"com.minideploy.managed=true",
			"--label",
			"com.minideploy.app="+options.App,
			"--label",
			"com.minideploy.service="+options.Service,
		)
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

var commandRunner = executeCommand

func runCommand(
	dir string,
	name string,
	args ...string,
) (string, error) {
	return commandRunner(dir, name, args...)
}

func executeCommand(
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
