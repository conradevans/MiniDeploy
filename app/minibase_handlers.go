package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

var (
	databaseAttachmentMu       sync.Mutex
	databaseAttachmentRedeploy = safeRedeploy
)

type databaseAttachmentRequest struct {
	Mode        string `json:"mode"`
	DisplayName string `json:"displayName"`
	DatabaseID  string `json:"databaseId"`
}

type databaseAttachmentStatusResponse struct {
	Supported  bool                      `json:"supported"`
	Attachment *DatabaseAttachmentRecord `json:"attachment"`
}

func availableMiniBaseDatabasesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), miniBaseRequestTimeout)
	defer cancel()
	databases, err := miniBaseClient.ListDatabases(ctx)
	if err != nil {
		http.Error(w, "MiniBase unavailable", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, databases)
}

func deploymentDatabaseHandler(w http.ResponseWriter, r *http.Request) {
	record, err := getDeployment(r.PathValue("app"))
	if errors.Is(err, ErrDeploymentNotFound) {
		http.Error(w, "deployment not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load deployment", http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodGet {
		attachment, attached, attachmentErr := currentDatabaseAttachment(record)
		if attachmentErr != nil {
			http.Error(w, "invalid deployment database metadata", http.StatusInternalServerError)
			return
		}
		result := databaseAttachmentStatusResponse{Supported: deploymentSupportsMiniBase(record)}
		if attached {
			result.Attachment = &attachment
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	attachMiniBaseDatabase(w, r, record)
}

func attachMiniBaseDatabase(w http.ResponseWriter, r *http.Request, record DeploymentRecord) {
	databaseAttachmentMu.Lock()
	defer databaseAttachmentMu.Unlock()

	latest, err := getDeployment(record.App)
	if err != nil {
		http.Error(w, "failed to load deployment", http.StatusInternalServerError)
		return
	}
	if !deploymentSupportsMiniBase(latest) {
		http.Error(w, "this deployment strategy does not support MiniBase", http.StatusUnprocessableEntity)
		return
	}
	if _, attached, err := currentDatabaseAttachment(latest); err != nil {
		http.Error(w, "invalid deployment database metadata", http.StatusInternalServerError)
		return
	} else if attached {
		http.Error(w, "a primary MiniBase database is already attached", http.StatusConflict)
		return
	}
	environment, err := runtimeEnvironmentStore.Load(latest.App)
	if err != nil {
		http.Error(w, "failed to inspect runtime environment", http.StatusInternalServerError)
		return
	}
	if _, conflict := environment["DATABASE_URL"]; conflict {
		http.Error(w, "remove the existing DATABASE_URL before attaching MiniBase", http.StatusConflict)
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var input databaseAttachmentRequest
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(w, "request must contain one JSON value", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*miniBaseRequestTimeout)
	defer cancel()
	var database miniBaseDatabase
	switch input.Mode {
	case "create":
		if strings.TrimSpace(input.DisplayName) == "" || input.DatabaseID != "" {
			http.Error(w, "create mode requires displayName only", http.StatusBadRequest)
			return
		}
		database, err = miniBaseClient.CreateDatabase(ctx, strings.TrimSpace(input.DisplayName))
	case "attach":
		if input.DisplayName != "" || !miniBaseDatabaseIDPattern.MatchString(input.DatabaseID) {
			http.Error(w, "attach mode requires databaseId only", http.StatusBadRequest)
			return
		}
		var databases []miniBaseDatabase
		databases, err = miniBaseClient.ListDatabases(ctx)
		if err == nil {
			for _, candidate := range databases {
				if candidate.ID == input.DatabaseID && !candidate.Attached {
					database = candidate
					break
				}
			}
			if database.ID == "" {
				err = fmt.Errorf("database is unavailable for attachment")
			}
		}
	default:
		http.Error(w, "mode must be create or attach", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "MiniBase database operation failed", http.StatusBadGateway)
		return
	}

	attachment, err := miniBaseClient.CreateAttachment(ctx, database.ID, latest.App)
	if err != nil {
		http.Error(w, "MiniBase attachment failed; the database was preserved", http.StatusBadGateway)
		return
	}
	latest.DatabaseAttachments = []DatabaseAttachmentRecord{{
		AttachmentID: attachment.ID,
		DatabaseID:   database.ID,
		DisplayName:  database.DisplayName,
		BindingName:  miniBaseBindingPrimary,
	}}
	if err := store.Save(latest); err != nil {
		if detachErr := miniBaseClient.DeleteAttachment(ctx, attachment.ID); detachErr != nil {
			http.Error(w, "database attachment was created, but local metadata save and automatic detachment failed; the database was preserved", http.StatusInternalServerError)
			return
		}
		http.Error(w, "failed to save database attachment metadata; the attachment was removed and the database was preserved", http.StatusInternalServerError)
		return
	}
	updated, err := databaseAttachmentRedeploy(latest, nil)
	if err != nil {
		deploymentEvent(latest.App, "ERROR: MiniBase attached, but deployment update failed; the current release remains live.")
		http.Error(w, "database attached but deployment update failed; retry redeploy", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, deploymentResponse(updated))
}
