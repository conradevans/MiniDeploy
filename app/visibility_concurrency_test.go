package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func newVisibilityConcurrencyStore(
	t *testing.T,
	record DeploymentRecord,
) *JSONStore {
	t.Helper()

	metadata := NewJSONStore(
		filepath.Join(t.TempDir(), "deployments.json"),
	)
	if err := metadata.Save(record); err != nil {
		t.Fatalf("Save(initial) error: %v", err)
	}
	return metadata
}

func visibilityConcurrencyRecord(app string) DeploymentRecord {
	return DeploymentRecord{
		App:           app,
		RepoURL:       "https://github.com/example/" + app + ".git",
		Container:     app + "-old-container",
		Image:         app + ":old",
		Port:          8100,
		ContainerPort: 3000,
		HealthPath:    "/health",
		Strategy:      deploymentStrategyNodeExpress,
		GuestVisible:  false,
	}
}

func requireVisibilityConcurrencyRecord(
	t *testing.T,
	metadata *JSONStore,
	want DeploymentRecord,
) {
	t.Helper()

	got, err := metadata.Get(want.App)
	if err != nil {
		t.Fatalf("Get(%q) error: %v", want.App, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored deployment = %#v; want %#v", got, want)
	}
}

func TestVisibilityDuringRedeployMergesLatestState(t *testing.T) {
	initial := visibilityConcurrencyRecord("redeploy-visibility-app")
	metadata := newVisibilityConcurrencyStore(t, initial)

	// The lifecycle snapshot is intentionally captured before the visibility
	// change, exactly as a redeploy does before its final metadata save.
	candidate := initial
	candidate.Container = initial.App + "-candidate-container"
	candidate.Image = initial.App + ":candidate"
	candidate.Port = 8200

	if _, err := metadata.UpdateGuestVisibility(initial.App, true); err != nil {
		t.Fatalf("UpdateGuestVisibility() error: %v", err)
	}
	if err := metadata.Save(candidate); err != nil {
		t.Fatalf("Save(candidate) error: %v", err)
	}

	candidate.GuestVisible = true
	requireVisibilityConcurrencyRecord(t, metadata, candidate)
}

func TestRedeployBeforeVisibilityDoesNotRestoreStaleMetadata(t *testing.T) {
	initial := visibilityConcurrencyRecord("visibility-after-redeploy-app")
	metadata := newVisibilityConcurrencyStore(t, initial)

	candidate := initial
	candidate.Container = initial.App + "-candidate-container"
	candidate.Image = initial.App + ":candidate"
	candidate.Port = 8300
	if err := metadata.Save(candidate); err != nil {
		t.Fatalf("Save(candidate) error: %v", err)
	}

	updated, err := metadata.UpdateGuestVisibility(initial.App, true)
	if err != nil {
		t.Fatalf("UpdateGuestVisibility() error: %v", err)
	}
	candidate.GuestVisible = true
	if !reflect.DeepEqual(updated, candidate) {
		t.Fatalf("visibility update changed lifecycle metadata: %#v; want %#v", updated, candidate)
	}
	requireVisibilityConcurrencyRecord(t, metadata, candidate)
}

func TestVisibilityDuringWebhookRedeployUsesLatestState(t *testing.T) {
	initial := visibilityConcurrencyRecord("webhook-visibility-app")
	metadata := newVisibilityConcurrencyStore(t, initial)

	selected, found := deploymentForWebhook(
		[]DeploymentRecord{initial},
		initial.RepoURL,
	)
	if !found {
		t.Fatal("deploymentForWebhook() did not select deployment")
	}

	if _, err := metadata.UpdateGuestVisibility(initial.App, true); err != nil {
		t.Fatalf("UpdateGuestVisibility() error: %v", err)
	}
	selected.Container = initial.App + "-webhook-container"
	selected.Image = initial.App + ":webhook"
	selected.Port = 8400
	if err := metadata.Save(selected); err != nil {
		t.Fatalf("Save(webhook candidate) error: %v", err)
	}

	selected.GuestVisible = true
	requireVisibilityConcurrencyRecord(t, metadata, selected)
}

func TestVisibilityDuringRollbackUsesLatestState(t *testing.T) {
	current := visibilityConcurrencyRecord("rollback-visibility-app")
	metadata := newVisibilityConcurrencyStore(t, current)

	previous := DeploymentVersion{
		App:                current.App,
		RepoURL:            current.RepoURL,
		Container:          current.App + "-previous-container",
		Image:              current.App + ":previous",
		Port:               8050,
		ContainerPort:      current.ContainerPort,
		HealthPath:         current.HealthPath,
		Strategy:           current.Strategy,
		PackageManager:     current.PackageManager,
		PackageInstallMode: current.PackageInstallMode,
	}
	rollback := previous.RecordWithFallback(current)

	if _, err := metadata.UpdateGuestVisibility(current.App, true); err != nil {
		t.Fatalf("UpdateGuestVisibility() error: %v", err)
	}
	if err := metadata.Save(rollback); err != nil {
		t.Fatalf("Save(rollback) error: %v", err)
	}

	rollback.GuestVisible = true
	requireVisibilityConcurrencyRecord(t, metadata, rollback)
}

func TestVisibilityAndDeleteAreAtomic(t *testing.T) {
	t.Run("delete wins", func(t *testing.T) {
		initial := visibilityConcurrencyRecord("delete-before-visibility-app")
		metadata := newVisibilityConcurrencyStore(t, initial)

		if _, err := metadata.Get(initial.App); err != nil {
			t.Fatalf("capture stale deployment snapshot: %v", err)
		}
		if err := metadata.Delete(initial.App); err != nil {
			t.Fatalf("Delete() error: %v", err)
		}
		if _, err := metadata.UpdateGuestVisibility(initial.App, true); !errors.Is(err, ErrDeploymentNotFound) {
			t.Fatalf("UpdateGuestVisibility() error = %v; want ErrDeploymentNotFound", err)
		}
		if _, err := metadata.Get(initial.App); !errors.Is(err, ErrDeploymentNotFound) {
			t.Fatalf("deleted deployment was recreated: %v", err)
		}
	})

	t.Run("visibility wins then delete", func(t *testing.T) {
		initial := visibilityConcurrencyRecord("visibility-before-delete-app")
		metadata := newVisibilityConcurrencyStore(t, initial)

		if _, err := metadata.UpdateGuestVisibility(initial.App, true); err != nil {
			t.Fatalf("UpdateGuestVisibility() error: %v", err)
		}
		if err := metadata.Delete(initial.App); err != nil {
			t.Fatalf("Delete() error: %v", err)
		}
		if _, err := metadata.Get(initial.App); !errors.Is(err, ErrDeploymentNotFound) {
			t.Fatalf("deleted deployment remains stored: %v", err)
		}
	})
}

func TestVisibilityAndMiniBaseMetadataPreserveEachOther(t *testing.T) {
	initial := visibilityConcurrencyRecord("minibase-visibility-app")
	metadata := newVisibilityConcurrencyStore(t, initial)

	miniBaseUpdate := initial
	miniBaseUpdate.DatabaseAttachments = []DatabaseAttachmentRecord{{
		AttachmentID: "attachment-current",
		DatabaseID:   "database-current",
		DisplayName:  "Current Database",
		BindingName:  miniBaseBindingPrimary,
	}}

	if _, err := metadata.UpdateGuestVisibility(initial.App, true); err != nil {
		t.Fatalf("UpdateGuestVisibility(true) error: %v", err)
	}
	if err := metadata.Save(miniBaseUpdate); err != nil {
		t.Fatalf("Save(MiniBase metadata) error: %v", err)
	}

	miniBaseUpdate.GuestVisible = true
	requireVisibilityConcurrencyRecord(t, metadata, miniBaseUpdate)

	updated, err := metadata.UpdateGuestVisibility(initial.App, false)
	if err != nil {
		t.Fatalf("UpdateGuestVisibility(false) error: %v", err)
	}
	miniBaseUpdate.GuestVisible = false
	if !reflect.DeepEqual(updated, miniBaseUpdate) {
		t.Fatalf("visibility update changed MiniBase metadata: %#v; want %#v", updated, miniBaseUpdate)
	}
	requireVisibilityConcurrencyRecord(t, metadata, miniBaseUpdate)
}
