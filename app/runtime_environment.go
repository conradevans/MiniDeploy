package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	runtimeSecretsDirectory  = "/srv/minideploy/data/secrets"
	redactedEnvironmentValue = "[REDACTED]"
)

var (
	runtimeEnvironmentNamePattern = regexp.MustCompile(
		`^[A-Za-z_][A-Za-z0-9_]*$`,
	)
	runtimeEnvironmentStore = newRuntimeEnvironmentFileStore(
		runtimeSecretsDirectory,
	)
)

type runtimeEnvironmentFileStore struct {
	root string
}

func newRuntimeEnvironmentFileStore(
	root string,
) *runtimeEnvironmentFileStore {
	return &runtimeEnvironmentFileStore{
		root: root,
	}
}

func validateRuntimeEnvironment(
	values map[string]string,
) error {
	for name, value := range values {
		if !runtimeEnvironmentNamePattern.MatchString(name) {
			return fmt.Errorf(
				"invalid runtime environment variable name %q",
				name,
			)
		}

		if name == "PORT" {
			return fmt.Errorf(
				"runtime environment variable PORT is managed by MiniDeploy",
			)
		}

		if strings.ContainsRune(value, '\x00') {
			return fmt.Errorf(
				"runtime environment variable %q contains NUL",
				name,
			)
		}

		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf(
				"runtime environment variable %q contains a multiline value, which is not supported",
				name,
			)
		}
	}

	return nil
}

func cloneRuntimeEnvironment(
	values map[string]string,
) map[string]string {
	cloned := make(map[string]string, len(values))

	for name, value := range values {
		cloned[name] = value
	}

	return cloned
}

func runtimeEnvironmentNames(
	values map[string]string,
) []string {
	names := make([]string, 0, len(values))

	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func normalizeRuntimeEnvironmentNames(
	names []string,
) []string {
	unique := make(map[string]bool)

	for _, name := range names {
		if name == "PORT" ||
			!runtimeEnvironmentNamePattern.MatchString(name) {

			continue
		}

		unique[name] = true
	}

	normalized := make([]string, 0, len(unique))
	for name := range unique {
		normalized = append(normalized, name)
	}

	sort.Strings(normalized)
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func verifyRuntimeEnvironmentMetadata(
	record DeploymentRecord,
	values map[string]string,
) error {
	expected := normalizeRuntimeEnvironmentNames(
		record.EnvironmentVariables,
	)
	actual := runtimeEnvironmentNames(values)

	if !slices.Equal(expected, actual) {
		return fmt.Errorf(
			"runtime environment metadata does not match secure storage",
		)
	}

	return nil
}

func (s *runtimeEnvironmentFileStore) path(
	app string,
) (string, error) {
	if err := validateApplicationName(app); err != nil {
		return "", err
	}

	return strictChildPath(
		s.root,
		filepath.Join(s.root, app+".env"),
	)
}

func (s *runtimeEnvironmentFileStore) ensureRoot() error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return fmt.Errorf(
			"create runtime environment directory: %w",
			err,
		)
	}

	if err := os.Chmod(s.root, 0700); err != nil {
		return fmt.Errorf(
			"secure runtime environment directory: %w",
			err,
		)
	}

	return nil
}

func (s *runtimeEnvironmentFileStore) Load(
	app string,
) (map[string]string, error) {
	path, err := s.path(app)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve runtime environment path: %w",
			err,
		)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf(
			"read runtime environment: %w",
			err,
		)
	}

	rootInfo, err := os.Stat(s.root)
	if err != nil {
		return nil, fmt.Errorf(
			"inspect runtime environment directory: %w",
			err,
		)
	}

	if !rootInfo.IsDir() || rootInfo.Mode().Perm() != 0700 {
		return nil, fmt.Errorf(
			"runtime environment directory permissions must be 0700",
		)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf(
			"inspect runtime environment: %w",
			err,
		)
	}

	if info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf(
			"runtime environment file permissions must be 0600",
		)
	}

	values, err := parseRuntimeEnvironment(data)
	if err != nil {
		return nil, fmt.Errorf(
			"parse runtime environment: %w",
			err,
		)
	}

	return values, nil
}

func parseRuntimeEnvironment(
	data []byte,
) (map[string]string, error) {
	values := make(map[string]string)

	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}

		parts := bytes.SplitN(line, []byte{'='}, 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(
				"invalid runtime environment file",
			)
		}

		name := string(parts[0])
		value := string(parts[1])

		if _, exists := values[name]; exists {
			return nil, fmt.Errorf(
				"duplicate runtime environment variable %q",
				name,
			)
		}

		values[name] = value
	}

	if err := validateRuntimeEnvironment(values); err != nil {
		return nil, err
	}

	return values, nil
}

func encodeRuntimeEnvironment(
	values map[string]string,
) []byte {
	var content strings.Builder

	for _, name := range runtimeEnvironmentNames(values) {
		content.WriteString(name)
		content.WriteByte('=')
		content.WriteString(values[name])
		content.WriteByte('\n')
	}

	return []byte(content.String())
}

func (s *runtimeEnvironmentFileStore) Replace(
	app string,
	values map[string]string,
) error {
	if err := validateRuntimeEnvironment(values); err != nil {
		return err
	}

	path, err := s.path(app)
	if err != nil {
		return fmt.Errorf(
			"resolve runtime environment path: %w",
			err,
		)
	}

	if len(values) == 0 {
		return removeStrictChildPath(s.root, path)
	}

	if err := s.ensureRoot(); err != nil {
		return err
	}

	file, err := os.CreateTemp(
		s.root,
		"."+app+"-environment-*.tmp",
	)
	if err != nil {
		return fmt.Errorf(
			"create temporary runtime environment: %w",
			err,
		)
	}

	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = removeStrictChildPath(s.root, tempPath)
	}()

	if err := file.Chmod(0600); err != nil {
		return fmt.Errorf(
			"secure temporary runtime environment: %w",
			err,
		)
	}

	if _, err := file.Write(
		encodeRuntimeEnvironment(values),
	); err != nil {
		return fmt.Errorf(
			"write runtime environment: %w",
			err,
		)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf(
			"sync runtime environment: %w",
			err,
		)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf(
			"close runtime environment: %w",
			err,
		)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf(
			"replace runtime environment: %w",
			err,
		)
	}

	return nil
}

func (s *runtimeEnvironmentFileStore) Delete(
	app string,
) error {
	path, err := s.path(app)
	if err != nil {
		return fmt.Errorf(
			"resolve runtime environment path: %w",
			err,
		)
	}

	return removeStrictChildPath(s.root, path)
}

func (s *runtimeEnvironmentFileStore) TemporaryDockerEnvFile(
	app string,
	values map[string]string,
) (string, func(), error) {
	if len(values) == 0 {
		return "", func() {}, nil
	}

	if err := validateDockerRuntimeEnvironment(values); err != nil {
		return "", nil, err
	}

	if err := s.ensureRoot(); err != nil {
		return "", nil, err
	}

	file, err := os.CreateTemp(
		s.root,
		"."+app+"-docker-*.env",
	)
	if err != nil {
		return "", nil, fmt.Errorf(
			"create Docker runtime environment: %w",
			err,
		)
	}

	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = removeStrictChildPath(s.root, path)
	}

	if err := file.Chmod(0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf(
			"secure Docker runtime environment: %w",
			err,
		)
	}

	if _, err := file.Write(
		encodeRuntimeEnvironment(values),
	); err != nil {
		cleanup()
		return "", nil, fmt.Errorf(
			"write Docker runtime environment: %w",
			err,
		)
	}

	if err := file.Sync(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf(
			"sync Docker runtime environment: %w",
			err,
		)
	}

	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf(
			"close Docker runtime environment: %w",
			err,
		)
	}

	return path, cleanup, nil
}

func validateDockerRuntimeEnvironment(
	values map[string]string,
) error {
	for name, value := range values {
		if !runtimeEnvironmentNamePattern.MatchString(name) {
			return fmt.Errorf(
				"invalid Docker runtime environment variable name %q",
				name,
			)
		}

		if strings.ContainsRune(value, '\x00') ||
			strings.ContainsAny(value, "\r\n") {

			return fmt.Errorf(
				"invalid Docker runtime environment value for %q",
				name,
			)
		}
	}

	return nil
}

type runtimeEnvironmentChange struct {
	app       string
	previous  map[string]string
	effective map[string]string
	replace   bool
	committed bool
}

func prepareRuntimeEnvironmentChange(
	app string,
	replacement map[string]string,
	replace bool,
) (*runtimeEnvironmentChange, error) {
	previous, err := runtimeEnvironmentStore.Load(app)
	if err != nil {
		return nil, err
	}

	effective := cloneRuntimeEnvironment(previous)
	if replace {
		if err := validateRuntimeEnvironment(replacement); err != nil {
			return nil, err
		}

		effective = cloneRuntimeEnvironment(replacement)
	}

	return &runtimeEnvironmentChange{
		app:       app,
		previous:  previous,
		effective: effective,
		replace:   replace,
	}, nil
}

func (c *runtimeEnvironmentChange) Commit() error {
	if !c.replace {
		return nil
	}

	if err := runtimeEnvironmentStore.Replace(
		c.app,
		c.effective,
	); err != nil {
		return err
	}

	c.committed = true
	return nil
}

func (c *runtimeEnvironmentChange) Rollback() error {
	if !c.committed {
		return nil
	}

	if err := runtimeEnvironmentStore.Replace(
		c.app,
		c.previous,
	); err != nil {
		return err
	}

	c.committed = false
	return nil
}

func redactRuntimeEnvironmentValues(
	text string,
	values map[string]string,
) string {
	sensitiveValues := make([]string, 0, len(values))

	for _, value := range values {
		if value != "" {
			sensitiveValues = append(sensitiveValues, value)
		}
	}

	sort.Slice(
		sensitiveValues,
		func(i, j int) bool {
			return len(sensitiveValues[i]) > len(sensitiveValues[j])
		},
	)

	for _, value := range sensitiveValues {
		text = strings.ReplaceAll(
			text,
			value,
			redactedEnvironmentValue,
		)
	}

	return text
}
