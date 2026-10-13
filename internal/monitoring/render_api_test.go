package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type renderTestHarness struct {
	mu                    sync.Mutex
	service               RenderService
	head                  string
	deploys               []RenderDeploy
	triggerSHAs           []string
	deployListCalls       int
	advancedPostCount     int
	onDeployList          func(*renderTestHarness, int)
	postStatus            int
	postWithoutBody       bool
	renderServiceFailures int
}

func newRenderTestClient(t *testing.T, h *renderTestHarness) *RenderClient {
	t.Helper()
	renderServer := httptest.NewServer(http.HandlerFunc(h.renderHandler))
	t.Cleanup(renderServer.Close)
	githubServer := httptest.NewServer(http.HandlerFunc(h.githubHandler))
	t.Cleanup(githubServer.Close)

	client := NewRenderClient("render-test-key", "srv-test", "github-read-token")
	client.baseURL = renderServer.URL
	client.githubURL = githubServer.URL
	client.http = renderServer.Client()
	client.pollInterval = time.Millisecond
	return client
}

func newRenderTestHarness(head string, deploys ...RenderDeploy) *renderTestHarness {
	return &renderTestHarness{
		service: RenderService{
			ID: "srv-test", Type: "web_service",
			Repo: "https://github.com/example/private-casino.git", Branch: "main",
		},
		head:    head,
		deploys: append([]RenderDeploy(nil), deploys...),
	}
}

func (h *renderTestHarness) renderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer render-test-key" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/services/srv-test":
		if h.renderServiceFailures > 0 {
			h.renderServiceFailures--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(h.service)
	case r.Method == http.MethodGet && r.URL.Path == "/services/srv-test/deploys":
		h.deployListCalls++
		if h.onDeployList != nil {
			h.onDeployList(h, h.deployListCalls)
		}
		entries := make([]renderDeployListEntry, 0, len(h.deploys))
		for _, deploy := range h.deploys {
			entries = append(entries, renderDeployListEntry{Deploy: deploy})
		}
		_ = json.NewEncoder(w).Encode(entries)
	case r.Method == http.MethodPost && r.URL.Path == "/services/srv-test/deploys":
		var payload struct {
			CommitID string `json:"commitId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad payload", http.StatusBadRequest)
			return
		}
		h.triggerSHAs = append(h.triggerSHAs, payload.CommitID)
		status := h.postStatus
		if status == 0 {
			status = http.StatusCreated
		}
		if h.postWithoutBody {
			h.deploys = []RenderDeploy{renderTestDeploy("dep-accepted", "queued", payload.CommitID)}
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(RenderDeploy{
			ID:     "dep-triggered-" + string(rune('0'+len(h.triggerSHAs))),
			Status: "created",
		})
	default:
		http.NotFound(w, r)
	}
}

func (h *renderTestHarness) githubHandler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer github-read-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Header.Get("Accept") != "application/vnd.github+json" {
		http.Error(w, "missing accept", http.StatusBadRequest)
		return
	}
	if r.URL.Path != "/repos/example/private-casino/commits/main" {
		http.Error(w, "unexpected repo or branch", http.StatusNotFound)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_ = json.NewEncoder(w).Encode(struct {
		SHA string `json:"sha"`
	}{SHA: h.head})
}

func renderTestDeploy(id, status, sha string) RenderDeploy {
	deploy := RenderDeploy{
		ID:        id,
		Status:    status,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	deploy.Commit.ID = sha
	return deploy
}

func TestSnapshotVerifiesLatestBranchAndLiveCommit(t *testing.T) {
	sha := strings.Repeat("a", 40)
	h := newRenderTestHarness(sha, renderTestDeploy("dep-live", "live", sha))
	client := newRenderTestClient(t, h)

	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.Service.Branch != "main" || snapshot.LatestCommitSHA != sha ||
		snapshot.LiveCommitSHA != sha || snapshot.DeploymentState != DeploymentUpToDate {
		t.Fatalf("snapshot did not verify latest LIVE SHA: %+v", snapshot)
	}
}

func TestSnapshotSelectsNewestLiveDeploymentByRenderTimestamp(t *testing.T) {
	latest := strings.Repeat("a", 40)
	old := strings.Repeat("b", 40)
	olderLive := renderTestDeploy("dep-old-live", "live", old)
	olderLive.CreatedAt = "2026-01-01T00:00:00Z"
	newerLive := renderTestDeploy("dep-new-live", "live", latest)
	newerLive.CreatedAt = "2026-02-01T00:00:00Z"
	h := newRenderTestHarness(latest, olderLive, newerLive)
	client := newRenderTestClient(t, h)

	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot returned error: %v", err)
	}
	if snapshot.LiveCommitSHA != latest || snapshot.DeploymentState != DeploymentUpToDate ||
		snapshot.LatestDeploy == nil || snapshot.LatestDeploy.ID != "dep-new-live" {
		t.Fatalf("snapshot did not use the newest Render deployment timestamp: %+v", snapshot)
	}
}

func TestEnsureLatestWaitsForExistingDeployment(t *testing.T) {
	latest := strings.Repeat("a", 40)
	old := strings.Repeat("b", 40)
	h := newRenderTestHarness(latest,
		renderTestDeploy("dep-running", "build_in_progress", latest),
		renderTestDeploy("dep-old-live", "live", old),
	)
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if calls > 1 {
			h.deploys[0].Status = "live"
			h.deploys[0].FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureLatest returned error: %v", err)
	}
	if result.State != DeploymentUpToDate || result.TriggerCount != 0 || len(h.triggerSHAs) != 0 {
		t.Fatalf("existing deployment should be awaited, not duplicated: result=%+v triggers=%v", result, h.triggerSHAs)
	}
}

func TestEnsureLatestWaitsForActiveDeploymentWhenLatestOrderIsUnknown(t *testing.T) {
	latest := strings.Repeat("a", 40)
	active := renderTestDeploy("dep-running", "build_in_progress", latest)
	previousLive := renderTestDeploy("dep-old-live", "live", latest)
	previousLive.CreatedAt = ""
	h := newRenderTestHarness(latest, active, previousLive)
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if calls > 1 {
			h.deploys[0].Status = "live"
			h.deploys[1].CreatedAt = "2026-01-01T00:00:00Z"
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureLatest should await the known active deployment: %v", err)
	}
	if result.State != DeploymentUpToDate || result.TriggerCount != 0 || len(h.triggerSHAs) != 0 {
		t.Fatalf("active deployment should be awaited without an extra trigger: result=%+v triggers=%v", result, h.triggerSHAs)
	}
	if h.deployListCalls < 3 {
		t.Fatalf("expected deployment polling and post-completion verification, got %d list calls", h.deployListCalls)
	}
}

func TestEnsureLatestTriggersPinnedSHAAndWaitsForLive(t *testing.T) {
	latest := strings.Repeat("c", 40)
	old := strings.Repeat("d", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-old-live", "live", old))
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if len(h.triggerSHAs) == 1 && calls >= 2 {
			h.deploys = []RenderDeploy{renderTestDeploy("dep-triggered-1", "live", latest)}
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureLatest returned error: %v", err)
	}
	if result.State != DeploymentUpToDate || result.TriggerCount != 1 || len(h.triggerSHAs) != 1 ||
		h.triggerSHAs[0] != latest {
		t.Fatalf("expected pinned SHA deploy verified LIVE, got result=%+v triggers=%v", result, h.triggerSHAs)
	}
}

func TestEnsureLatestTracksAcceptedDeployWithoutResponseBody(t *testing.T) {
	latest := strings.Repeat("c", 40)
	old := strings.Repeat("d", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-old-live", "live", old))
	h.postStatus = http.StatusAccepted
	h.postWithoutBody = true
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if len(h.triggerSHAs) == 1 && calls >= 2 {
			h.deploys[0].Status = "live"
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureLatest should track an accepted response without a body: %v", err)
	}
	if result.State != DeploymentUpToDate || result.TriggerCount != 1 {
		t.Fatalf("accepted deployment was not verified LIVE: %+v", result)
	}
}

func TestEnsureLatestReportsDeploymentFailure(t *testing.T) {
	latest := strings.Repeat("e", 40)
	old := strings.Repeat("f", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-old-live", "live", old))
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if len(h.triggerSHAs) == 1 && calls >= 2 {
			h.deploys = []RenderDeploy{renderTestDeploy("dep-triggered-1", "build_failed", latest)}
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if !errors.Is(err, ErrDeploymentFailed) || result.TriggerCount != 1 {
		t.Fatalf("expected failed deployment without success result, got result=%+v err=%v", result, err)
	}
}

func TestEnsureLatestDoesNotRetryAmbiguousDeployPost(t *testing.T) {
	latest := strings.Repeat("a", 40)
	old := strings.Repeat("b", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-old-live", "live", old))
	h.postStatus = http.StatusServiceUnavailable
	client := newRenderTestClient(t, h)

	_, err := client.EnsureLatest(context.Background(), nil)
	var apiErr *RenderAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected sanitized Render error, got %v", err)
	}
	if len(h.triggerSHAs) != 1 {
		t.Fatalf("deploy POST must not be retried after an ambiguous response: %d requests", len(h.triggerSHAs))
	}
}

func TestEnsureLatestDeploysCommitThatAppearsDuringBuild(t *testing.T) {
	first := strings.Repeat("1", 40)
	second := strings.Repeat("2", 40)
	old := strings.Repeat("3", 40)
	h := newRenderTestHarness(first, renderTestDeploy("dep-old-live", "live", old))
	h.onDeployList = func(h *renderTestHarness, calls int) {
		switch {
		case len(h.triggerSHAs) == 1 && calls >= 2:
			h.head = second
			h.deploys = []RenderDeploy{renderTestDeploy("dep-triggered-1", "live", first)}
		case len(h.triggerSHAs) == 2 && calls >= 4:
			h.deploys = []RenderDeploy{renderTestDeploy("dep-triggered-2", "live", second)}
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("EnsureLatest returned error: %v", err)
	}
	if result.State != DeploymentUpToDate || result.TriggerCount != 2 ||
		len(h.triggerSHAs) != 2 || h.triggerSHAs[0] != first || h.triggerSHAs[1] != second {
		t.Fatalf("new commit appearing during deployment was not followed safely: result=%+v triggers=%v", result, h.triggerSHAs)
	}
}

func TestEnsureLatestStopsAfterBoundedFollowUpDeployments(t *testing.T) {
	initial := strings.Repeat("4", 40)
	live := strings.Repeat("5", 40)
	firstHead := strings.Repeat("6", 40)
	secondHead := strings.Repeat("7", 40)
	thirdHead := strings.Repeat("8", 40)
	h := newRenderTestHarness(initial, renderTestDeploy("dep-old-live", "live", live))
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if len(h.triggerSHAs) > h.advancedPostCount {
			h.advancedPostCount = len(h.triggerSHAs)
			if h.advancedPostCount == 1 {
				h.head = firstHead
			} else {
				h.head = secondHead
			}
			deployID := "dep-triggered-" + string(rune('0'+h.advancedPostCount))
			h.deploys = []RenderDeploy{renderTestDeploy(deployID, "live", h.triggerSHAs[len(h.triggerSHAs)-1])}
		}
		if h.advancedPostCount == maxDeployTriggers && calls > 4 {
			h.head = thirdHead
		}
	}
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected bounded incomplete result rather than an error: %v", err)
	}
	if result.State != DeploymentUpdatePending || result.TriggerCount != maxDeployTriggers ||
		len(h.triggerSHAs) != maxDeployTriggers {
		t.Fatalf("deployment loop was not bounded and explicit: result=%+v triggers=%v", result, h.triggerSHAs)
	}
}

func TestEnsureLatestDoesNotTriggerForUnknownStatus(t *testing.T) {
	latest := strings.Repeat("7", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-unknown", "future_status", latest))
	client := newRenderTestClient(t, h)

	result, err := client.EnsureLatest(context.Background(), nil)
	if !errors.Is(err, ErrDeploymentUnknown) || len(h.triggerSHAs) != 0 ||
		result.State != DeploymentUnknown {
		t.Fatalf("unknown status should stop without trigger or success: result=%+v triggers=%v err=%v", result, h.triggerSHAs, err)
	}
}

func TestEnsureLatestReturnsUnknownOnTimeout(t *testing.T) {
	latest := strings.Repeat("8", 40)
	old := strings.Repeat("9", 40)
	h := newRenderTestHarness(latest, renderTestDeploy("dep-old-live", "live", old))
	h.onDeployList = func(h *renderTestHarness, calls int) {
		if len(h.triggerSHAs) == 1 {
			h.deploys = []RenderDeploy{renderTestDeploy("dep-triggered-1", "build_in_progress", latest)}
		}
	}
	client := newRenderTestClient(t, h)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := client.EnsureLatest(ctx, nil)
	if !errors.Is(err, ErrDeploymentTimeout) || result.State == DeploymentUpToDate {
		t.Fatalf("timeout must not be reported as latest: result=%+v err=%v", result, err)
	}
}

func TestSnapshotRejectsMismatchedServiceID(t *testing.T) {
	sha := strings.Repeat("a", 40)
	h := newRenderTestHarness(sha, renderTestDeploy("dep-live", "live", sha))
	h.service.ID = "srv-someone-else"
	client := newRenderTestClient(t, h)

	_, err := client.Snapshot(context.Background())
	if !errors.Is(err, ErrServiceIDMismatch) {
		t.Fatalf("expected service ID mismatch, got %v", err)
	}
}

func TestSnapshotRetriesTransientReadFailures(t *testing.T) {
	sha := strings.Repeat("a", 40)
	h := newRenderTestHarness(sha, renderTestDeploy("dep-live", "live", sha))
	h.renderServiceFailures = 2
	client := newRenderTestClient(t, h)

	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot should retry transient GET failures: %v", err)
	}
	if snapshot.DeploymentState != DeploymentUpToDate || h.renderServiceFailures != 0 {
		t.Fatalf("transient read retries did not recover: snapshot=%+v remaining=%d", snapshot, h.renderServiceFailures)
	}
}

func TestRenderAPIErrorDoesNotExposeCredentialsOrBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sensitive response render-test-key github-read-token", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewRenderClient("render-test-key", "srv-test", "github-read-token")
	client.baseURL = server.URL
	client.http = server.Client()
	_, err := client.Snapshot(context.Background())
	if err == nil || strings.Contains(err.Error(), "sensitive") ||
		strings.Contains(err.Error(), "render-test-key") || strings.Contains(err.Error(), "github-read-token") {
		t.Fatalf("Render API error leaked a response body or credential: %v", err)
	}
}

func TestGitHubAPIErrorDoesNotExposeCredentialsOrBody(t *testing.T) {
	head := strings.Repeat("a", 40)
	h := newRenderTestHarness(head, renderTestDeploy("dep-live", "live", head))
	client := newRenderTestClient(t, h)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sensitive response github-read-token", http.StatusUnauthorized)
	}))
	defer server.Close()
	client.githubURL = server.URL

	_, err := client.Snapshot(context.Background())
	var apiErr *GitHubAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized ||
		strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "github-read-token") {
		t.Fatalf("GitHub API error leaked response body or credential: %v", err)
	}
}

func TestGitHubRepositoryParts(t *testing.T) {
	for _, repository := range []string{
		"https://github.com/owner/repository.git",
		"git@github.com:owner/repository.git",
	} {
		owner, repo, err := githubRepositoryParts(repository)
		if err != nil || owner != "owner" || repo != "repository" {
			t.Errorf("githubRepositoryParts(%q) = %q, %q, %v", repository, owner, repo, err)
		}
	}
	for _, repository := range []string{
		"https://attacker.example/owner/repository",
		"https://github.com/owner/repository/extra",
		"https://user:password@github.com/owner/repository",
		"git@attacker.example:owner/repository.git",
	} {
		if _, _, err := githubRepositoryParts(repository); !errors.Is(err, ErrGitSourceMissing) {
			t.Errorf("expected unsupported repository %q to be rejected, got %v", repository, err)
		}
	}
}
