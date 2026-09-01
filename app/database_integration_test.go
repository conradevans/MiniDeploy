package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fakeMiniBaseClient struct {
	databases       []miniBaseDatabase
	createdDatabase miniBaseDatabase
	attachment      miniBaseAttachment
	binding         miniBaseBinding
	err             error
	deleteErr       error
	deleted         []string
	createdName     string
	attachedDB      string
	consumerRef     string
	onCreateAttach  func()
}

func (client *fakeMiniBaseClient) ListDatabases(context.Context) ([]miniBaseDatabase, error) {
	return client.databases, client.err
}

func (client *fakeMiniBaseClient) CreateDatabase(_ context.Context, displayName string) (miniBaseDatabase, error) {
	client.createdName = displayName
	return client.createdDatabase, client.err
}

func (client *fakeMiniBaseClient) CreateAttachment(_ context.Context, databaseID, consumerRef string) (miniBaseAttachment, error) {
	client.attachedDB = databaseID
	client.consumerRef = consumerRef
	if client.onCreateAttach != nil {
		client.onCreateAttach()
	}
	return client.attachment, client.err
}

func (client *fakeMiniBaseClient) DeleteAttachment(_ context.Context, id string) error {
	client.deleted = append(client.deleted, id)
	if client.deleteErr != nil {
		return client.deleteErr
	}
	return client.err
}

func (client *fakeMiniBaseClient) ResolveBinding(context.Context, string) (miniBaseBinding, error) {
	return client.binding, client.err
}

func attachedNodeRecord() DeploymentRecord {
	return normalizeDeploymentRecord(DeploymentRecord{
		App:                 "scheduler",
		RepoURL:             "https://github.com/example/scheduler.git",
		Container:           "minideploy-scheduler",
		Image:               "minideploy-scheduler:latest",
		Port:                8081,
		ContainerPort:       3000,
		HealthPath:          "/",
		Strategy:            deploymentStrategyNodeExpress,
		PackageManager:      packageManagerNPM,
		ReactorLabMigration: true,
		DatabaseAttachments: []DatabaseAttachmentRecord{{
			AttachmentID: "attachment_0123456789abcdef0123456789abcdef",
			DatabaseID:   "database_0123456789abcdef0123456789abcdef",
			DisplayName:  "Scheduler Production",
			BindingName:  miniBaseBindingPrimary,
		}},
	})
}

func replaceMiniBaseClientForTest(t *testing.T, replacement miniBaseAPI) {
	t.Helper()
	previous := miniBaseClient
	miniBaseClient = replacement
	t.Cleanup(func() { miniBaseClient = previous })
}

func TestDatabaseAttachmentCompatibilityPersistenceAndHistoryIsolation(t *testing.T) {
	var legacy DeploymentRecord
	if err := json.Unmarshal([]byte(`{"app":"legacy","repoUrl":"https://github.com/example/legacy.git","container":"legacy","image":"legacy:latest","port":8080,"containerPort":8080,"healthPath":"/"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy.DatabaseAttachments) != 0 || normalizeDeploymentRecord(legacy).Strategy != deploymentStrategyDockerfile {
		t.Fatal("legacy deployment record compatibility regressed")
	}

	record := attachedNodeRecord()
	store := NewJSONStore(filepath.Join(t.TempDir(), "deployments.json"))
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(record.App)
	if err != nil || len(loaded.DatabaseAttachments) != 1 || loaded.DatabaseAttachments[0] != record.DatabaseAttachments[0] {
		t.Fatalf("safe database attachment did not survive persistence: err=%v", err)
	}
	encoded, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"postgresql://", "password", "credentialPath", "mb_role_"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("deployment metadata contains forbidden managed-secret marker %q", forbidden)
		}
	}

	version := deploymentVersion(record)
	versionJSON, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(versionJSON), "databaseAttachments") || len(version.Record().DatabaseAttachments) != 0 {
		t.Fatal("release history incorrectly captured application-level database attachment state")
	}
	rollbackCandidate := version.RecordWithFallback(record)
	rollbackCandidate.DatabaseAttachments = cloneDatabaseAttachments(record.DatabaseAttachments)
	if rollbackCandidate.DatabaseAttachments[0] != record.DatabaseAttachments[0] {
		t.Fatal("rollback-style candidate did not retain current application attachment")
	}
}

func TestGuestDTOExcludesAllDatabaseAttachmentMetadata(t *testing.T) {
	record := attachedNodeRecord()
	guest, err := guestDeploymentResponse(record, "running")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(guest)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["app"] == nil || fields["status"] == nil || fields["url"] == nil {
		t.Fatalf("Guest DTO fields = %v", fields)
	}
	for _, forbidden := range []string{"attachment_", "database_", "databaseAttachments", "DATABASE_URL", "postgresql://"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("Guest DTO exposed database marker %q", forbidden)
		}
	}
}

func TestDatabaseAttachmentStrategyAndManagedEnvironmentRules(t *testing.T) {
	node := attachedNodeRecord()
	if !deploymentSupportsMiniBase(node) {
		t.Fatal("node-express deployment did not support MiniBase")
	}
	fullstack := DeploymentRecord{
		App:      "project",
		Strategy: deploymentStrategyFullstackViteNode,
		Services: []DeploymentServiceRecord{{Name: fullstackFrontendService, Strategy: deploymentStrategyViteStatic}, {Name: fullstackBackendService, Strategy: deploymentStrategyNodeExpress}},
	}
	if !deploymentSupportsMiniBase(fullstack) {
		t.Fatal("fullstack Node backend did not support MiniBase")
	}
	for _, strategy := range []string{deploymentStrategyViteStatic, deploymentStrategyDockerfile} {
		if deploymentSupportsMiniBase(DeploymentRecord{App: "unsupported", Strategy: strategy}) {
			t.Fatalf("unsupported %s deployment accepted MiniBase", strategy)
		}
	}
	if err := validateManagedDatabaseEnvironment(node, map[string]string{"DATABASE_URL": "external"}); err == nil {
		t.Fatal("attached deployment accepted a user-managed DATABASE_URL")
	}
	node.DatabaseAttachments = nil
	if err := validateManagedDatabaseEnvironment(node, map[string]string{"DATABASE_URL": "external"}); err != nil {
		t.Fatalf("unattached deployment rejected external DATABASE_URL: %v", err)
	}
}

func TestResolveDatabaseRuntimeBuildsEscapedURIInMemory(t *testing.T) {
	record := attachedNodeRecord()
	password := "p@ss:/?#[]% with spaces"
	client := &fakeMiniBaseClient{binding: miniBaseBinding{
		DatabaseID:    record.DatabaseAttachments[0].DatabaseID,
		Engine:        "postgresql",
		Host:          "minibase-postgres",
		Port:          5432,
		Database:      "mb_db_0123456789abcdef0123456789abcdef",
		Username:      "mb_role_0123456789abcdef0123456789abcdef",
		Password:      password,
		DockerNetwork: reactorLabDataNetwork,
	}}
	replaceMiniBaseClientForTest(t, client)
	runtime, err := resolveDatabaseRuntime(record, map[string]string{"SAFE_NAME": "safe"})
	if err != nil {
		t.Fatal(err)
	}
	connection := runtime.Environment["DATABASE_URL"]
	parsed, err := url.Parse(connection)
	if err != nil {
		t.Fatal("DATABASE_URL was not a valid URI")
	}
	decodedPassword, present := parsed.User.Password()
	if !present || decodedPassword != password || parsed.User.Username() != client.binding.Username {
		t.Fatal("DATABASE_URL did not preserve escaped credentials")
	}
	if parsed.Hostname() != "minibase-postgres" || parsed.Port() != "5432" || parsed.Path != "/"+client.binding.Database || parsed.Query().Get("sslmode") != "disable" {
		t.Fatal("DATABASE_URL contained unexpected connection coordinates")
	}
	if runtime.DataNetwork != reactorLabDataNetwork || runtime.Redaction["MINIBASE_DATABASE_PASSWORD"] != password {
		t.Fatal("managed binding redaction/network metadata was incomplete")
	}
}

func TestDatabaseEnabledContainerConnectsPrivateNetworkBeforeStart(t *testing.T) {
	store := replaceRuntimeEnvironmentStoreForTest(t)
	var commands [][]string
	var envFile string
	var envNamesPresent bool
	replaceCommandRunnerForTest(t, func(_ string, name string, args ...string) (string, error) {
		command := append([]string{name}, args...)
		commands = append(commands, command)
		if slices.Equal(args, []string{"network", "inspect", "-f", "{{.Driver}} {{.Internal}}", reactorLabDataNetwork}) {
			return "bridge true\n", nil
		}
		if len(args) > 0 && args[0] == "create" {
			for index, arg := range args {
				if arg == "--env-file" && index+1 < len(args) {
					envFile = args[index+1]
					content, readErr := os.ReadFile(envFile)
					if readErr != nil {
						return "", readErr
					}
					envNamesPresent = strings.Contains(string(content), "DATABASE_URL=") && strings.Contains(string(content), "PORT=3000")
				}
			}
		}
		return "", nil
	})
	sentinel := "managed-value-not-for-command-line"
	err := startManagedDeploymentContainerWithOptions(
		"scheduler", "minideploy-scheduler-backend-release-test", "minideploy-scheduler-backend:test",
		8082, 3000, deploymentStrategyNodeExpress, map[string]string{"DATABASE_URL": sentinel},
		managedContainerOptions{Network: "minideploy-scheduler-release-test", DataNetwork: reactorLabDataNetwork, Service: fullstackBackendService, App: "scheduler"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 || commands[1][1] != "create" || !slices.Equal(commands[2][1:4], []string{"network", "connect", reactorLabDataNetwork}) || commands[3][1] != "start" {
		t.Fatalf("database-enabled Docker lifecycle order was not inspect/create/connect/start: %#v", commands)
	}
	joined := ""
	for _, command := range commands {
		joined += strings.Join(command, " ") + "\n"
	}
	for _, required := range []string{"--network minideploy-scheduler-release-test", "127.0.0.1:8082:3000", "--network-alias backend"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("database-enabled Docker lifecycle omitted %q", required)
		}
	}
	if strings.Contains(joined, sentinel) || !envNamesPresent {
		t.Fatal("DATABASE_URL was exposed in command arguments or absent from the secure env file")
	}
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Fatal("temporary database runtime env file was not cleaned")
	}
	if store.root == "" {
		t.Fatal("test runtime store was not configured")
	}
}

func TestReactorLabMigrationLifecycleAndRedaction(t *testing.T) {
	replaceRuntimeEnvironmentStoreForTest(t)
	previousLogs := deploymentLogsDir
	deploymentLogsDir = filepath.Join(t.TempDir(), "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousLogs })

	password := "migration-password-sentinel"
	connection := "postgresql://role:" + password + "@minibase-postgres:5432/database?sslmode=disable"
	runtime := databaseRuntime{
		Environment: map[string]string{"DATABASE_URL": connection},
		Redaction: map[string]string{
			"DATABASE_URL":               connection,
			"MINIBASE_DATABASE_PASSWORD": password,
		},
		DataNetwork: reactorLabDataNetwork,
	}
	var commands [][]string
	var migrationEnvFile string
	replaceCommandRunnerForTest(t, func(_ string, name string, args ...string) (string, error) {
		commands = append(commands, append([]string{name}, args...))
		if slices.Equal(args, []string{"network", "inspect", "-f", "{{.Driver}} {{.Internal}}", reactorLabDataNetwork}) {
			return "bridge true\n", nil
		}
		if len(args) > 0 && args[0] == "run" {
			for index, arg := range args {
				if arg == "--env-file" && index+1 < len(args) {
					migrationEnvFile = args[index+1]
					content, readErr := os.ReadFile(migrationEnvFile)
					if readErr != nil || !strings.Contains(string(content), "DATABASE_URL=") {
						return "", errors.New("migration env file unavailable")
					}
				}
			}
			return "migration connected with " + connection + " and " + password, nil
		}
		return "", nil
	})
	if err := runReactorLabMigration("scheduler", "scheduler:test", packageManagerNPM, true, runtime); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("migration command count = %d", len(commands))
	}
	joined := strings.Join(commands[1], " ")
	for _, required := range []string{"run", "--rm", "--network " + reactorLabDataNetwork, "npm run reactorlab:migrate"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("migration command omitted %q", required)
		}
	}
	if strings.Contains(joined, connection) || strings.Contains(joined, password) || strings.Contains(joined, " -p ") {
		t.Fatal("migration exposed a secret in arguments or published a host port")
	}
	if _, err := os.Stat(migrationEnvFile); !os.IsNotExist(err) {
		t.Fatal("migration temporary env file was not cleaned")
	}
	logData, err := readDeploymentLog("scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logData, connection) || strings.Contains(logData, password) || !strings.Contains(logData, redactedEnvironmentValue) {
		t.Fatal("migration output was not safely redacted")
	}

	commands = nil
	if err := runReactorLabMigration("scheduler", "scheduler:test", packageManagerNPM, false, runtime); err != nil || len(commands) != 0 {
		t.Fatal("absent reactorlab:migrate script did not skip normally")
	}
}

func TestReactorLabMigrationFailureIsSafeAndCleansCandidate(t *testing.T) {
	replaceRuntimeEnvironmentStoreForTest(t)
	previousLogs := deploymentLogsDir
	deploymentLogsDir = filepath.Join(t.TempDir(), "deploy-logs")
	t.Cleanup(func() { deploymentLogsDir = previousLogs })
	password := "failed-migration-password-sentinel"
	runtime := databaseRuntime{
		Environment: map[string]string{"DATABASE_URL": "postgresql://role:" + password + "@minibase-postgres/database"},
		Redaction:   map[string]string{"MINIBASE_DATABASE_PASSWORD": password},
		DataNetwork: reactorLabDataNetwork,
	}
	var removed bool
	replaceCommandRunnerForTest(t, func(_ string, _ string, args ...string) (string, error) {
		if slices.Equal(args, []string{"network", "inspect", "-f", "{{.Driver}} {{.Internal}}", reactorLabDataNetwork}) {
			return "bridge true", nil
		}
		if len(args) > 0 && args[0] == "run" {
			return "failed with " + password, errors.New("exit status 1")
		}
		if len(args) > 2 && args[0] == "rm" && args[1] == "-f" {
			removed = true
		}
		return "", nil
	})
	err := runReactorLabMigration("scheduler", "scheduler:test", packageManagerNPM, true, runtime)
	if err == nil || strings.Contains(err.Error(), password) || !removed {
		t.Fatal("failed migration did not abort safely and clean its candidate")
	}
	logData, readErr := readDeploymentLog("scheduler")
	if readErr != nil || strings.Contains(logData, password) {
		t.Fatal("failed migration leaked managed database credentials")
	}
}

func TestReactorLabDataNetworkMustBePrivateBridge(t *testing.T) {
	for _, output := range []string{"bridge false", "overlay true", "", "bridge true extra"} {
		t.Run(strings.ReplaceAll(output, " ", "_"), func(t *testing.T) {
			replaceCommandRunnerForTest(t, func(_ string, _ string, _ ...string) (string, error) { return output, nil })
			if err := validateReactorLabDataNetwork(); err == nil {
				t.Fatalf("unsafe data network description %q accepted", output)
			}
		})
	}
}
