package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var ErrDeploymentNotFound = errors.New("deployment not found")

type DeploymentRecord struct {
	App           string `json:"app"`
	RepoURL       string `json:"repoUrl"`
	Container     string `json:"container"`
	Image         string `json:"image"`
	Port          int    `json:"port"`
	ContainerPort int    `json:"containerPort"`
	HealthPath    string `json:"healthPath"`
}

type DeploymentStore interface {
	Save(deployment DeploymentRecord) error
	Get(app string) (DeploymentRecord, error)
	List() ([]DeploymentRecord, error)
	Delete(app string) error
}

type JSONStore struct {
	path string
	mu   sync.Mutex
}

func NewJSONStore(path string) *JSONStore {
	return &JSONStore{
		path: path,
	}
}

func (s *JSONStore) Save(deployment DeploymentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.load()
	if err != nil {
		return err
	}

	updated := false

	for i, existing := range deployments {
		if existing.App == deployment.App {
			deployments[i] = deployment
			updated = true
			break
		}
	}

	if !updated {
		deployments = append(deployments, deployment)
	}

	return s.write(deployments)
}

func (s *JSONStore) Get(app string) (DeploymentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.load()
	if err != nil {
		return DeploymentRecord{}, err
	}

	for _, deployment := range deployments {
		if deployment.App == app {
			return deployment, nil
		}
	}

	return DeploymentRecord{}, ErrDeploymentNotFound
}

func (s *JSONStore) List() ([]DeploymentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load()
}

func (s *JSONStore) Delete(app string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.load()
	if err != nil {
		return err
	}

	filtered := make([]DeploymentRecord, 0, len(deployments))
	found := false

	for _, deployment := range deployments {
		if deployment.App == app {
			found = true
			continue
		}

		filtered = append(filtered, deployment)
	}

	if !found {
		return ErrDeploymentNotFound
	}

	return s.write(filtered)
}

func (s *JSONStore) load() ([]DeploymentRecord, error) {
	data, err := os.ReadFile(s.path)

	if errors.Is(err, os.ErrNotExist) {
		return []DeploymentRecord{}, nil
	}

	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return []DeploymentRecord{}, nil
	}

	var deployments []DeploymentRecord

	if err := json.Unmarshal(data, &deployments); err != nil {
		return nil, err
	}

	return deployments, nil
}

func (s *JSONStore) write(deployments []DeploymentRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(deployments, "", "  ")
	if err != nil {
		return err
	}

	tempPath := s.path + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tempPath, s.path)
}
