package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fullstackFrontendService = "frontend"
	fullstackBackendService  = "backend"
)

type fullstackViteNodeStrategyDetector struct{}

func (fullstackViteNodeStrategyDetector) Detect(
	repositoryPath string,
	requested deploymentConfig,
) (deploymentBuildPlan, bool, error) {
	frontendPath, found, err := resolvedRepositoryServiceDirectory(
		repositoryPath,
		fullstackFrontendService,
	)
	if err != nil || !found {
		return deploymentBuildPlan{}, false, err
	}

	backendPath, found, err := resolvedRepositoryServiceDirectory(
		repositoryPath,
		fullstackBackendService,
	)
	if err != nil || !found {
		return deploymentBuildPlan{}, false, err
	}

	frontendPlan, detected, err := (viteStaticStrategyDetector{}).Detect(
		frontendPath,
		deploymentConfig{},
	)
	if err != nil {
		return deploymentBuildPlan{}, false, fmt.Errorf(
			"inspect frontend service: %w",
			err,
		)
	}
	if !detected {
		return deploymentBuildPlan{}, false, nil
	}

	backendPlan, detected, err := (nodeExpressStrategyDetector{}).Detect(
		backendPath,
		requested,
	)
	if err != nil {
		return deploymentBuildPlan{}, false, fmt.Errorf(
			"inspect backend service: %w",
			err,
		)
	}
	if !detected {
		return deploymentBuildPlan{}, false, nil
	}

	return deploymentBuildPlan{
		Strategy: deploymentStrategyFullstackViteNode,
		Services: []deploymentServiceBuildPlan{
			{
				Name:           fullstackFrontendService,
				Path:           fullstackFrontendService,
				RepositoryPath: frontendPath,
				Build:          frontendPlan,
			},
			{
				Name:           fullstackBackendService,
				Path:           fullstackBackendService,
				RepositoryPath: backendPath,
				Build:          backendPlan,
			},
		},
	}, true, nil
}

// resolvedRepositoryServiceDirectory accepts only one fixed service directory
// directly beneath a checkout. Both its lexical path and symlink-resolved target
// must be strict descendants of the checkout root.
func resolvedRepositoryServiceDirectory(
	repositoryPath string,
	serviceName string,
) (string, bool, error) {
	if serviceName != fullstackFrontendService &&
		serviceName != fullstackBackendService {

		return "", false, fmt.Errorf(
			"unsupported full-stack service %q",
			serviceName,
		)
	}

	cleanRoot, err := filepath.Abs(filepath.Clean(repositoryPath))
	if err != nil {
		return "", false, fmt.Errorf("resolve repository root: %w", err)
	}

	candidate, err := strictChildPath(
		cleanRoot,
		filepath.Join(cleanRoot, serviceName),
	)
	if err != nil {
		return "", false, err
	}

	_, err = os.Lstat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf(
			"inspect %s service path: %w",
			serviceName,
			err,
		)
	}

	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", false, fmt.Errorf(
			"resolve repository root symlinks: %w",
			err,
		)
	}

	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false, fmt.Errorf(
			"resolve %s service symlinks: %w",
			serviceName,
			err,
		)
	}

	resolvedCandidate, err = strictChildPath(
		resolvedRoot,
		resolvedCandidate,
	)
	if err != nil {
		return "", false, fmt.Errorf(
			"%s service escapes repository: %w",
			serviceName,
			err,
		)
	}

	info, err := os.Stat(resolvedCandidate)
	if err != nil {
		return "", false, fmt.Errorf(
			"inspect resolved %s service: %w",
			serviceName,
			err,
		)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf(
			"%s service path is not a directory",
			serviceName,
		)
	}

	return resolvedCandidate, true, nil
}

func persistedFullstackBuildPlan(
	record DeploymentRecord,
	repositoryPath string,
) (deploymentBuildPlan, error) {
	_, dockerfile, err := (dockerfileStrategyDetector{}).Detect(
		repositoryPath,
		deploymentConfig{},
	)
	if err != nil {
		return deploymentBuildPlan{}, err
	}
	if dockerfile {
		return deploymentBuildPlan{}, fmt.Errorf(
			"persisted full-stack strategy cannot override a root Dockerfile",
		)
	}
	if err := validateFullstackServiceMetadata(record.Services); err != nil {
		return deploymentBuildPlan{}, err
	}

	services := make([]deploymentServiceBuildPlan, 0, 2)
	for _, serviceName := range []string{
		fullstackFrontendService,
		fullstackBackendService,
	} {
		service, _ := deploymentServiceByName(record, serviceName)
		servicePath, found, err := resolvedRepositoryServiceDirectory(
			repositoryPath,
			serviceName,
		)
		if err != nil {
			return deploymentBuildPlan{}, err
		}
		if !found {
			return deploymentBuildPlan{}, fmt.Errorf(
				"persisted full-stack strategy requires %s/",
				serviceName,
			)
		}

		serviceRecord := DeploymentRecord{
			App:                record.App,
			Container:          service.Container,
			Image:              service.Image,
			Port:               service.Port,
			ContainerPort:      service.ContainerPort,
			HealthPath:         service.HealthPath,
			Strategy:           service.Strategy,
			PackageManager:     service.PackageManager,
			PackageInstallMode: service.PackageInstallMode,
		}
		servicePlan, err := deploymentStrategyForRecord(
			serviceRecord,
			servicePath,
		)
		if err != nil {
			return deploymentBuildPlan{}, fmt.Errorf(
				"prepare %s service: %w",
				serviceName,
				err,
			)
		}

		services = append(services, deploymentServiceBuildPlan{
			Name:           serviceName,
			Path:           serviceName,
			RepositoryPath: servicePath,
			Build:          servicePlan,
		})
	}

	return deploymentBuildPlan{
		Strategy: deploymentStrategyFullstackViteNode,
		Services: services,
	}, nil
}

func validateFullstackServiceMetadata(
	services []DeploymentServiceRecord,
) error {
	if len(services) != 2 {
		return fmt.Errorf("full-stack project must contain exactly two services")
	}

	expected := map[string]string{
		fullstackFrontendService: deploymentStrategyViteStatic,
		fullstackBackendService:  deploymentStrategyNodeExpress,
	}
	seen := make(map[string]bool, 2)

	for _, service := range services {
		strategy, ok := expected[service.Name]
		if !ok || seen[service.Name] {
			return fmt.Errorf(
				"invalid full-stack service %q",
				service.Name,
			)
		}
		if service.Path != service.Name {
			return fmt.Errorf(
				"full-stack service %s must use fixed path %s/",
				service.Name,
				service.Name,
			)
		}
		if service.Strategy != strategy {
			return fmt.Errorf(
				"full-stack service %s has invalid strategy %q",
				service.Name,
				service.Strategy,
			)
		}
		seen[service.Name] = true
	}

	return nil
}

func fullstackBuildServiceByName(
	plan deploymentBuildPlan,
	name string,
) (deploymentServiceBuildPlan, bool) {
	for _, service := range plan.Services {
		if service.Name == name {
			return service, true
		}
	}

	return deploymentServiceBuildPlan{}, false
}

func fullstackServiceDisplayName(name string) string {
	if name == "" {
		return "Service"
	}

	return strings.ToUpper(name[:1]) + name[1:]
}
