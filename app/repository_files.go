package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// repositoryRegularFile resolves a repository-controlled file without allowing
// a symlink to escape the checkout. Callers provide only known relative paths.
func repositoryRegularFile(
	repositoryPath string,
	relativePath string,
) (string, bool, error) {
	if filepath.IsAbs(relativePath) {
		return "", false, fmt.Errorf("repository file path must be relative")
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanRelative == "." || cleanRelative == ".." ||
		strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {

		return "", false, fmt.Errorf("repository file path escapes checkout")
	}

	cleanRoot, err := filepath.Abs(filepath.Clean(repositoryPath))
	if err != nil {
		return "", false, fmt.Errorf("resolve repository root: %w", err)
	}
	candidate, err := strictChildPath(
		cleanRoot,
		filepath.Join(cleanRoot, cleanRelative),
	)
	if err != nil {
		return "", false, err
	}
	_, err = os.Lstat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect repository file: %w", err)
	}

	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", false, fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false, fmt.Errorf("resolve repository file symlinks: %w", err)
	}
	resolvedCandidate, err = strictChildPath(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", false, fmt.Errorf("repository file escapes checkout: %w", err)
	}
	info, err := os.Stat(resolvedCandidate)
	if err != nil {
		return "", false, fmt.Errorf("inspect resolved repository file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("repository file is not a regular file")
	}
	return resolvedCandidate, true, nil
}
