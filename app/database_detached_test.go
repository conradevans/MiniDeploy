package main

import (
	"errors"
	"strings"
	"testing"
)

func TestDatabaseDetachedStatusDoesNotDependOnContainers(t *testing.T) {
	record := DeploymentRecord{
		App:              "scheduler",
		Strategy:         deploymentStrategyFullstackViteNode,
		DatabaseDetached: true,
	}

	if status := deploymentProjectStatus(record); status != "database-detached" {
		t.Fatalf("status = %q; want database-detached", status)
	}
}

func TestDatabaseDetachedProxyUsesControlledUnavailableResponse(t *testing.T) {
	record := DeploymentRecord{
		App:              "scheduler",
		DatabaseDetached: true,
	}

	local, public := detachedProxyRouteFragments(
		record,
		3,
		"scheduler.reactorlab.dev",
	)

	for name, fragment := range map[string]string{
		"local":  local,
		"public": public,
	} {
		if !strings.Contains(fragment, databaseDetachedMessage) {
			t.Fatalf("%s route does not contain detached message: %s", name, fragment)
		}
		if !strings.Contains(fragment, "503") {
			t.Fatalf("%s route does not return 503: %s", name, fragment)
		}
		if strings.Contains(fragment, "reverse_proxy") {
			t.Fatalf("%s detached route still reverse proxies: %s", name, fragment)
		}
	}

	if !strings.Contains(local, "handle_path /scheduler/*") {
		t.Fatalf("local detached route is incorrect: %s", local)
	}
	if !strings.Contains(public, "host scheduler.reactorlab.dev") {
		t.Fatalf("public detached route is incorrect: %s", public)
	}
}

func TestDatabaseDetachedBlocksRedeployAndRollback(t *testing.T) {
	record := DeploymentRecord{
		App:              "scheduler",
		DatabaseDetached: true,
	}

	if _, err := safeRedeploy(record, nil); !errors.Is(err, ErrDatabaseDetached) {
		t.Fatalf("safeRedeploy() error = %v; want ErrDatabaseDetached", err)
	}

	if _, err := rollbackDeployment(record); !errors.Is(err, ErrDatabaseDetached) {
		t.Fatalf("rollbackDeployment() error = %v; want ErrDatabaseDetached", err)
	}
}
