package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testCommitSHA = "0123456789abcdef0123456789abcdef01234567"
	testImageID   = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

type testCommandExitError struct{ code int }

func (err testCommandExitError) Error() string { return "command exited" }
func (err testCommandExitError) ExitCode() int { return err.code }

func TestInspectDeploymentSourceCapturesCommitAndBranch(t *testing.T) {
	var commands []string
	source, err := inspectDeploymentSourceWithRunner(
		"/managed/checkout",
		"https://github.com/conradevans/MiniDeploy.git",
		func(dir string, name string, args ...string) (string, error) {
			if dir != "/managed/checkout" || name != "git" {
				t.Fatalf("unexpected command target: %q %q", dir, name)
			}
			commands = append(commands, strings.Join(args, " "))
			if args[0] == "rev-parse" {
				return testCommitSHA + "\n", nil
			}
			return "feature/exact-source\n", nil
		},
	)
	if err != nil {
		t.Fatalf("inspectDeploymentSourceWithRunner() error: %v", err)
	}
	if source.CommitSHA != testCommitSHA || source.Branch != "feature/exact-source" ||
		source.Provider != "github" ||
		source.Repository != "conradevans/MiniDeploy" {

		t.Fatalf("source = %#v", source)
	}
	wantCommands := []string{
		"rev-parse --verify HEAD",
		"symbolic-ref --quiet --short HEAD",
	}
	if !reflect.DeepEqual(commands, wantCommands) {
		t.Fatalf("commands = %v; want %v", commands, wantCommands)
	}
}

func TestInspectDeploymentSourceAllowsDetachedHead(t *testing.T) {
	source, err := inspectDeploymentSourceWithRunner(
		"/managed/checkout",
		"https://git.example.test/team/app.git",
		func(_ string, _ string, args ...string) (string, error) {
			if args[0] == "rev-parse" {
				return strings.Repeat("A", 64) + "\n", nil
			}
			return "", testCommandExitError{code: 1}
		},
	)
	if err != nil {
		t.Fatalf("detached inspection error: %v", err)
	}
	if source.Branch != "" || source.CommitSHA != strings.Repeat("a", 64) ||
		source.Provider != "" || source.Repository != "" {

		t.Fatalf("detached source = %#v", source)
	}
}

func TestInspectDeploymentSourceRejectsMissingOrMalformedHead(t *testing.T) {
	for _, test := range []struct {
		name   string
		output string
		err    error
	}{
		{name: "missing", err: errors.New("no HEAD")},
		{name: "short", output: "abc123\n"},
		{name: "non-hex", output: strings.Repeat("z", 40)},
		{name: "multiple lines", output: testCommitSHA + "\n" + testCommitSHA},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := inspectDeploymentSourceWithRunner(
				"/managed/checkout",
				"https://git.example.test/team/app.git",
				func(_ string, _ string, _ ...string) (string, error) {
					return test.output, test.err
				},
			)
			if err == nil {
				t.Fatal("malformed or missing HEAD was accepted")
			}
		})
	}
}

func TestGitHubRepositoryIdentityCanonicalizesSafeForms(t *testing.T) {
	for _, repositoryURL := range []string{
		"https://github.com/conradevans/MiniDeploy",
		"https://github.com/conradevans/MiniDeploy.git",
		"git@github.com:conradevans/MiniDeploy.git",
		"ssh://git@github.com/conradevans/MiniDeploy.git",
	} {
		t.Run(repositoryURL, func(t *testing.T) {
			provider, repository := githubRepositoryIdentity(repositoryURL)
			if provider != "github" || repository != "conradevans/MiniDeploy" {
				t.Fatalf("identity = %q, %q", provider, repository)
			}
		})
	}
}

func TestGitHubRepositoryIdentityRejectsLookalikesAndNeverLeaksCredentials(t *testing.T) {
	provider, repository := githubRepositoryIdentity(
		"https://github.com.evil.example/conradevans/MiniDeploy.git",
	)
	if provider != "" || repository != "" {
		t.Fatalf("lookalike identity = %q, %q", provider, repository)
	}

	secretURL := "https://token:password@github.com/conradevans/MiniDeploy.git?access_token=query-secret#fragment-secret"
	provider, repository = githubRepositoryIdentity(secretURL)
	if provider != "github" || repository != "conradevans/MiniDeploy" {
		t.Fatalf("credential-bearing identity = %q, %q", provider, repository)
	}
	for _, secret := range []string{"token", "password", "query-secret", "fragment-secret"} {
		if strings.Contains(repository, secret) {
			t.Fatalf("repository leaked %q: %q", secret, repository)
		}
	}
}

func TestInspectBuiltImageIDValidatesImmutableIdentity(t *testing.T) {
	replaceCommandRunnerForTest(
		t,
		func(_ string, name string, args ...string) (string, error) {
			if name != "docker" || !reflect.DeepEqual(
				args,
				[]string{"image", "inspect", "--format", "{{.Id}}", "minideploy-app:test"},
			) {
				t.Fatalf("unexpected image inspection: %s %v", name, args)
			}
			return testImageID + "\n", nil
		},
	)

	imageID, err := inspectBuiltImageID("minideploy-app:test")
	if err != nil || imageID != testImageID {
		t.Fatalf("inspectBuiltImageID() = %q, %v", imageID, err)
	}
}

func TestDeploymentVersionPreservesProvenanceAndLegacyStaysUnknown(t *testing.T) {
	activatedAt := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	record := DeploymentRecord{
		App:         "source-app",
		Image:       "minideploy-source-app:v1",
		ImageID:     testImageID,
		Source:      &DeploymentSourceRecord{CommitSHA: testCommitSHA, Branch: "main"},
		ActivatedAt: &activatedAt,
		Services: []DeploymentServiceRecord{{
			Name:    fullstackFrontendService,
			ImageID: testImageID,
		}},
	}

	version := deploymentVersion(record)
	restored := version.RecordWithFallback(DeploymentRecord{
		Source:      &DeploymentSourceRecord{CommitSHA: strings.Repeat("f", 40)},
		ImageID:     "sha256:" + strings.Repeat("f", 64),
		ActivatedAt: func() *time.Time { value := activatedAt.Add(time.Hour); return &value }(),
	})
	if restored.Source == nil || restored.Source.CommitSHA != testCommitSHA ||
		restored.ImageID != testImageID || restored.ActivatedAt == nil ||
		!restored.ActivatedAt.Equal(activatedAt) ||
		len(restored.Services) != 1 || restored.Services[0].ImageID != testImageID {

		t.Fatalf("restored provenance = %#v", restored)
	}

	legacy := (DeploymentVersion{App: "legacy"}).RecordWithFallback(record)
	if legacy.Source != nil || legacy.ActivatedAt != nil || legacy.ImageID != "" {
		t.Fatalf("legacy provenance was invented: %#v", legacy)
	}

	fullstackFallback := fullstackTestRecord("legacy-fullstack", "current")
	for index := range fullstackFallback.Services {
		fullstackFallback.Services[index].ImageID = testImageID
	}
	legacyFullstack := (DeploymentVersion{App: "legacy-fullstack"}).RecordWithFallback(fullstackFallback)
	for _, service := range legacyFullstack.Services {
		if service.ImageID != "" {
			t.Fatalf("legacy full-stack service image ID was invented: %#v", service)
		}
	}
}

func TestPrivateResponseExposesProvenanceAndGuestDoesNot(t *testing.T) {
	activatedAt := time.Date(2026, 9, 21, 13, 0, 0, 0, time.UTC)
	record := DeploymentRecord{
		App:         "provenance-app",
		Container:   "minideploy-provenance-app",
		ImageID:     testImageID,
		Source:      &DeploymentSourceRecord{CommitSHA: testCommitSHA, Branch: "main"},
		ActivatedAt: &activatedAt,
	}
	replaceCommandRunnerForTest(t, func(string, string, ...string) (string, error) {
		return "running\n", nil
	})

	admin := deploymentResponse(record)
	if admin.Source == nil || admin.Source.CommitSHA != testCommitSHA ||
		admin.ActivatedAt == nil || !admin.ActivatedAt.Equal(activatedAt) ||
		admin.ImageID != testImageID {

		t.Fatalf("private response provenance = %#v", admin)
	}

	guest, err := guestDeploymentResponse(record, "running")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(guest)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 3 || fields["app"] != record.App ||
		strings.Contains(string(data), "source") ||
		strings.Contains(string(data), "commit") ||
		strings.Contains(string(data), "imageId") ||
		strings.Contains(string(data), "activatedAt") {

		t.Fatalf("Guest response exposed provenance: %s", data)
	}
}

func TestRollbackCandidatePreservesSourceAndImageWithNewActivation(t *testing.T) {
	previousActivation := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	rollbackActivation := previousActivation.Add(24 * time.Hour)
	previousClock := deploymentActivationTime
	deploymentActivationTime = func() time.Time { return rollbackActivation }
	t.Cleanup(func() { deploymentActivationTime = previousClock })

	previous := DeploymentRecord{
		Source:      &DeploymentSourceRecord{CommitSHA: testCommitSHA, Branch: "main"},
		ImageID:     testImageID,
		ActivatedAt: &previousActivation,
	}
	rollback := rollbackCandidateRecord(previous, "rollback", 8082, nil)
	if rollback.Source == nil || rollback.Source.CommitSHA != testCommitSHA ||
		rollback.ImageID != testImageID || rollback.ActivatedAt == nil ||
		!rollback.ActivatedAt.Equal(rollbackActivation) {

		t.Fatalf("rollback provenance = %#v", rollback)
	}
}
