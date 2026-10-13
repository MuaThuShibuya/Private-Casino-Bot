package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderSnapshotAndTriggerDeploy(t *testing.T) {
	triggered := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing Render API bearer authentication")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/services/srv-test":
			_ = json.NewEncoder(w).Encode(RenderService{
				ID: "srv-test", Name: "casino", Type: "web_service",
				Repo: "https://github.com/example/casino", Branch: "main",
				AutoDeploy: "yes", DashboardURL: "https://dashboard.render.com/web/srv-test",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/services/srv-test/deploys":
			_ = json.NewEncoder(w).Encode([]renderDeployListEntry{{
				Deploy: RenderDeploy{ID: "dep-live", Status: "live", Commit: struct {
					ID        string `json:"id"`
					Message   string `json:"message"`
					CreatedAt string `json:"createdAt"`
				}{ID: "abc123"}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/services/srv-test/deploys":
			triggered = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload) != 0 {
				t.Errorf("expected empty JSON deploy payload, got %v (err %v)", payload, err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(RenderDeploy{ID: "dep-new", Status: "created"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewRenderClient("test-key", "srv-test")
	client.baseURL = server.URL
	client.http = server.Client()

	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.Service.Branch != "main" || len(snapshot.Deploys) != 1 || snapshot.Deploys[0].Status != "live" {
		t.Fatalf("unexpected Render snapshot: %+v", snapshot)
	}

	deploy, err := client.TriggerDeploy(context.Background())
	if err != nil {
		t.Fatalf("TriggerDeploy returned error: %v", err)
	}
	if !triggered || deploy.ID != "dep-new" {
		t.Fatalf("deploy was not triggered: triggered=%v deploy=%+v", triggered, deploy)
	}
}

func TestRenderTriggerRejectsActiveDeploy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/srv-test":
			_ = json.NewEncoder(w).Encode(RenderService{
				ID: "srv-test", Repo: "https://github.com/example/casino", Branch: "main",
			})
		case "/services/srv-test/deploys":
			_ = json.NewEncoder(w).Encode([]renderDeployListEntry{
				{Deploy: RenderDeploy{ID: "dep-live", Status: "live"}},
				{Deploy: RenderDeploy{ID: "dep-running", Status: "build_in_progress"}},
			})
		default:
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewRenderClient("test-key", "srv-test")
	client.baseURL = server.URL
	client.http = server.Client()
	_, err := client.TriggerDeploy(context.Background())
	if !errors.Is(err, ErrDeployInProgress) {
		t.Fatalf("expected in-progress guard error, got %v", err)
	}
}

func TestRenderTriggerRequiresGitSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/services/srv-test" {
			_ = json.NewEncoder(w).Encode(RenderService{ID: "srv-test"})
			return
		}
		_ = json.NewEncoder(w).Encode([]renderDeployListEntry{})
	}))
	defer server.Close()

	client := NewRenderClient("test-key", "srv-test")
	client.baseURL = server.URL
	client.http = server.Client()
	_, err := client.TriggerDeploy(context.Background())
	if !errors.Is(err, ErrGitSourceMissing) {
		t.Fatalf("expected missing Git source error, got %v", err)
	}
}

func TestRenderAPIErrorDoesNotExposeResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sensitive internal response", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewRenderClient("test-key", "srv-test")
	client.baseURL = server.URL
	client.http = server.Client()
	_, err := client.Snapshot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected sanitized HTTP status error, got %v", err)
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("Render API error leaked sensitive content: %v", err)
	}
}
