package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DeploymentVersion struct {
	App           string    `json:"app"`
	RepoURL       string    `json:"repoUrl"`
	Container     string    `json:"container"`
	Image         string    `json:"image"`
	Port          int       `json:"port"`
	ContainerPort int       `json:"containerPort"`
	HealthPath    string    `json:"healthPath"`
	DeployedAt    time.Time `json:"deployedAt"`
}

func deploymentVersion(record DeploymentRecord) DeploymentVersion {
	record = normalizeDeploymentRecord(record)

	return DeploymentVersion{
		App:           record.App,
		RepoURL:       record.RepoURL,
		Container:     record.Container,
		Image:         record.Image,
		Port:          record.Port,
		ContainerPort: record.ContainerPort,
		HealthPath:    record.HealthPath,
		DeployedAt:    time.Now().UTC(),
	}
}

func (v DeploymentVersion) Record() DeploymentRecord {
	return normalizeDeploymentRecord(
		DeploymentRecord{
			App:           v.App,
			RepoURL:       v.RepoURL,
			Container:     v.Container,
			Image:         v.Image,
			Port:          v.Port,
			ContainerPort: v.ContainerPort,
			HealthPath:    v.HealthPath,
		},
	)
}

// RecordWithFallback supports history written before ContainerPort
// and HealthPath were persisted. New history entries are completely
// self-contained, while legacy entries safely inherit the current
// deployment configuration.
func (v DeploymentVersion) RecordWithFallback(
	fallback DeploymentRecord,
) DeploymentRecord {
	fallback = normalizeDeploymentRecord(fallback)

	record := v.Record()

	if v.ContainerPort == 0 {
		record.ContainerPort = fallback.ContainerPort
	}

	if v.HealthPath == "" {
		record.HealthPath = fallback.HealthPath
	}

	return normalizeDeploymentRecord(record)
}

type JSONHistoryStore struct {
	path        string
	maxPrevious int
	mu          sync.Mutex
}

func NewJSONHistoryStore(path string, maxPrevious int) *JSONHistoryStore {
	return &JSONHistoryStore{
		path:        path,
		maxPrevious: maxPrevious,
	}
}

func (s *JSONHistoryStore) List(app string) ([]DeploymentVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	history, err := s.read()
	if err != nil {
		return nil, err
	}

	versions := history[app]

	result := make([]DeploymentVersion, len(versions))
	copy(result, versions)

	return result, nil
}

func (s *JSONHistoryStore) Push(
	record DeploymentRecord,
) ([]DeploymentVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	history, err := s.read()
	if err != nil {
		return nil, err
	}

	existing := history[record.App]

	filtered := make([]DeploymentVersion, 0, len(existing))
	for _, version := range existing {
		if version.Image != record.Image {
			filtered = append(filtered, version)
		}
	}

	versions := append(
		[]DeploymentVersion{deploymentVersion(record)},
		filtered...,
	)

	var pruned []DeploymentVersion

	if len(versions) > s.maxPrevious {
		pruned = append(
			pruned,
			versions[s.maxPrevious:]...,
		)

		versions = versions[:s.maxPrevious]
	}

	history[record.App] = versions

	if err := s.write(history); err != nil {
		return nil, err
	}

	return pruned, nil
}

func (s *JSONHistoryStore) Set(
	app string,
	versions []DeploymentVersion,
) ([]DeploymentVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	history, err := s.read()
	if err != nil {
		return nil, err
	}

	var pruned []DeploymentVersion

	if len(versions) > s.maxPrevious {
		pruned = append(
			pruned,
			versions[s.maxPrevious:]...,
		)

		versions = versions[:s.maxPrevious]
	}

	history[app] = versions

	if err := s.write(history); err != nil {
		return nil, err
	}

	return pruned, nil
}

func (s *JSONHistoryStore) Clear(app string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	history, err := s.read()
	if err != nil {
		return err
	}

	delete(history, app)

	return s.write(history)
}

func (s *JSONHistoryStore) read() (
	map[string][]DeploymentVersion,
	error,
) {
	data, err := os.ReadFile(s.path)

	if errors.Is(err, os.ErrNotExist) {
		return make(map[string][]DeploymentVersion), nil
	}

	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return make(map[string][]DeploymentVersion), nil
	}

	var history map[string][]DeploymentVersion

	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	if history == nil {
		history = make(map[string][]DeploymentVersion)
	}

	return history, nil
}

func (s *JSONHistoryStore) write(
	history map[string][]DeploymentVersion,
) error {
	if err := os.MkdirAll(
		filepath.Dir(s.path),
		0755,
	); err != nil {
		return err
	}

	data, err := json.MarshalIndent(
		history,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	tempPath := s.path + ".tmp"

	if err := os.WriteFile(
		tempPath,
		data,
		0644,
	); err != nil {
		return err
	}

	return os.Rename(
		tempPath,
		s.path,
	)
}
