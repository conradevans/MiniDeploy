package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInitialSingleServicePersistsCompleteProvenance(t *testing.T) {
	harness := newInitialDeployHarness(t)
	record, err := deployRepository(harness.repo, 0, "", nil, "")
	if err != nil {
		t.Fatalf("deployRepository() error: %v", err)
	}
	if record.Source == nil || record.Source.CommitSHA == "" ||
		record.Source.Branch != "main" || record.ActivatedAt == nil ||
		record.ImageID != "sha256:"+strings.Repeat("a", 64) {

		t.Fatalf("initial provenance = %#v", record)
	}
	persisted, err := harness.metadata.Get(harness.app)
	if err != nil || persisted.Source == nil ||
		persisted.Source.CommitSHA != record.Source.CommitSHA ||
		persisted.ActivatedAt == nil ||
		!persisted.ActivatedAt.Equal(*record.ActivatedAt) ||
		persisted.ImageID != record.ImageID {

		t.Fatalf("persisted initial provenance = %#v, %v", persisted, err)
	}
}

func TestInitialFullstackPersistsSourceActivationAndServiceImageIDs(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	if err := harness.store.Delete(harness.app); err != nil {
		t.Fatal(err)
	}

	record, err := deployRepository(harness.repo, 0, "", nil, "")
	if err != nil {
		t.Fatalf("deployRepository() error: %v", err)
	}
	if record.Source == nil || record.Source.CommitSHA == "" ||
		record.Source.Branch != "main" || record.ActivatedAt == nil {

		t.Fatalf("initial full-stack provenance = %#v", record)
	}
	for _, service := range record.Services {
		if service.ImageID != "sha256:"+strings.Repeat("b", 64) {
			t.Fatalf("%s image ID = %q", service.Name, service.ImageID)
		}
	}
	admin := deploymentResponse(record)
	for _, service := range admin.Services {
		if service.ImageID == "" {
			t.Fatalf("private %s service response omitted image ID", service.Name)
		}
	}
}

func TestFullstackRedeployUsesNewCheckoutIdentityAndArchivesOldProvenance(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	oldActivation := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	harness.old.Source = &DeploymentSourceRecord{
		Provider:   "github",
		Repository: "example/old",
		Branch:     "main",
		CommitSHA:  strings.Repeat("f", 40),
	}
	harness.old.ActivatedAt = &oldActivation
	harness.old.ImageID = "sha256:" + strings.Repeat("e", 64)
	for index := range harness.old.Services {
		harness.old.Services[index].ImageID = "sha256:" + strings.Repeat("e", 64)
	}
	harness.old = normalizeDeploymentRecord(harness.old)

	redeployed, err := safeRedeploy(harness.old, nil)
	if err != nil {
		t.Fatalf("safeRedeploy() error: %v", err)
	}
	if redeployed.Source == nil || redeployed.Source.CommitSHA == harness.old.Source.CommitSHA ||
		redeployed.Source.Branch != "main" || redeployed.ActivatedAt == nil ||
		!redeployed.ActivatedAt.After(oldActivation) {

		t.Fatalf("candidate provenance = %#v", redeployed)
	}
	for _, service := range redeployed.Services {
		if service.ImageID == "" {
			t.Fatalf("candidate %s image ID is missing", service.Name)
		}
	}

	versions, err := historyStore.List(harness.app)
	if err != nil || len(versions) != 1 || versions[0].Source == nil ||
		versions[0].Source.CommitSHA != harness.old.Source.CommitSHA ||
		versions[0].ActivatedAt == nil ||
		!versions[0].ActivatedAt.Equal(oldActivation) ||
		versions[0].ImageID != harness.old.ImageID {

		t.Fatalf("archived old provenance = %#v, %v", versions, err)
	}
}

func TestFullstackCutoverFailureRestoresOldSourceAndActivation(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	oldActivation := time.Date(2026, 9, 20, 11, 0, 0, 0, time.UTC)
	harness.old.Source = &DeploymentSourceRecord{
		Branch:    "main",
		CommitSHA: strings.Repeat("d", 40),
	}
	harness.old.ActivatedAt = &oldActivation
	if err := harness.store.Save(harness.old); err != nil {
		t.Fatal(err)
	}
	harness.syncCount = 0
	fullstackSynchronizeProxyRoutes = func() error {
		harness.syncCount++
		if harness.syncCount == 1 {
			return errors.New("injected cutover failure")
		}
		return nil
	}

	if _, err := safeRedeploy(harness.old, nil); err == nil {
		t.Fatal("expected cutover failure")
	}
	persisted, err := harness.store.Get(harness.app)
	if err != nil || persisted.Source == nil ||
		persisted.Source.CommitSHA != harness.old.Source.CommitSHA ||
		persisted.ActivatedAt == nil ||
		!persisted.ActivatedAt.Equal(oldActivation) {

		t.Fatalf("restored old provenance = %#v, %v", persisted, err)
	}
}

func TestFullstackRollbackPreservesSelectedSourceWithNewActivation(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	currentActivation := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	harness.old.Source = &DeploymentSourceRecord{CommitSHA: strings.Repeat("c", 40)}
	harness.old.ActivatedAt = &currentActivation

	previousActivation := currentActivation.Add(-time.Hour)
	previous := fullstackTestRecord(harness.app, "previous-identity")
	previous.RepoURL = harness.repo
	previous.Source = &DeploymentSourceRecord{
		Branch:    "main",
		CommitSHA: strings.Repeat("b", 40),
	}
	previous.ActivatedAt = &previousActivation
	for index := range previous.Services {
		previous.Services[index].ImageID = "sha256:" + strings.Repeat("b", 64)
	}
	previous = normalizeDeploymentRecord(previous)
	if _, err := historyStore.Push(previous); err != nil {
		t.Fatal(err)
	}

	rollbackActivation := currentActivation.Add(time.Hour)
	previousClock := deploymentActivationTime
	deploymentActivationTime = func() time.Time { return rollbackActivation }
	t.Cleanup(func() { deploymentActivationTime = previousClock })

	rolledBack, err := rollbackDeployment(harness.old)
	if err != nil {
		t.Fatalf("rollbackDeployment() error: %v", err)
	}
	if rolledBack.Source == nil ||
		rolledBack.Source.CommitSHA != previous.Source.CommitSHA ||
		rolledBack.ActivatedAt == nil ||
		!rolledBack.ActivatedAt.Equal(rollbackActivation) {

		t.Fatalf("rollback provenance = %#v", rolledBack)
	}
	for _, service := range rolledBack.Services {
		if service.ImageID == "" {
			t.Fatalf("rollback lost %s image ID", service.Name)
		}
	}

	versions, err := historyStore.List(harness.app)
	if err != nil || len(versions) != 1 || versions[0].Source == nil ||
		versions[0].Source.CommitSHA != harness.old.Source.CommitSHA ||
		versions[0].ActivatedAt == nil ||
		!versions[0].ActivatedAt.Equal(currentActivation) {

		t.Fatalf("post-rollback history provenance = %#v, %v", versions, err)
	}
}

func TestImageIDInspectionFailurePreventsPromotion(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	replaceCommandRunnerForTest(
		t,
		func(dir string, name string, args ...string) (string, error) {
			if name == "docker" && len(args) >= 3 &&
				args[0] == "image" && args[1] == "inspect" &&
				args[2] == "--format" {

				return "", errors.New("injected image identity failure")
			}
			return harness.commands.run(dir, name, args...)
		},
	)

	if _, err := safeRedeploy(harness.old, nil); err == nil {
		t.Fatal("expected image identity inspection failure")
	}
	harness.requireOldStillActive(t)
	if harness.syncCount != 0 {
		t.Fatal("proxy cutover occurred after image identity failure")
	}
	versions, err := historyStore.List(harness.app)
	if err != nil || len(versions) != 0 {
		t.Fatalf("failed candidate entered history: %#v, %v", versions, err)
	}
}
