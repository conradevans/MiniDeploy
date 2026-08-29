package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
)

func startManagedContainer(
	containerName string,
	imageName string,
	port int,
) error {
	output, err := runCommand(
		"",
		"docker",
		"run",
		"-d",
		"--restart",
		"unless-stopped",
		"--name",
		containerName,
		"-p",
		fmt.Sprintf(
			"%d:80",
			port,
		),
		imageName,
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

func containerLogs(
	containerName string,
	tail int,
) (string, error) {
	return runCommand(
		"",
		"docker",
		"logs",
		"--tail",
		fmt.Sprintf("%d", tail),
		containerName,
	)
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
				":%d",
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
