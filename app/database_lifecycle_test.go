package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func attachDatabaseToFullstackHarness(t *testing.T, harness *fullstackLifecycleHarness) *fakeMiniBaseClient {
	t.Helper()
	attachment := DatabaseAttachmentRecord{
		AttachmentID: "attachment_0123456789abcdef0123456789abcdef",
		DatabaseID:   "database_0123456789abcdef0123456789abcdef",
		DisplayName:  "Phase 6 Production",
		BindingName:  miniBaseBindingPrimary,
	}
	harness.old.DatabaseAttachments = []DatabaseAttachmentRecord{attachment}
	if err := harness.store.Save(harness.old); err != nil {
		t.Fatal(err)
	}
	client := &fakeMiniBaseClient{binding: miniBaseBinding{
		DatabaseID:    attachment.DatabaseID,
		Engine:        "postgresql",
		Host:          "minibase-postgres",
		Port:          5432,
		Database:      "mb_db_0123456789abcdef0123456789abcdef",
		Username:      "mb_role_0123456789abcdef0123456789abcdef",
		Password:      "attached-lifecycle-password-sentinel",
		DockerNetwork: reactorLabDataNetwork,
	}}
	replaceMiniBaseClientForTest(t, client)
	return client
}

func assertFullstackDatabaseRuntime(t *testing.T, harness *fullstackLifecycleHarness, record DeploymentRecord) {
	t.Helper()
	if len(record.DatabaseAttachments) != 1 || record.DatabaseAttachments[0] != harness.old.DatabaseAttachments[0] {
		t.Fatal("candidate did not preserve the current application-level attachment")
	}
	if len(harness.commands.dockerCreates) != 1 || len(harness.commands.dockerRuns) != 1 {
		t.Fatalf("database-enabled fullstack start did not use backend create plus frontend run: creates=%d runs=%d", len(harness.commands.dockerCreates), len(harness.commands.dockerRuns))
	}
	if len(harness.commands.networkConnects) != 1 || !slices.Equal(harness.commands.networkConnects[0], []string{"network", "connect", reactorLabDataNetwork, record.Services[1].Container}) {
		t.Fatalf("backend private network connections = %#v", harness.commands.networkConnects)
	}
	backendEnvironment := harness.commands.dockerEnv[fullstackBackendService]
	if !strings.Contains(backendEnvironment, "DATABASE_URL=") || !strings.Contains(backendEnvironment, "PORT=3000") {
		t.Fatal("backend did not receive managed DATABASE_URL and PORT through its secure env file")
	}
	if _, exists := harness.commands.dockerEnv[fullstackFrontendService]; exists {
		t.Fatal("frontend received a runtime environment file")
	}
	allArguments := ""
	for _, command := range append(slices.Clone(harness.commands.dockerRuns), harness.commands.dockerCreates...) {
		allArguments += strings.Join(command, " ")
	}
	if strings.Contains(allArguments, "attached-lifecycle-password-sentinel") || strings.Contains(allArguments, "DATABASE_URL=") {
		t.Fatal("managed database credential entered Docker command arguments")
	}
}

func TestAttachedDatabasePersistsAcrossFullstackRedeployAndHistory(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	attachDatabaseToFullstackHarness(t, harness)
	redeployed, err := safeRedeploy(harness.old, nil)
	if err != nil {
		t.Fatalf("attached safeRedeploy() error: %v", err)
	}
	assertFullstackDatabaseRuntime(t, harness, redeployed)
	versions, err := historyStore.List(harness.app)
	if err != nil || len(versions) != 1 {
		t.Fatalf("history = %#v, err=%v", versions, err)
	}
	historyJSON, err := os.ReadFile(historyStore.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(historyJSON), "databaseAttachments") {
		t.Fatal("release history persisted application-level attachment state")
	}
}

func TestAttachedDatabasePersistsAcrossWebhookRedeploy(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	attachDatabaseToFullstackHarness(t, harness)
	selected, ok := deploymentForWebhook([]DeploymentRecord{harness.old}, harness.repo)
	if !ok || len(selected.DatabaseAttachments) != 1 {
		t.Fatal("webhook selection lost current database attachment")
	}
	redeployed, err := safeRedeploy(selected, nil)
	if err != nil {
		t.Fatalf("attached webhook-style redeploy error: %v", err)
	}
	assertFullstackDatabaseRuntime(t, harness, redeployed)
}

func TestAttachedDatabasePersistsAcrossRollback(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	attachDatabaseToFullstackHarness(t, harness)
	previous := fullstackTestRecord(harness.app, "previous")
	previous.RepoURL = harness.repo
	if _, err := historyStore.Push(previous); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := rollbackDeployment(harness.old)
	if err != nil {
		t.Fatalf("attached rollback error: %v", err)
	}
	assertFullstackDatabaseRuntime(t, harness, rolledBack)
}

func TestFullstackMigrationFailurePreservesOldReleaseAndAttachment(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	attachDatabaseToFullstackHarness(t, harness)
	writeStrategyTestFile(t, filepath.Join(harness.repo, fullstackBackendService), "package.json", `{
  "scripts": {
    "start": "node server.js",
    "reactorlab:migrate": "node migrate.js"
  },
  "dependencies": {"express": "5.1.0"}
}`)
	for _, args := range [][]string{{"add", "backend/package.json"}, {"commit", "-m", "add migration fixture"}} {
		if output, err := executeCommand(harness.repo, "git", args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	baseRunner := harness.commands.run
	replaceCommandRunnerForTest(t, func(dir, name string, args ...string) (string, error) {
		if name == "docker" && len(args) > 0 && args[0] == "run" && strings.Contains(strings.Join(args, " "), "npm run reactorlab:migrate") {
			return "migration failed with attached-lifecycle-password-sentinel", errors.New("migration failed")
		}
		return baseRunner(dir, name, args...)
	})

	if _, err := safeRedeploy(harness.old, nil); err == nil {
		t.Fatal("expected reactorlab:migrate failure")
	}
	if harness.syncCount != 0 {
		t.Fatal("routing changed after migration failure")
	}
	harness.requireOldStillActive(t)
	persisted, err := harness.store.Get(harness.app)
	if err != nil || len(persisted.DatabaseAttachments) != 1 || persisted.DatabaseAttachments[0] != harness.old.DatabaseAttachments[0] {
		t.Fatal("migration failure lost the application-level database attachment")
	}
	logData, err := readDeploymentLog(harness.app)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logData, "attached-lifecycle-password-sentinel") || !strings.Contains(logData, redactedEnvironmentValue) {
		t.Fatal("migration failure log did not redact the managed database credential")
	}
}

func TestDeploymentDeleteDetachesRelationshipWithoutDatabaseDeletion(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	client := attachDatabaseToFullstackHarness(t, harness)
	if err := deleteFullstackProject(harness.old); err != nil {
		t.Fatalf("deleteFullstackProject() error: %v", err)
	}
	if !slices.Equal(client.deleted, []string{harness.old.DatabaseAttachments[0].AttachmentID}) {
		t.Fatalf("attachment deletion calls = %v", client.deleted)
	}
	if _, err := harness.store.Get(harness.app); err == nil {
		t.Fatal("deployment metadata remained after successful detach/delete")
	}
	credentialPath := filepath.Join(t.TempDir(), "database-credential-sentinel")
	if err := os.WriteFile(credentialPath, []byte("independently-managed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatal("deployment deletion affected an independent database credential")
	}
}

func TestNodeDetectorPersistsReactorLabMigrationDeclaration(t *testing.T) {
	repository := t.TempDir()
	writeStrategyTestFile(t, repository, "package.json", `{
  "scripts": {
    "start": "node server.js",
    "reactorlab:migrate": "node migrate.js"
  },
  "dependencies": {"express": "5.1.0"}
}`)
	writeStrategyTestFile(t, repository, "server.js", "// server\n")
	writeStrategyTestFile(t, repository, "migrate.js", "// migration\n")
	plan, err := detectDeploymentStrategy(repository, deploymentConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Strategy != deploymentStrategyNodeExpress || !plan.ReactorLabMigration {
		t.Fatal("reactorlab:migrate declaration was not captured in the build plan")
	}
	record := DeploymentRecord{
		App:                 "migration-app",
		RepoURL:             "https://github.com/example/migration-app.git",
		Strategy:            plan.Strategy,
		ContainerPort:       plan.ContainerPort,
		HealthPath:          plan.HealthPath,
		PackageManager:      plan.PackageManager,
		PackageInstallMode:  plan.PackageInstallMode,
		ReactorLabMigration: plan.ReactorLabMigration,
	}
	if !deploymentVersion(record).Record().ReactorLabMigration {
		t.Fatal("reactorlab:migrate declaration did not survive history serialization")
	}
}
