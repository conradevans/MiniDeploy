package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxDeploymentBranchLength = 1024

var (
	gitObjectIDPattern          = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
	dockerImageIDPattern        = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)
	dockerImageTargetPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*(:[A-Za-z0-9][A-Za-z0-9_.-]*)?$`)
	githubRepositoryPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	deploymentActivationTime    = func() time.Time {
		return time.Now().UTC()
	}
)

type DeploymentSourceRecord struct {
	Provider     string `json:"provider,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Branch       string `json:"branch,omitempty"`
	RequestedRef string `json:"requestedRef,omitempty"`
	CommitSHA    string `json:"commitSha,omitempty"`
}

type deploymentCommandRunner func(
	dir string,
	name string,
	args ...string,
) (string, error)

func inspectDeploymentSource(
	checkoutPath string,
	repositoryURL string,
) (*DeploymentSourceRecord, error) {
	return inspectDeploymentSourceWithRunner(
		checkoutPath,
		repositoryURL,
		runCommand,
	)
}

func inspectDeploymentSourceWithRunner(
	checkoutPath string,
	repositoryURL string,
	runner deploymentCommandRunner,
) (*DeploymentSourceRecord, error) {
	commitOutput, err := runner(
		checkoutPath,
		"git",
		"rev-parse",
		"--verify",
		"HEAD",
	)
	if err != nil {
		return nil, fmt.Errorf("inspect Git HEAD: %w", err)
	}

	commitSHA := strings.TrimSpace(commitOutput)
	if !gitObjectIDPattern.MatchString(commitSHA) {
		return nil, fmt.Errorf("inspect Git HEAD: invalid commit identity")
	}

	branch := ""
	branchOutput, branchErr := runner(
		checkoutPath,
		"git",
		"symbolic-ref",
		"--quiet",
		"--short",
		"HEAD",
	)
	if branchErr == nil {
		branch = strings.TrimSuffix(
			strings.TrimSuffix(branchOutput, "\n"),
			"\r",
		)
		if err := validateDeploymentBranch(branch); err != nil {
			return nil, err
		}
	} else {
		var exitError interface{ ExitCode() int }
		if !errors.As(branchErr, &exitError) || exitError.ExitCode() != 1 {
			return nil, fmt.Errorf("inspect Git branch: %w", branchErr)
		}
	}

	provider, repository := githubRepositoryIdentity(repositoryURL)
	return &DeploymentSourceRecord{
		Provider:   provider,
		Repository: repository,
		Branch:     branch,
		CommitSHA:  strings.ToLower(commitSHA),
	}, nil
}

func validateDeploymentBranch(branch string) error {
	if branch == "" {
		return fmt.Errorf("inspect Git branch: empty branch identity")
	}
	if len(branch) > maxDeploymentBranchLength || !utf8.ValidString(branch) {
		return fmt.Errorf("inspect Git branch: invalid branch identity")
	}
	for _, character := range branch {
		if unicode.IsControl(character) {
			return fmt.Errorf("inspect Git branch: invalid branch identity")
		}
	}
	return nil
}

func githubRepositoryIdentity(repositoryURL string) (string, string) {
	path := ""

	if strings.HasPrefix(repositoryURL, "git@github.com:") {
		path = strings.TrimPrefix(repositoryURL, "git@github.com:")
		if strings.ContainsAny(path, "?#") {
			return "", ""
		}
	} else {
		parsed, err := url.Parse(repositoryURL)
		if err != nil ||
			(parsed.Scheme != "https" && parsed.Scheme != "ssh") ||
			!strings.EqualFold(parsed.Hostname(), "github.com") ||
			parsed.Port() != "" ||
			parsed.RawPath != "" {

			return "", ""
		}
		if parsed.Scheme == "ssh" && parsed.User.Username() != "git" {
			return "", ""
		}
		path = parsed.Path
	}

	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !validGitHubRepositoryPart(parts[0]) ||
		!validGitHubRepositoryPart(parts[1]) {

		return "", ""
	}

	return "github", parts[0] + "/" + parts[1]
}

func validGitHubRepositoryPart(part string) bool {
	return len(part) <= 100 && part != "." && part != ".." &&
		githubRepositoryPartPattern.MatchString(part)
}

func inspectBuiltImageID(image string) (string, error) {
	if image == "" || len(image) > 255 || !strings.HasPrefix(image, "minideploy-") || !dockerImageTargetPattern.MatchString(image) {
		return "", fmt.Errorf("inspect built Docker image: invalid image name")
	}

	output, err := runCommand(
		"",
		"docker",
		"image",
		"inspect",
		"--format",
		"{{.Id}}",
		image,
	)
	if err != nil {
		return "", fmt.Errorf("inspect built Docker image: %w", err)
	}

	imageID := strings.TrimSpace(output)
	if !dockerImageIDPattern.MatchString(imageID) {
		return "", fmt.Errorf("inspect built Docker image: invalid image identity")
	}
	return strings.ToLower(imageID), nil
}

func cloneDeploymentSource(
	source *DeploymentSourceRecord,
) *DeploymentSourceRecord {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneActivationTime(activatedAt *time.Time) *time.Time {
	if activatedAt == nil {
		return nil
	}
	cloned := *activatedAt
	return &cloned
}

func activateDeploymentRecord(record DeploymentRecord) DeploymentRecord {
	activatedAt := deploymentActivationTime().UTC()
	record.ActivatedAt = &activatedAt
	return record
}
