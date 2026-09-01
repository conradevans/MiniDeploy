package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	deploymentLogsDir = "/srv/minideploy/data/deploy-logs"
	deploymentLogMu   sync.Mutex
)

func resetDeploymentLog(app string, operation string) {
	path, err := deploymentLogPath(app)
	if err != nil {
		log.Printf("failed to prepare deployment log for %s: %v", app, err)
		return
	}

	deploymentLogMu.Lock()
	defer deploymentLogMu.Unlock()

	if err := os.MkdirAll(deploymentLogsDir, 0755); err != nil {
		log.Printf("failed to create deployment log directory: %v", err)
		return
	}

	content := fmt.Sprintf(
		"%s  Starting %s for %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		operation,
		app,
	)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		log.Printf("failed to reset deployment log for %s: %v", app, err)
	}
}

func deploymentEvent(app string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)

	log.Printf("[%s] %s", app, message)

	path, err := deploymentLogPath(app)
	if err != nil {
		log.Printf("failed to resolve deployment log for %s: %v", app, err)
		return
	}

	deploymentLogMu.Lock()
	defer deploymentLogMu.Unlock()

	if err := os.MkdirAll(deploymentLogsDir, 0755); err != nil {
		log.Printf("failed to create deployment log directory: %v", err)
		return
	}

	file, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		log.Printf("failed to open deployment log for %s: %v", app, err)
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message = strings.TrimRight(message, "\n")

	for _, line := range strings.Split(message, "\n") {
		if _, err := fmt.Fprintf(
			file,
			"%s  %s\n",
			timestamp,
			line,
		); err != nil {
			log.Printf("failed writing deployment log for %s: %v", app, err)
			return
		}
	}
}

func readDeploymentLog(app string) (string, error) {
	path, err := deploymentLogPath(app)
	if err != nil {
		return "", err
	}

	deploymentLogMu.Lock()
	defer deploymentLogMu.Unlock()

	data, err := os.ReadFile(path)

	if os.IsNotExist(err) {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	return string(data), nil
}

func deploymentLogPath(app string) (string, error) {
	app = strings.TrimSpace(app)

	if app == "" ||
		app == "." ||
		app == ".." ||
		filepath.Base(app) != app {
		return "", fmt.Errorf("invalid app name")
	}

	for _, r := range app {
		valid :=
			(r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') ||
				r == '-' ||
				r == '_' ||
				r == '.'

		if !valid {
			return "", fmt.Errorf("invalid app name")
		}
	}

	return filepath.Join(
		deploymentLogsDir,
		app+".log",
	), nil
}
