package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateApplicationName(name string) error {
	if strings.TrimSpace(name) == "" ||
		name == "." ||
		name == ".." ||
		strings.ContainsAny(name, "/\\") {

		return fmt.Errorf("invalid application name %q", name)
	}

	return nil
}

func strictChildPath(
	root string,
	candidate string,
) (string, error) {
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf(
			"resolve managed deployment root: %w",
			err,
		)
	}

	cleanCandidate, err := filepath.Abs(
		filepath.Clean(candidate),
	)
	if err != nil {
		return "", fmt.Errorf(
			"resolve managed deployment path: %w",
			err,
		)
	}

	relative, err := filepath.Rel(
		cleanRoot,
		cleanCandidate,
	)
	if err != nil {
		return "", fmt.Errorf(
			"compare managed deployment path: %w",
			err,
		)
	}

	parentPrefix := ".." + string(filepath.Separator)
	if relative == "" ||
		relative == "." ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(relative, parentPrefix) {

		return "", fmt.Errorf(
			"deployment path %q is not a strict child of %q",
			cleanCandidate,
			cleanRoot,
		)
	}

	return cleanCandidate, nil
}

func managedDeploymentPath(
	appName string,
) (string, error) {
	if err := validateApplicationName(appName); err != nil {
		return "", err
	}

	return strictChildPath(
		deploymentsDir,
		filepath.Join(deploymentsDir, appName),
	)
}

func removeStrictChildPath(
	root string,
	candidate string,
) error {
	safePath, err := strictChildPath(root, candidate)
	if err != nil {
		return err
	}

	return os.RemoveAll(safePath)
}

func removeManagedDeploymentPath(path string) error {
	return removeStrictChildPath(deploymentsDir, path)
}
