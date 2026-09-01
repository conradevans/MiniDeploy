package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func phaseTwoSecretSentinel() string {
	return strings.Join(
		[]string{
			"SHOULD_NEVER",
			"APPEAR_SECRET",
			"12345",
		},
		"_",
	)
}

func replaceRuntimeEnvironmentStoreForTest(
	t *testing.T,
) *runtimeEnvironmentFileStore {
	t.Helper()

	previous := runtimeEnvironmentStore
	replacement := newRuntimeEnvironmentFileStore(
		filepath.Join(t.TempDir(), "secrets"),
	)
	runtimeEnvironmentStore = replacement

	t.Cleanup(func() {
		runtimeEnvironmentStore = previous
	})

	return replacement
}

func TestRuntimeEnvironmentValidation(t *testing.T) {
	for _, name := range []string{
		"MONGODB_URI",
		"JWT_SECRET",
		"API_KEY_2",
		"_PRIVATE",
	} {
		t.Run("accept "+name, func(t *testing.T) {
			if err := validateRuntimeEnvironment(
				map[string]string{name: "value"},
			); err != nil {

				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	for _, name := range []string{
		"",
		"1INVALID",
		"HAS=EQUALS",
		"HAS SPACE",
		"HAS\nNEWLINE",
		"HAS-DASH",
		"$(SHELL)",
		"PORT",
	} {
		t.Run("reject "+name, func(t *testing.T) {
			if err := validateRuntimeEnvironment(
				map[string]string{name: "value"},
			); err == nil {

				t.Fatalf(
					"environment name %q unexpectedly accepted",
					name,
				)
			}
		})
	}

	for _, value := range []string{
		"line one\nline two",
		"line one\rline two",
		"nul\x00value",
	} {
		if err := validateRuntimeEnvironment(
			map[string]string{"API_KEY": value},
		); err == nil {

			t.Fatalf(
				"environment value %q unexpectedly accepted",
				value,
			)
		}
	}
}

func TestRuntimeEnvironmentFilePermissionsAndRoundTrip(
	t *testing.T,
) {
	store := replaceRuntimeEnvironmentStoreForTest(t)
	values := map[string]string{
		"MONGODB_URI": "mongodb://example.invalid/database?x=1",
		"JWT_SECRET":  phaseTwoSecretSentinel(),
	}

	if err := store.Replace("express-app", values); err != nil {
		t.Fatalf("Replace() error: %v", err)
	}

	directoryInfo, err := os.Stat(store.root)
	if err != nil {
		t.Fatalf("Stat(root) error: %v", err)
	}

	if directoryInfo.Mode().Perm() != 0700 {
		t.Fatalf(
			"secret directory mode = %o; want 700",
			directoryInfo.Mode().Perm(),
		)
	}

	path, err := store.path("express-app")
	if err != nil {
		t.Fatalf("path() error: %v", err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(file) error: %v", err)
	}

	if fileInfo.Mode().Perm() != 0600 {
		t.Fatalf(
			"secret file mode = %o; want 600",
			fileInfo.Mode().Perm(),
		)
	}

	loaded, err := store.Load("express-app")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if !reflect.DeepEqual(loaded, values) {
		t.Fatalf("loaded = %#v; want %#v", loaded, values)
	}

	if err := os.Chmod(store.root, 0755); err != nil {
		t.Fatalf("Chmod(root) error: %v", err)
	}

	if _, err := store.Load("express-app"); err == nil {
		t.Fatal("Load() accepted an insecure secret directory mode")
	}
}

func TestRuntimeEnvironmentPathContainmentAndDeletion(
	t *testing.T,
) {
	parent := t.TempDir()
	root := filepath.Join(parent, "secrets")
	store := newRuntimeEnvironmentFileStore(root)
	sentinel := filepath.Join(parent, "project-sentinel")

	if err := os.WriteFile(
		sentinel,
		[]byte("must remain"),
		0600,
	); err != nil {
		t.Fatalf("WriteFile(sentinel) error: %v", err)
	}

	for _, app := range []string{
		"",
		".",
		"..",
		"../project",
		"project/..",
		"..\\project",
	} {
		if _, err := store.path(app); err == nil {
			t.Fatalf("path(%q) unexpectedly succeeded", app)
		}

		if err := store.Delete(app); err == nil {
			t.Fatalf("Delete(%q) unexpectedly succeeded", app)
		}
	}

	if _, err := strictChildPath(
		root,
		filepath.Join(root, "..", "escaped.env"),
	); err == nil {
		t.Fatal("escaped secret path unexpectedly passed containment")
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("parent/project sentinel was removed: %v", err)
	}

	if err := store.Replace(
		"express-app",
		map[string]string{"API_KEY": "value"},
	); err != nil {
		t.Fatalf("Replace() error: %v", err)
	}

	path, _ := store.path("express-app")
	if err := store.Delete("express-app"); err != nil {
		t.Fatalf("Delete(valid app) error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("secret file still exists after delete: %v", err)
	}
}

func TestRuntimeEnvironmentUpdateSemantics(t *testing.T) {
	store := replaceRuntimeEnvironmentStoreForTest(t)
	previous := map[string]string{
		"MONGODB_URI": "mongodb://previous.invalid/database",
	}

	if err := store.Replace("express-app", previous); err != nil {
		t.Fatalf("Replace(previous) error: %v", err)
	}

	preserved, err := prepareRuntimeEnvironmentChange(
		"express-app",
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("prepare omitted update error: %v", err)
	}

	if !reflect.DeepEqual(preserved.effective, previous) {
		t.Fatalf(
			"omitted update = %#v; want %#v",
			preserved.effective,
			previous,
		)
	}

	if err := preserved.Commit(); err != nil {
		t.Fatalf("Commit(omitted) error: %v", err)
	}

	replacement := map[string]string{
		"JWT_SECRET": phaseTwoSecretSentinel(),
	}
	changed, err := prepareRuntimeEnvironmentChange(
		"express-app",
		replacement,
		true,
	)
	if err != nil {
		t.Fatalf("prepare replacement error: %v", err)
	}

	beforeCommit, err := store.Load("express-app")
	if err != nil {
		t.Fatalf("Load(before commit) error: %v", err)
	}

	if !reflect.DeepEqual(beforeCommit, previous) {
		t.Fatal("replacement modified working secrets before commit")
	}

	if err := changed.Commit(); err != nil {
		t.Fatalf("Commit(replacement) error: %v", err)
	}

	afterCommit, err := store.Load("express-app")
	if err != nil {
		t.Fatalf("Load(after commit) error: %v", err)
	}

	if !reflect.DeepEqual(afterCommit, replacement) {
		t.Fatalf(
			"committed values = %#v; want %#v",
			afterCommit,
			replacement,
		)
	}

	if err := changed.Rollback(); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}

	afterRollback, err := store.Load("express-app")
	if err != nil {
		t.Fatalf("Load(after rollback) error: %v", err)
	}

	if !reflect.DeepEqual(afterRollback, previous) {
		t.Fatalf(
			"rolled back values = %#v; want %#v",
			afterRollback,
			previous,
		)
	}

	cleared, err := prepareRuntimeEnvironmentChange(
		"express-app",
		map[string]string{},
		true,
	)
	if err != nil {
		t.Fatalf("prepare clear error: %v", err)
	}

	if err := cleared.Commit(); err != nil {
		t.Fatalf("Commit(clear) error: %v", err)
	}

	loaded, err := store.Load("express-app")
	if err != nil {
		t.Fatalf("Load(clear) error: %v", err)
	}

	if len(loaded) != 0 {
		t.Fatalf("cleared environment = %#v; want empty", loaded)
	}
}

func TestRedeployEnvironmentJSONSemantics(t *testing.T) {
	omitted, err := decodeRedeployRequest(strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("decode omitted environment: %v", err)
	}

	if omitted.Environment != nil {
		t.Fatalf("omitted environment = %#v; want nil", omitted.Environment)
	}

	replacement, err := decodeRedeployRequest(
		strings.NewReader(`{"environment":{}}`),
	)
	if err != nil {
		t.Fatalf("decode empty replacement: %v", err)
	}

	if replacement.Environment == nil || len(replacement.Environment) != 0 {
		t.Fatalf(
			"empty replacement = %#v; want non-nil empty map",
			replacement.Environment,
		)
	}

	if _, err := decodeRedeployRequest(
		strings.NewReader(`{"environment":null}`),
	); err == nil {
		t.Fatal("null environment unexpectedly accepted")
	}
}

func TestRestartPreservesExistingContainerEnvironment(t *testing.T) {
	got := restartContainerArguments("minideploy-express-app")
	want := []string{"restart", "minideploy-express-app"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("restart arguments = %v; want %v", got, want)
	}
}

func TestRollbackCandidateUsesCurrentRuntimeEnvironmentNames(
	t *testing.T,
) {
	previous := DeploymentRecord{
		App:                  "express-app",
		Container:            "previous-container",
		Port:                 8081,
		EnvironmentVariables: nil,
	}
	currentEnvironment := map[string]string{
		"CURRENT_SECRET": "current-value",
		"MONGODB_URI":    "mongodb://current.invalid/app",
	}

	record := rollbackCandidateRecord(
		previous,
		"rollback-candidate",
		8082,
		currentEnvironment,
	)

	if record.Container != "rollback-candidate" || record.Port != 8082 {
		t.Fatalf("unexpected rollback record: %#v", record)
	}

	wantNames := []string{"CURRENT_SECRET", "MONGODB_URI"}
	if !reflect.DeepEqual(record.EnvironmentVariables, wantNames) {
		t.Fatalf(
			"rollback names = %v; want %v",
			record.EnvironmentVariables,
			wantNames,
		)
	}
}

func TestDockerEnvironmentUsesEnvFileAndInjectsNodePort(
	t *testing.T,
) {
	store := replaceRuntimeEnvironmentStoreForTest(t)
	secret := phaseTwoSecretSentinel()
	configured := map[string]string{
		"JWT_SECRET": secret,
	}

	dockerEnvironment := dockerRuntimeEnvironment(
		deploymentStrategyNodeExpress,
		4321,
		configured,
	)

	if dockerEnvironment["PORT"] != "4321" {
		t.Fatalf(
			"PORT = %q; want 4321",
			dockerEnvironment["PORT"],
		)
	}

	if _, exists := configured["PORT"]; exists {
		t.Fatal("PORT injection mutated configured environment")
	}

	path, cleanup, err := store.TemporaryDockerEnvFile(
		"express-app",
		dockerEnvironment,
	)
	if err != nil {
		t.Fatalf("TemporaryDockerEnvFile() error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(temp env) error: %v", err)
	}

	if !strings.Contains(string(content), "PORT=4321\n") ||
		!strings.Contains(string(content), secret) {

		t.Fatalf("unexpected Docker env file content")
	}

	args := managedContainerRunArguments(
		"minideploy-express-app",
		"minideploy-express-app:v1",
		8081,
		4321,
		path,
	)
	joinedArgs := strings.Join(args, " ")

	if !strings.Contains(joinedArgs, "--env-file "+path) {
		t.Fatalf("Docker args missing env file: %v", args)
	}

	if strings.Contains(joinedArgs, secret) {
		t.Fatal("secret value leaked into Docker command arguments")
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary Docker env file still exists: %v", err)
	}
}

func TestRuntimeEnvironmentValuesNeverEnterMetadataOrResponses(
	t *testing.T,
) {
	secret := phaseTwoSecretSentinel()
	record := DeploymentRecord{
		App:                  "express-app",
		RepoURL:              "https://github.com/example/express-app.git",
		Container:            "minideploy-express-app",
		Image:                "minideploy-express-app:v1",
		Port:                 8081,
		ContainerPort:        3000,
		HealthPath:           "/",
		Strategy:             deploymentStrategyNodeExpress,
		PackageManager:       packageManagerNPM,
		PackageInstallMode:   packageInstallModeCI,
		EnvironmentVariables: []string{"JWT_SECRET", "MONGODB_URI"},
	}

	storePath := filepath.Join(t.TempDir(), "deployments.json")
	metadataStore := NewJSONStore(storePath)
	if err := metadataStore.Save(record); err != nil {
		t.Fatalf("Save(metadata) error: %v", err)
	}

	historyPath := filepath.Join(t.TempDir(), "history.json")
	metadataHistory := NewJSONHistoryStore(historyPath, 3)
	if _, err := metadataHistory.Push(record); err != nil {
		t.Fatalf("Push(history) error: %v", err)
	}

	adminJSON, err := json.Marshal(
		DeploymentResponse{
			App:                  record.App,
			Strategy:             record.Strategy,
			EnvironmentVariables: record.EnvironmentVariables,
		},
	)
	if err != nil {
		t.Fatalf("Marshal(admin response) error: %v", err)
	}

	guest, err := guestDeploymentResponse(record, "running")
	if err != nil {
		t.Fatalf("guestDeploymentResponse() error: %v", err)
	}

	guestJSON, err := json.Marshal(guest)
	if err != nil {
		t.Fatalf("Marshal(guest response) error: %v", err)
	}

	for name, data := range map[string][]byte{
		"deployment metadata": mustReadFile(t, storePath),
		"history metadata":    mustReadFile(t, historyPath),
		"Admin response":      adminJSON,
		"Guest response":      guestJSON,
		"Node Dockerfile": []byte(
			generatedNodeExpressDockerfile(
				packageInstallModeCI,
				3000,
			),
		),
	} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("%s leaked secret value", name)
		}
	}

	var guestFields map[string]any
	if err := json.Unmarshal(guestJSON, &guestFields); err != nil {
		t.Fatalf("Unmarshal(guest) error: %v", err)
	}

	if len(guestFields) != 3 {
		t.Fatalf(
			"guest fields = %v; want three-field DTO",
			guestFields,
		)
	}
}

func TestRuntimeEnvironmentRedaction(t *testing.T) {
	secret := phaseTwoSecretSentinel()
	redacted := redactRuntimeEnvironmentValues(
		"startup failed with "+secret,
		map[string]string{"JWT_SECRET": secret},
	)

	if strings.Contains(redacted, secret) {
		t.Fatal("redacted error still contains secret")
	}

	if !strings.Contains(
		redacted,
		redactedEnvironmentValue,
	) {
		t.Fatal("redacted error is missing replacement marker")
	}
}

func TestRedactedRuntimeEnvironmentDoesNotEnterDeploymentLogs(
	t *testing.T,
) {
	previousDirectory := deploymentLogsDir
	deploymentLogsDir = filepath.Join(
		t.TempDir(),
		"deploy-logs",
	)
	t.Cleanup(func() {
		deploymentLogsDir = previousDirectory
	})

	secret := phaseTwoSecretSentinel()
	message := redactRuntimeEnvironmentValues(
		"candidate output: "+secret,
		map[string]string{"JWT_SECRET": secret},
	)

	resetDeploymentLog("express-app", "test")
	deploymentEvent("express-app", "%s", message)

	logs, err := readDeploymentLog("express-app")
	if err != nil {
		t.Fatalf("readDeploymentLog() error: %v", err)
	}

	if strings.Contains(logs, secret) {
		t.Fatal("deployment log contains secret value")
	}

	if !strings.Contains(
		logs,
		redactedEnvironmentValue,
	) {
		t.Fatal("deployment log is missing redaction marker")
	}
}

func TestRuntimeEnvironmentMetadataMustMatchSecureStore(
	t *testing.T,
) {
	record := DeploymentRecord{
		EnvironmentVariables: []string{"JWT_SECRET"},
	}

	if err := verifyRuntimeEnvironmentMetadata(
		record,
		map[string]string{},
	); err == nil {

		t.Fatal("missing secure value unexpectedly matched metadata")
	}

	if err := verifyRuntimeEnvironmentMetadata(
		record,
		map[string]string{"JWT_SECRET": "value"},
	); err != nil {

		t.Fatalf("matching metadata error: %v", err)
	}
}

func TestSecretSentinelDoesNotAppearInSourceFiles(t *testing.T) {
	secret := []byte(phaseTwoSecretSentinel())
	root := filepath.Clean("..")

	for _, relativeRoot := range []string{
		"app",
		"frontend/src",
		"docs",
	} {
		err := filepath.WalkDir(
			filepath.Join(root, relativeRoot),
			func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if entry.IsDir() {
					return nil
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}

				if bytes.Contains(data, secret) {
					t.Fatalf("secret sentinel appears in %s", path)
				}

				return nil
			},
		)
		if err != nil {
			t.Fatalf("scan %s: %v", relativeRoot, err)
		}
	}

	for _, path := range []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "frontend", "README.md"),
	} {
		if bytes.Contains(mustReadFile(t, path), secret) {
			t.Fatalf("secret sentinel appears in %s", path)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", path, err)
	}

	return data
}
