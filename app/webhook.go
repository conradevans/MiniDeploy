package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

const maxWebhookBody = 1024 * 1024

type githubPushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

func githubWebhookHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	secret := os.Getenv(
		"MINIDEPLOY_GITHUB_WEBHOOK_SECRET",
	)

	if secret == "" {
		http.Error(
			w,
			"webhook secret is not configured",
			http.StatusServiceUnavailable,
		)
		return
	}

	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxWebhookBody,
	)

	body, err := io.ReadAll(r.Body)

	if err != nil {
		http.Error(
			w,
			"invalid webhook body",
			http.StatusBadRequest,
		)
		return
	}

	if !validGitHubSignature(
		body,
		r.Header.Get("X-Hub-Signature-256"),
		secret,
	) {
		http.Error(
			w,
			"invalid webhook signature",
			http.StatusUnauthorized,
		)
		return
	}

	event := r.Header.Get("X-GitHub-Event")

	if event == "ping" {
		writeJSON(
			w,
			http.StatusOK,
			map[string]string{
				"status": "ok",
			},
		)
		return
	}

	if event != "push" {
		writeJSON(
			w,
			http.StatusAccepted,
			map[string]string{
				"status": "ignored",
			},
		)
		return
	}

	var payload githubPushPayload

	if err := json.Unmarshal(
		body,
		&payload,
	); err != nil {
		http.Error(
			w,
			"invalid GitHub payload",
			http.StatusBadRequest,
		)
		return
	}

	// MiniDeploy currently deploys the main branch.
	if payload.Ref != "refs/heads/main" {
		writeJSON(
			w,
			http.StatusAccepted,
			map[string]string{
				"status": "ignored",
			},
		)
		return
	}

	records, err := store.List()

	if err != nil {
		http.Error(
			w,
			"failed to load deployments",
			http.StatusInternalServerError,
		)
		return
	}

	deployment, found := deploymentForWebhook(
		records,
		payload.Repository.CloneURL,
	)

	if !found {
		writeJSON(
			w,
			http.StatusAccepted,
			map[string]string{
				"status": "repository not deployed",
			},
		)
		return
	}

	// Respond immediately so GitHub does not have to wait
	// for Docker clone/build/health checks to finish.
	writeJSON(
		w,
		http.StatusAccepted,
		map[string]string{
			"status": "deployment queued",
			"app":    deployment.App,
		},
	)

	go func(record DeploymentRecord) {
		log.Printf(
			"GitHub push received for %s; starting redeploy",
			record.App,
		)

		newRecord, err := safeRedeploy(record, nil)

		if err != nil {
			log.Printf(
				"GitHub redeploy failed for %s: %v",
				record.App,
				err,
			)
			return
		}

		log.Printf(
			"GitHub push deployment completed for %s using %s",
			newRecord.App,
			newRecord.Image,
		)
	}(deployment)
}

func deploymentForWebhook(
	records []DeploymentRecord,
	cloneURL string,
) (DeploymentRecord, bool) {
	normalizedCloneURL := normalizeRepoURL(cloneURL)

	for _, record := range records {
		if normalizeRepoURL(record.RepoURL) !=
			normalizedCloneURL {
			continue
		}

		return normalizeDeploymentRecord(
			record,
		), true
	}

	return DeploymentRecord{}, false
}

func validGitHubSignature(
	body []byte,
	signature string,
	secret string,
) bool {
	const prefix = "sha256="

	if !strings.HasPrefix(
		signature,
		prefix,
	) {
		return false
	}

	provided, err := hex.DecodeString(
		strings.TrimPrefix(
			signature,
			prefix,
		),
	)

	if err != nil {
		return false
	}

	mac := hmac.New(
		sha256.New,
		[]byte(secret),
	)

	_, _ = mac.Write(body)

	expected := mac.Sum(nil)

	return hmac.Equal(
		expected,
		provided,
	)
}

func normalizeRepoURL(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	repoURL = strings.TrimSuffix(repoURL, "/")
	repoURL = strings.TrimSuffix(repoURL, ".git")

	return strings.ToLower(repoURL)
}
