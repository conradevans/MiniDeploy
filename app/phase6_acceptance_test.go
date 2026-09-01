package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func phase6AcceptanceID(t *testing.T) string {
	t.Helper()
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(random)
}

func phase6AcceptancePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func phase6Docker(t *testing.T, args ...string) string {
	t.Helper()
	output, err := executeCommand("", "docker", args...)
	if err != nil {
		t.Fatalf("acceptance Docker operation %q failed", strings.Join(args[:min(2, len(args))], " "))
	}
	return strings.TrimSpace(output)
}

func phase6AdminSQL(t *testing.T, input string) string {
	t.Helper()
	command := exec.Command(
		"docker", "exec", "-i", "minibase-postgres", "psql",
		"-X", "--no-psqlrc", "-v", "ON_ERROR_STOP=1",
		"-U", "minibase_admin", "-d", "postgres", "-A", "-t", "-q",
	)
	command.Stdin = strings.NewReader(input)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		t.Fatal("acceptance PostgreSQL administration failed safely")
	}
	return strings.TrimSpace(stdout.String())
}

func phase6ContainerSQL(t *testing.T, container, sql string) string {
	t.Helper()
	output, err := executeCommand("", "docker", "exec", container, "sh", "-c", `psql "$DATABASE_URL" -X --no-psqlrc -v ON_ERROR_STOP=1 -A -t -q -c "$1"`, "phase6", sql)
	if err != nil {
		t.Fatal("acceptance application-role PostgreSQL operation failed")
	}
	return strings.TrimSpace(output)
}

func phase6ContainerEnvironmentNames(t *testing.T, container string) map[string]bool {
	t.Helper()
	output := phase6Docker(t, "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", container)
	names := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		name, _, found := strings.Cut(line, "=")
		if found {
			names[name] = true
		}
	}
	return names
}

func phase6ContainerNetworks(t *testing.T, container string) map[string]json.RawMessage {
	t.Helper()
	output := phase6Docker(t, "inspect", "-f", "{{json .NetworkSettings.Networks}}", container)
	var networks map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &networks); err != nil {
		t.Fatal("acceptance container network inspection was malformed")
	}
	return networks
}

func TestPhase6RealCrossServiceAcceptance(t *testing.T) {
	if os.Getenv("REACTORLAB_PHASE6_ACCEPTANCE") != "1" {
		t.Skip("set REACTORLAB_PHASE6_ACCEPTANCE=1 for real PostgreSQL/Docker acceptance")
	}
	miniBaseBinary := os.Getenv("REACTORLAB_PHASE6_MINIBASE_BINARY")
	if miniBaseBinary == "" {
		t.Fatal("REACTORLAB_PHASE6_MINIBASE_BINARY is required")
	}
	if _, err := os.Stat(miniBaseBinary); err != nil {
		t.Fatal("Phase 6 MiniBase acceptance binary is unavailable")
	}

	runID := phase6AcceptanceID(t)
	app := "phase6-acceptance-" + runID
	root, err := os.MkdirTemp("/tmp", "reactorlab-phase6-acceptance-")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(root, "/tmp/reactorlab-phase6-acceptance-") {
		t.Fatal("acceptance root containment failed")
	}
	rootRemoved := false
	defer func() {
		if !rootRemoved {
			_ = os.RemoveAll(root)
		}
	}()

	port := phase6AcceptancePort(t)
	tokenPath := filepath.Join(root, "secrets", "integration-token")
	credentialRoot := filepath.Join(root, "secrets", "databases")
	metadataPath := filepath.Join(root, "data", "minibase.db")
	backupRoot := filepath.Join(root, "backups")
	frontendRoot := filepath.Join(root, "frontend")
	if err := os.MkdirAll(frontendRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	process := exec.Command(
		miniBaseBinary,
		"-listen", "127.0.0.1:"+strconv.Itoa(port),
		"-metadata-db", metadataPath,
		"-database-secrets", credentialRoot,
		"-backup-root", backupRoot,
		"-frontend-dir", frontendRoot,
		"-integration-token", tokenPath,
	)
	process.Stdout = io.Discard
	process.Stderr = io.Discard
	if err := process.Start(); err != nil {
		t.Fatal("temporary MiniBase process did not start")
	}
	processDone := make(chan error, 1)
	go func() { processDone <- process.Wait() }()
	processStopped := false
	stopProcess := func() {
		if processStopped {
			return
		}
		processStopped = true
		_ = process.Process.Signal(os.Interrupt)
		select {
		case <-processDone:
		case <-time.After(5 * time.Second):
			_ = process.Process.Kill()
			<-processDone
		}
	}
	defer stopProcess()

	baseURL := "http://127.0.0.1:" + strconv.Itoa(port)
	healthClient := &http.Client{Timeout: time.Second}
	healthy := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		response, requestErr := healthClient.Get(baseURL + "/health")
		if requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				healthy = true
				break
			}
		}
		select {
		case <-processDone:
			processStopped = true
			t.Fatal("temporary MiniBase process exited before becoming healthy")
		default:
		}
	}
	if !healthy {
		t.Fatal("temporary loopback MiniBase integration API did not become healthy")
	}

	client, err := newMiniBaseHTTPClient(baseURL, tokenPath, &http.Client{Timeout: miniBaseRequestTimeout})
	if err != nil {
		t.Fatal(err)
	}
	database, err := client.CreateDatabase(context.Background(), "Phase 6 Acceptance "+runID)
	if err != nil {
		t.Fatal("real MiniBase database provisioning failed safely")
	}
	attachment, err := client.CreateAttachment(context.Background(), database.ID, app)
	if err != nil {
		t.Fatal("real MiniBase attachment creation failed safely")
	}
	binding, err := client.ResolveBinding(context.Background(), attachment.ID)
	if err != nil || binding.DatabaseID != database.ID {
		t.Fatal("authenticated real binding resolution failed")
	}
	credentialPath := filepath.Join(credentialRoot, database.ID, "password")
	credentialInfo, err := os.Stat(credentialPath)
	if err != nil || credentialInfo.Mode().Perm() != 0o600 {
		t.Fatal("real acceptance credential did not have mode 0600")
	}

	databaseCreated := true
	defer func() {
		if databaseCreated {
			phase6AdminSQL(t, `DROP DATABASE IF EXISTS "`+binding.Database+`" WITH (FORCE); DROP ROLE IF EXISTS "`+binding.Username+`";`)
		}
	}()

	buildRoot := filepath.Join(root, "image")
	if err := os.MkdirAll(buildRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerfile := "FROM postgres:17\nLABEL com.minideploy.phase6-acceptance=\"" + runID + "\"\nCOPY npm /usr/local/bin/npm\nRUN chmod 0755 /usr/local/bin/npm\nCMD [\"sleep\",\"infinity\"]\n"
	npmShim := "#!/bin/sh\n[ \"$1\" = run ] && [ \"$2\" = reactorlab:migrate ] || exit 64\nexec psql \"$DATABASE_URL\" -X --no-psqlrc -v ON_ERROR_STOP=1 -q -c 'CREATE TABLE IF NOT EXISTS reactorlab_phase6_migration (marker text NOT NULL)'\n"
	if err := os.WriteFile(filepath.Join(buildRoot, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(buildRoot, "npm"), []byte(npmShim), 0o700); err != nil {
		t.Fatal(err)
	}
	image := "minideploy-phase6-acceptance:" + runID
	phase6Docker(t, "build", "--label", "com.minideploy.phase6-acceptance="+runID, "-t", image, buildRoot)
	imageCreated := true
	defer func() {
		if imageCreated {
			_, _ = executeCommand("", "docker", "image", "rm", "-f", image)
		}
	}()

	network := "minideploy-phase6-acceptance-" + runID
	phase6Docker(t, "network", "create", "--driver", "bridge", "--label", "com.minideploy.phase6-acceptance="+runID, network)
	networkCreated := true
	defer func() {
		if networkCreated {
			_, _ = executeCommand("", "docker", "network", "rm", network)
		}
	}()

	previousClient := miniBaseClient
	miniBaseClient = client
	defer func() { miniBaseClient = previousClient }()
	previousRuntimeStore := runtimeEnvironmentStore
	runtimeEnvironmentStore = newRuntimeEnvironmentFileStore(filepath.Join(root, "runtime-secrets"))
	defer func() { runtimeEnvironmentStore = previousRuntimeStore }()
	previousLogs := deploymentLogsDir
	deploymentLogsDir = filepath.Join(root, "deploy-logs")
	defer func() { deploymentLogsDir = previousLogs }()
	record := attachedNodeRecord()
	record.App = app
	record.DatabaseAttachments = []DatabaseAttachmentRecord{{AttachmentID: attachment.ID, DatabaseID: database.ID, DisplayName: database.DisplayName, BindingName: miniBaseBindingPrimary}}
	runtime, err := resolveDatabaseRuntime(record, map[string]string{"ACCEPTANCE_MODE": "phase6"})
	if err != nil {
		t.Fatal("managed runtime resolution failed")
	}
	if err := runReactorLabMigration(app, image, packageManagerNPM, true, runtime); err != nil {
		t.Fatal("real reactorlab:migrate acceptance failed")
	}

	containers := make([]string, 0, 4)
	cleanupContainers := func() {
		for _, container := range containers {
			_, _ = executeCommand("", "docker", "rm", "-f", container)
		}
		containers = nil
	}
	defer cleanupContainers()
	startBackend := func(kind string, current databaseRuntime) string {
		name := "minideploy-" + app + "-" + kind
		port := phase6AcceptancePort(t)
		if err := startManagedDeploymentContainerWithOptions(app, name, image, port, 3000, deploymentStrategyNodeExpress, current.Environment, managedContainerOptions{Network: network, DataNetwork: reactorLabDataNetwork, Service: fullstackBackendService, App: app}); err != nil {
			t.Fatal("managed backend acceptance container failed to start")
		}
		containers = append(containers, name)
		return name
	}

	backend := startBackend("candidate-one", runtime)
	frontend := "minideploy-" + app + "-frontend"
	if err := startManagedDeploymentContainerWithOptions(app, frontend, image, phase6AcceptancePort(t), 80, deploymentStrategyViteStatic, nil, managedContainerOptions{Network: network, Service: fullstackFrontendService, App: app}); err != nil {
		t.Fatal("synthetic frontend acceptance container failed to start")
	}
	containers = append(containers, frontend)

	backendNames := phase6ContainerEnvironmentNames(t, backend)
	frontendNames := phase6ContainerEnvironmentNames(t, frontend)
	if !backendNames["DATABASE_URL"] || frontendNames["DATABASE_URL"] {
		t.Fatal("DATABASE_URL presence was not limited to the backend")
	}
	backendNetworks := phase6ContainerNetworks(t, backend)
	frontendNetworks := phase6ContainerNetworks(t, frontend)
	if backendNetworks[network] == nil || backendNetworks[reactorLabDataNetwork] == nil || frontendNetworks[network] == nil || frontendNetworks[reactorLabDataNetwork] != nil {
		t.Fatal("backend/frontend Docker network isolation was incorrect")
	}
	if phase6ContainerSQL(t, backend, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='reactorlab_phase6_migration'") != "1" {
		t.Fatal("migration-created table was not visible to the application role")
	}
	phase6ContainerSQL(t, backend, "CREATE TABLE IF NOT EXISTS reactorlab_phase6_data (marker text PRIMARY KEY); INSERT INTO reactorlab_phase6_data(marker) VALUES ('persisted') ON CONFLICT DO NOTHING")

	secondBinding, err := client.ResolveBinding(context.Background(), attachment.ID)
	if err != nil || secondBinding.DatabaseID != binding.DatabaseID {
		t.Fatal("replacement binding did not resolve the same database ID")
	}
	secondRuntime, err := resolveDatabaseRuntime(record, map[string]string{"ACCEPTANCE_MODE": "phase6"})
	if err != nil {
		t.Fatal("replacement managed runtime resolution failed")
	}
	if err := runReactorLabMigration(app, image, packageManagerNPM, true, secondRuntime); err != nil {
		t.Fatal("replacement migration failed")
	}
	replacement := startBackend("candidate-two", secondRuntime)
	if phase6ContainerSQL(t, replacement, "SELECT count(*) FROM reactorlab_phase6_data WHERE marker='persisted'") != "1" {
		t.Fatal("sample data did not survive candidate replacement")
	}
	phase6Docker(t, "rm", "-f", backend)

	rollbackBinding, err := client.ResolveBinding(context.Background(), attachment.ID)
	if err != nil || rollbackBinding.DatabaseID != binding.DatabaseID {
		t.Fatal("rollback-style binding did not resolve the same database ID")
	}
	rollbackRuntime, err := resolveDatabaseRuntime(record, map[string]string{"ACCEPTANCE_MODE": "phase6"})
	if err != nil {
		t.Fatal("rollback-style managed runtime resolution failed")
	}
	if err := runReactorLabMigration(app, image, packageManagerNPM, true, rollbackRuntime); err != nil {
		t.Fatal("rollback-style migration failed")
	}
	rollback := startBackend("rollback", rollbackRuntime)
	if phase6ContainerSQL(t, rollback, "SELECT count(*) FROM reactorlab_phase6_data WHERE marker='persisted'") != "1" {
		t.Fatal("sample data did not survive rollback-style replacement")
	}
	phase6Docker(t, "rm", "-f", replacement)

	if err := client.DeleteAttachment(context.Background(), attachment.ID); err != nil {
		t.Fatal("attachment detach failed")
	}
	if _, err := client.ResolveBinding(context.Background(), attachment.ID); err == nil {
		t.Fatal("detached attachment still resolved binding material")
	}
	if phase6ContainerSQL(t, rollback, "SELECT count(*) FROM reactorlab_phase6_data WHERE marker='persisted'") != "1" {
		t.Fatal("database data did not survive attachment detach")
	}
	if info, err := os.Stat(credentialPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("database credential did not survive attachment detach")
	}

	cleanupContainers()
	phase6AdminSQL(t, `DROP DATABASE IF EXISTS "`+binding.Database+`" WITH (FORCE); DROP ROLE IF EXISTS "`+binding.Username+`";`)
	databaseCreated = false
	if phase6AdminSQL(t, "SELECT (SELECT count(*) FROM pg_database WHERE datname='"+binding.Database+"')::text || '|' || (SELECT count(*) FROM pg_roles WHERE rolname='"+binding.Username+"')::text") != "0|0" {
		t.Fatal("acceptance database or role remained after exact cleanup")
	}
	if err := os.Remove(credentialPath); err != nil {
		t.Fatal("acceptance credential cleanup failed")
	}
	if err := os.Remove(filepath.Dir(credentialPath)); err != nil {
		t.Fatal("acceptance credential directory cleanup failed")
	}
	phase6Docker(t, "network", "rm", network)
	networkCreated = false
	phase6Docker(t, "image", "rm", "-f", image)
	imageCreated = false
	if phase6Docker(t, "ps", "-aq", "--filter", "label=com.minideploy.app="+app) != "" ||
		phase6Docker(t, "network", "ls", "-q", "--filter", "label=com.minideploy.phase6-acceptance="+runID) != "" ||
		phase6Docker(t, "image", "ls", "-q", "--filter", "label=com.minideploy.phase6-acceptance="+runID) != "" {
		t.Fatal("acceptance Docker resource count was not zero")
	}
	stopProcess()
	if err := os.RemoveAll(root); err != nil {
		t.Fatal("acceptance temporary control-plane cleanup failed")
	}
	rootRemoved = true
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("acceptance metadata, token, or temporary files remained")
	}
}
