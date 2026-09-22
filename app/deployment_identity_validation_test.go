package main

import (
	"strings"
	"testing"
)

func TestDeploymentBranchValidationRejectsUnsafeOrUnboundedValues(t *testing.T) {
	for _, branch := range []string{
		"feature/unsafe\nvalue",
		strings.Repeat("a", maxDeploymentBranchLength+1),
	} {
		if err := validateDeploymentBranch(branch); err == nil {
			t.Fatalf("unsafe branch was accepted: %q", branch)
		}
	}
}

func TestInspectBuiltImageIDRejectsMalformedIdentityAndTarget(t *testing.T) {
	commandCalled := false
	replaceCommandRunnerForTest(
		t,
		func(string, string, ...string) (string, error) {
			commandCalled = true
			return "sha256:short\n", nil
		},
	)

	if _, err := inspectBuiltImageID("--untrusted-option"); err == nil {
		t.Fatal("option-like image target was accepted")
	}
	if commandCalled {
		t.Fatal("invalid image target reached Docker")
	}

	if _, err := inspectBuiltImageID("minideploy-app:test"); err == nil {
		t.Fatal("malformed Docker image ID was accepted")
	}
	if !commandCalled {
		t.Fatal("valid generated image target was not inspected")
	}
}
