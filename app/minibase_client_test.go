package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testMiniBaseToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func miniBaseTokenFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "integration-token")
	if err := os.WriteFile(path, []byte(testMiniBaseToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMiniBaseHTTPClientLifecycleAndAuthentication(t *testing.T) {
	databaseID := "database_0123456789abcdef0123456789abcdef"
	attachmentID := "attachment_0123456789abcdef0123456789abcdef"
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testMiniBaseToken {
			t.Error("request omitted the integration bearer token")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/integrations/minideploy/databases":
			fmt.Fprintf(w, `[{"id":%q,"displayName":"Scheduler","status":"ready","attached":false}]`, databaseID)
		case "POST /api/v1/integrations/minideploy/databases":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":%q,"displayName":"Scheduler","status":"ready","attached":false}`, databaseID)
		case "POST /api/v1/integrations/minideploy/attachments":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":%q,"databaseId":%q,"consumerType":"minideploy","consumerRef":"scheduler","bindingName":"primary","createdAt":"2026-09-01T00:00:00Z","updatedAt":"2026-09-01T00:00:00Z"}`, attachmentID, databaseID)
		case "GET /api/v1/integrations/minideploy/attachments/" + attachmentID + "/binding":
			fmt.Fprintf(w, `{"databaseId":%q,"engine":"postgresql","host":"minibase-postgres","port":5432,"database":"mb_db_0123456789abcdef0123456789abcdef","username":"mb_role_0123456789abcdef0123456789abcdef","password":"p@ss:/?#[]","dockerNetwork":"reactorlab-data"}`, databaseID)
		case "DELETE /api/v1/integrations/minideploy/attachments/" + attachmentID:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := newMiniBaseHTTPClient(server.URL, miniBaseTokenFile(t), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if databases, err := client.ListDatabases(ctx); err != nil || len(databases) != 1 || databases[0].ID != databaseID {
		t.Fatalf("ListDatabases() returned invalid safe data: count=%d err=%v", len(databases), err)
	}
	if database, err := client.CreateDatabase(ctx, "Scheduler"); err != nil || database.ID != databaseID {
		t.Fatalf("CreateDatabase() returned unexpected metadata: err=%v", err)
	}
	attachment, err := client.CreateAttachment(ctx, databaseID, "scheduler")
	if err != nil || attachment.ID != attachmentID {
		t.Fatalf("CreateAttachment() returned unexpected metadata: err=%v", err)
	}
	binding, err := client.ResolveBinding(ctx, attachmentID)
	if err != nil || binding.DatabaseID != databaseID || binding.DockerNetwork != reactorLabDataNetwork {
		t.Fatalf("ResolveBinding() returned invalid structured binding: err=%v", err)
	}
	if err := client.DeleteAttachment(ctx, attachmentID); err != nil {
		t.Fatalf("DeleteAttachment() error = %v", err)
	}
	if len(calls) != 5 {
		t.Fatalf("integration call count = %d", len(calls))
	}
}

func TestMiniBaseHTTPClientRejectsUnsafeConfigurationAndToken(t *testing.T) {
	for _, rawURL := range []string{
		"", "https://127.0.0.1:9100", "http://0.0.0.0:9100", "http://192.0.2.1:9100",
		"http://localhost:9100", "http://127.0.0.1", "http://127.0.0.1:http",
		"http://user@127.0.0.1:9100", "http://127.0.0.1:9100/path", "http://127.0.0.1:9100/?query=yes",
	} {
		if _, err := newMiniBaseHTTPClient(rawURL, "/token", nil); err == nil {
			t.Fatalf("unsafe MiniBase URL %q accepted", rawURL)
		}
	}

	path := miniBaseTokenFile(t)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client, err := newMiniBaseHTTPClient(server.URL, path, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListDatabases(context.Background()); err == nil {
		t.Fatal("client accepted a group/world-readable integration token")
	}
}

func TestMiniBaseHTTPClientBoundsTimeoutsAndSanitizesErrors(t *testing.T) {
	tokenPath := miniBaseTokenFile(t)
	sentinel := "binding-response-secret-must-not-leak"
	for name, handler := range map[string]http.HandlerFunc{
		"non-2xx": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, sentinel, http.StatusInternalServerError)
		},
		"malformed": func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"password":"` + sentinel + `"}`))
		},
		"oversized": func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(strings.Repeat("x", miniBaseResponseLimit+1)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			client, err := newMiniBaseHTTPClient(server.URL, tokenPath, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.ListDatabases(context.Background())
			if err == nil {
				t.Fatal("unsafe MiniBase response accepted")
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatal("MiniBase error exposed response-body content")
			}
		})
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`[]`))
	}))
	defer slow.Close()
	client, err := newMiniBaseHTTPClient(slow.URL, tokenPath, &http.Client{Timeout: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListDatabases(context.Background()); err == nil || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatal("MiniBase timeout did not fail safely")
	}

	unavailable, err := newMiniBaseHTTPClient("http://127.0.0.1:1", tokenPath, &http.Client{Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unavailable.ListDatabases(context.Background()); err == nil || err.Error() != "MiniBase is unavailable" {
		t.Fatalf("unavailable MiniBase returned an unsafe or unexpected error: %v", err)
	}
}
