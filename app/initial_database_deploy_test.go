package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	initialDatabaseID   = "database_11111111111111111111111111111111"
	initialAttachmentID = "attachment_22222222222222222222222222222222"
)

type initialDeployCommandRecorder struct {
	commands             []string
	migrationEnvironment string
	backendEnvironment   string
	dataNetworkConnected bool
	removed              []string
}

func (recorder *initialDeployCommandRecorder) run(
	dir string,
	name string,
	args ...string,
) (string, error) {
	if name == "git" {
		return executeCommand(dir, name, args...)
	}
	if name != "docker" || len(args) == 0 {
		return "", nil
	}

	joined := strings.Join(args, " ")
	recorder.commands = append(recorder.commands, joined)
	if args[0] == "run" || args[0] == "create" {
		for index, argument := range args {
			if argument != "--env-file" || index+1 >= len(args) {
				continue
			}
			content, err := os.ReadFile(args[index+1])
			if err != nil {
				return "", err
			}
			if strings.Contains(joined, "npm run reactorlab:migrate") {
				recorder.migrationEnvironment = string(content)
			} else {
				recorder.backendEnvironment = string(content)
			}
		}
		return "container-id\n", nil
	}

	if args[0] == "network" {
		if len(args) >= 2 && args[1] == "inspect" {
			return "bridge true\n", nil
		}
		if len(args) >= 3 && args[1] == "connect" &&
			args[2] == reactorLabDataNetwork {
			recorder.dataNetworkConnected = true
		}
		return "", nil
	}
	if args[0] == "rm" || (args[0] == "image" && len(args) > 1 && args[1] == "rm") {
		recorder.removed = append(recorder.removed, args[len(args)-1])
	}
	if args[0] == "logs" {
		return "application output\n", nil
	}
	return "", nil
}

type initialDeployHarness struct {
	app       string
	repo      string
	metadata  *JSONStore
	client    *fakeMiniBaseClient
	commands  *initialDeployCommandRecorder
	healthErr error
	syncErr   error
	syncCalls int
}

type initialDeleteFailingStore struct {
	DeploymentStore
}

func (store initialDeleteFailingStore) Delete(string) error {
	return errors.New("injected metadata delete failure")
}

func newInitialDeployHarness(t *testing.T) *initialDeployHarness {
	t.Helper()
	root := t.TempDir()
	app := "initial-database-app"
	repository := filepath.Join(root, app)
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStrategyTestFile(t, repository, "package.json", `{
  "scripts": {
    "start": "node server.js",
    "reactorlab:migrate": "node migrate.js"
  },
  "dependencies": {"express": "5.1.0"}
}`)
	writeStrategyTestFile(t, repository, "server.js", "// server fixture\n")
	writeStrategyTestFile(t, repository, "migrate.js", "// migration fixture\n")
	writePackageLock(t, repository)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "MiniDeploy Test"},
		{"add", "."},
		{"commit", "-m", "fixture"},
	} {
		if output, err := executeCommand(repository, "git", args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}

	previousDeploymentsDir := deploymentsDir
	deploymentsDir = filepath.Join(root, "managed-deployments")
	t.Cleanup(func() { deploymentsDir = previousDeploymentsDir })

	previousLogsDir := deploymentLogsDir
	deploymentLogsDir = filepath.Join(root, "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousLogsDir })

	previousRuntimeStore := runtimeEnvironmentStore
	runtimeEnvironmentStore = newRuntimeEnvironmentFileStore(
		filepath.Join(root, "secrets"),
	)
	t.Cleanup(func() { runtimeEnvironmentStore = previousRuntimeStore })

	metadata := NewJSONStore(filepath.Join(root, "deployments.json"))
	previousStore := store
	store = metadata
	t.Cleanup(func() { store = previousStore })

	previousHistory := historyStore
	historyStore = NewJSONHistoryStore(filepath.Join(root, "history.json"), 3)
	t.Cleanup(func() { historyStore = previousHistory })

	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          initialDatabaseID,
			DisplayName: "Initial Database",
			Status:      "ready",
			Attached:    false,
		}},
		attachment: miniBaseAttachment{
			ID:           initialAttachmentID,
			DatabaseID:   initialDatabaseID,
			ConsumerType: "minideploy",
			ConsumerRef:  app,
			BindingName:  miniBaseBindingPrimary,
		},
		binding: miniBaseBinding{
			DatabaseID:    initialDatabaseID,
			Engine:        "postgresql",
			Host:          "minibase-postgres",
			Port:          5432,
			Database:      "mb_db_33333333333333333333333333333333",
			Username:      "mb_role_44444444444444444444444444444444",
			Password:      "initial-database-password-sentinel",
			DockerNetwork: reactorLabDataNetwork,
		},
	}
	replaceMiniBaseClientForTest(t, client)

	commands := &initialDeployCommandRecorder{}
	replaceCommandRunnerForTest(t, commands.run)
	harness := &initialDeployHarness{
		app:      app,
		repo:     repository,
		metadata: metadata,
		client:   client,
		commands: commands,
	}

	previousBuild := initialBuildDeploymentImage
	previousPort := initialFindAvailablePort
	previousStartup := initialVerifyStartup
	previousHealth := initialVerifyHTTPHealth
	previousSync := initialSynchronizeProxy
	t.Cleanup(func() {
		initialBuildDeploymentImage = previousBuild
		initialFindAvailablePort = previousPort
		initialVerifyStartup = previousStartup
		initialVerifyHTTPHealth = previousHealth
		initialSynchronizeProxy = previousSync
	})
	initialBuildDeploymentImage = func(
		string,
		string,
		deploymentBuildPlan,
	) (string, error) {
		return "built\n", nil
	}
	initialFindAvailablePort = func(int, int) (int, error) {
		return 8765, nil
	}
	initialVerifyStartup = func(string) error { return nil }
	initialVerifyHTTPHealth = func(int, string) error {
		return harness.healthErr
	}
	initialSynchronizeProxy = func() error {
		harness.syncCalls++
		err := harness.syncErr
		if err != nil {
			harness.syncErr = nil
		}
		return err
	}
	return harness
}

func TestInitialDeployWithoutDatabasePreservesExistingBehavior(t *testing.T) {
	harness := newInitialDeployHarness(t)
	harness.client.err = errors.New("MiniBase must not be contacted")
	record, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		"",
	)
	if err != nil {
		t.Fatalf("deployRepository() without database error: %v", err)
	}
	if len(record.DatabaseAttachments) != 0 {
		t.Fatal("deployment without database stored attachment metadata")
	}
	if harness.client.listCalls != 0 ||
		harness.client.attachmentCalls != 0 ||
		harness.client.bindingCalls != 0 {
		t.Fatal("deployment without database contacted MiniBase")
	}
	if harness.commands.migrationEnvironment != "" {
		t.Fatal("database migration behavior changed for an unattached initial deployment")
	}
}

func TestInitialNodeDeployAttachesExistingDatabaseAndUsesManagedRuntime(t *testing.T) {
	harness := newInitialDeployHarness(t)
	record, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err != nil {
		t.Fatalf("deployRepository() error: %v", err)
	}
	if record.Strategy != deploymentStrategyNodeExpress ||
		len(record.DatabaseAttachments) != 1 ||
		record.DatabaseAttachments[0].AttachmentID != initialAttachmentID ||
		record.DatabaseAttachments[0].DatabaseID != initialDatabaseID ||
		record.DatabaseAttachments[0].DisplayName != "Initial Database" ||
		record.DatabaseAttachments[0].BindingName != miniBaseBindingPrimary {
		t.Fatal("successful initial deployment did not retain safe attachment metadata")
	}
	if harness.client.listCalls != 1 ||
		harness.client.attachmentCalls != 1 ||
		harness.client.bindingCalls != 1 ||
		harness.client.consumerRef != harness.app {
		t.Fatal("initial deployment used an unexpected MiniBase lifecycle")
	}
	for _, environment := range []string{
		harness.commands.migrationEnvironment,
		harness.commands.backendEnvironment,
	} {
		if !strings.Contains(environment, "DATABASE_URL=") ||
			!strings.Contains(environment, "initial-database-password-sentinel") {
			t.Fatal("managed DATABASE_URL did not reach migration and backend env files")
		}
	}
	if !strings.Contains(harness.commands.backendEnvironment, "PORT=3000") ||
		!harness.commands.dataNetworkConnected ||
		harness.syncCalls != 1 {
		t.Fatal("backend did not receive the managed port and private data network")
	}
	allArguments := strings.Join(harness.commands.commands, "\n")
	if strings.Contains(allArguments, "initial-database-password-sentinel") ||
		strings.Contains(allArguments, "DATABASE_URL=") {
		t.Fatal("managed database credential appeared in command arguments")
	}

	persisted, err := harness.metadata.Get(harness.app)
	if err != nil || len(persisted.DatabaseAttachments) != 1 {
		t.Fatal("safe initial attachment metadata was not persisted")
	}
	metadataContent, err := os.ReadFile(harness.metadata.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(metadataContent), "initial-database-password-sentinel") ||
		strings.Contains(string(metadataContent), "DATABASE_URL") {
		t.Fatal("deployment metadata persisted managed database credentials")
	}
	environment, err := runtimeEnvironmentStore.Load(harness.app)
	if err != nil {
		t.Fatal(err)
	}
	if environment["JWT_SECRET"] != "application-secret" {
		t.Fatal("application environment was not persisted")
	}
	if _, exists := environment["DATABASE_URL"]; exists {
		t.Fatal("managed DATABASE_URL was persisted in the application secret file")
	}
}

func TestInitialDatabaseValidationRejectsUnsupportedAndUnavailable(t *testing.T) {
	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          initialDatabaseID,
			DisplayName: "Unavailable",
			Status:      "ready",
			Attached:    true,
		}},
	}
	replaceMiniBaseClientForTest(t, client)

	if _, err := createInitialMiniBaseAttachment(
		"static-app",
		initialDatabaseID,
		deploymentBuildPlan{Strategy: deploymentStrategyViteStatic},
	); !errors.Is(err, ErrInitialDatabaseUnsupported) {
		t.Fatalf("unsupported strategy error = %v", err)
	}
	if client.listCalls != 0 {
		t.Fatal("unsupported strategy contacted MiniBase before rejection")
	}

	if _, err := createInitialMiniBaseAttachment(
		"node-app",
		initialDatabaseID,
		deploymentBuildPlan{Strategy: deploymentStrategyNodeExpress},
	); !errors.Is(err, ErrMiniBaseDatabaseUnavailable) {
		t.Fatalf("attached database error = %v", err)
	}
	if client.listCalls != 1 || client.attachmentCalls != 0 {
		t.Fatal("unavailable database was attached")
	}
}

func TestInitialDeployRejectsUserDatabaseURLWithDatabaseID(t *testing.T) {
	restoreStore := replaceStoreForTest(t, staticDeploymentStore{})
	defer restoreStore()
	client := &fakeMiniBaseClient{}
	replaceMiniBaseClientForTest(t, client)
	request := httptest.NewRequest(
		http.MethodPost,
		"/deploy",
		strings.NewReader(`{
  "repoUrl":"https://github.com/example/conflict.git",
  "databaseId":"`+initialDatabaseID+`",
  "environment":{"DATABASE_URL":"user-controlled"}
}`),
	)
	response := httptest.NewRecorder()
	deployHandler(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("DATABASE_URL conflict status = %d", response.Code)
	}
	if client.listCalls != 0 || client.attachmentCalls != 0 {
		t.Fatal("DATABASE_URL conflict contacted MiniBase")
	}
}

func TestFailedInitialDeployDetachesOnlyAttachmentAndPreservesDatabase(t *testing.T) {
	harness := newInitialDeployHarness(t)
	harness.healthErr = errors.New("candidate health failed")
	_, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err == nil {
		t.Fatal("expected initial deployment failure")
	}
	if len(harness.client.deleted) != 1 ||
		harness.client.deleted[0] != initialAttachmentID {
		t.Fatal("failed deployment did not remove exactly the new attachment")
	}
	if len(harness.client.databases) != 1 ||
		harness.client.databases[0].ID != initialDatabaseID {
		t.Fatal("failed deployment modified the independent database")
	}
	if _, err := harness.metadata.Get(harness.app); !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatal("failed initial deployment persisted deployment metadata")
	}
}

func TestInitialProxyFailureRollsBackDeploymentAndAttachment(t *testing.T) {
	harness := newInitialDeployHarness(t)
	harness.syncErr = errors.New("proxy sync failed")
	_, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err == nil {
		t.Fatal("expected initial proxy synchronization failure")
	}
	if harness.syncCalls != 2 {
		t.Fatalf("proxy synchronization calls = %d; want failed cutover and restore", harness.syncCalls)
	}
	if len(harness.client.deleted) != 1 ||
		harness.client.deleted[0] != initialAttachmentID {
		t.Fatal("proxy failure did not detach the initial database attachment")
	}
	if _, err := harness.metadata.Get(harness.app); !errors.Is(err, ErrDeploymentNotFound) {
		t.Fatal("proxy failure left deployment metadata active")
	}
	if len(harness.commands.removed) < 2 {
		t.Fatal("proxy failure did not remove the candidate container and image")
	}
}

func TestInitialAttachmentCleanupFailureIsSafeAndRecoverable(t *testing.T) {
	harness := newInitialDeployHarness(t)
	harness.healthErr = errors.New("candidate health failed")
	harness.client.deleteErr = errors.New("cleanup-secret-sentinel")
	_, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err == nil || strings.Contains(err.Error(), "cleanup-secret-sentinel") {
		t.Fatal("attachment cleanup failure was absent or exposed unsafe details")
	}
	logContent, readErr := readDeploymentLog(harness.app)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(logContent, "cleanup-secret-sentinel") ||
		!strings.Contains(logContent, initialAttachmentID) ||
		!strings.Contains(logContent, initialDatabaseID) {
		t.Fatal("cleanup failure did not retain only safe recovery metadata")
	}
}

func TestInitialFullstackDatabaseReachesBackendOnly(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	if err := harness.store.Delete(harness.app); err != nil {
		t.Fatal(err)
	}
	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          initialDatabaseID,
			DisplayName: "Full-stack Database",
			Status:      "ready",
		}},
		attachment: miniBaseAttachment{
			ID:           initialAttachmentID,
			DatabaseID:   initialDatabaseID,
			ConsumerType: "minideploy",
			ConsumerRef:  harness.app,
			BindingName:  miniBaseBindingPrimary,
		},
		binding: miniBaseBinding{
			DatabaseID:    initialDatabaseID,
			Engine:        "postgresql",
			Host:          "minibase-postgres",
			Port:          5432,
			Database:      "mb_db_33333333333333333333333333333333",
			Username:      "mb_role_44444444444444444444444444444444",
			Password:      "fullstack-password-sentinel",
			DockerNetwork: reactorLabDataNetwork,
		},
	}
	replaceMiniBaseClientForTest(t, client)

	record, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err != nil {
		t.Fatalf("full-stack initial deployment error: %v", err)
	}
	if record.Strategy != deploymentStrategyFullstackViteNode ||
		len(record.DatabaseAttachments) != 1 {
		t.Fatal("full-stack initial deployment lost attachment metadata")
	}
	backendEnvironment := harness.commands.dockerEnv[fullstackBackendService]
	frontendEnvironment := harness.commands.dockerEnv[fullstackFrontendService]
	if !strings.Contains(backendEnvironment, "DATABASE_URL=") ||
		!strings.Contains(backendEnvironment, "fullstack-password-sentinel") {
		t.Fatal("full-stack backend did not receive managed DATABASE_URL")
	}
	if strings.Contains(frontendEnvironment, "DATABASE_URL") ||
		strings.Contains(frontendEnvironment, "fullstack-password-sentinel") {
		t.Fatal("full-stack frontend received managed database credentials")
	}
	dataConnects := 0
	for _, args := range harness.commands.networkConnects {
		if len(args) >= 4 && args[2] == reactorLabDataNetwork &&
			strings.Contains(args[3], fullstackBackendService) {
			dataConnects++
		}
	}
	if dataConnects != 1 {
		t.Fatalf("backend private data-network connections = %d; want 1", dataConnects)
	}
}

func TestInitialFullstackRollbackRetainsAttachmentWhenMetadataRemains(t *testing.T) {
	harness := newFullstackLifecycleHarness(t)
	if err := harness.store.Delete(harness.app); err != nil {
		t.Fatal(err)
	}
	client := &fakeMiniBaseClient{
		databases: []miniBaseDatabase{{
			ID:          initialDatabaseID,
			DisplayName: "Full-stack Database",
			Status:      "ready",
		}},
		attachment: miniBaseAttachment{
			ID:           initialAttachmentID,
			DatabaseID:   initialDatabaseID,
			ConsumerType: "minideploy",
			ConsumerRef:  harness.app,
			BindingName:  miniBaseBindingPrimary,
		},
		binding: miniBaseBinding{
			DatabaseID:    initialDatabaseID,
			Engine:        "postgresql",
			Host:          "minibase-postgres",
			Port:          5432,
			Database:      "mb_db_33333333333333333333333333333333",
			Username:      "mb_role_44444444444444444444444444444444",
			Password:      "fullstack-password-sentinel",
			DockerNetwork: reactorLabDataNetwork,
		},
	}
	replaceMiniBaseClientForTest(t, client)
	store = initialDeleteFailingStore{DeploymentStore: harness.store}
	fullstackSynchronizeProxyRoutes = func() error {
		return errors.New("injected proxy failure")
	}

	_, err := deployRepository(
		harness.repo,
		0,
		"",
		map[string]string{"JWT_SECRET": "application-secret"},
		initialDatabaseID,
	)
	if err == nil {
		t.Fatal("expected full-stack proxy rollback failure")
	}
	if len(client.deleted) != 0 {
		t.Fatal("attachment was deleted while deployment metadata remained")
	}
	persisted, getErr := harness.store.Get(harness.app)
	if getErr != nil || len(persisted.DatabaseAttachments) != 1 ||
		persisted.DatabaseAttachments[0].AttachmentID != initialAttachmentID {

		t.Fatal("safe metadata and attachment were not retained together for recovery")
	}
}
