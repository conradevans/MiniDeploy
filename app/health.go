package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func verifyContainerStartup(
	containerName string,
) error {
	const checks = 3

	for i := 0; i < checks; i++ {
		time.Sleep(time.Second)

		output, err := runCommand(
			"",
			"docker",
			"inspect",
			"-f",
			"{{.State.Status}}",
			containerName,
		)

		if err != nil {
			return fmt.Errorf(
				"inspect container: %w",
				err,
			)
		}

		status := strings.TrimSpace(output)

		if status != "running" {
			return fmt.Errorf(
				"container entered %q state",
				status,
			)
		}
	}

	return nil
}

func verifyHTTPHealth(port int) error {
	return verifyHTTPHealthPath(
		port,
		defaultHealthPath,
	)
}

func verifyHTTPHealthPath(
	port int,
	healthPath string,
) error {
	const attempts = 5

	healthPath = normalizedHealthPath(
		healthPath,
	)

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	url := fmt.Sprintf(
		"http://127.0.0.1:%d%s",
		port,
		healthPath,
	)

	var lastErr error

	for i := 0; i < attempts; i++ {
		response, err := client.Get(url)

		if err != nil {
			lastErr = err
		} else {
			response.Body.Close()

			if response.StatusCode >= 200 &&
				response.StatusCode < 400 {
				return nil
			}

			lastErr = fmt.Errorf(
				"received HTTP status %d",
				response.StatusCode,
			)
		}

		if i < attempts-1 {
			time.Sleep(time.Second)
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf(
			"unknown health-check failure",
		)
	}

	return fmt.Errorf(
		"HTTP health check failed for %s: %w",
		url,
		lastErr,
	)
}
