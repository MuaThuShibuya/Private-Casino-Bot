package monitoring

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	renderAPIBaseURL      = "https://api.render.com/v1"
	githubAPIBaseURL      = "https://api.github.com"
	maxRenderBody         = 1 << 20
	maxDeployPages        = 5
	maxDeployTriggers     = 2
	maxMissingDeployPolls = 6
	maxReadAttempts       = 3
	defaultDeployPollTime = 5 * time.Second
)

var (
	ErrRenderNotConfigured = errors.New("Render API is not configured")
	ErrGitHubNotConfigured = errors.New("GitHub API is not configured")
	ErrServiceIDMismatch   = errors.New("Render API returned a different service")
	ErrGitSourceMissing    = errors.New("Render service is not connected to a supported GitHub repository and branch")
	ErrServiceSuspended    = errors.New("Render service is suspended")
	ErrRenderTimeout       = errors.New("Render API request timed out")
	ErrGitHubTimeout       = errors.New("GitHub API request timed out")
	ErrDeploymentUnknown   = errors.New("deployment state could not be verified")
	ErrDeploymentFailed    = errors.New("deployment failed")
	ErrDeploymentTimeout   = errors.New("deployment did not finish before the deadline")
)

type RenderAPIError struct {
	StatusCode int
}

func (e *RenderAPIError) Error() string {
	return fmt.Sprintf("Render API returned HTTP %d", e.StatusCode)
}

type GitHubAPIError struct {
	StatusCode int
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub API returned HTTP %d", e.StatusCode)
}

type RenderService struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Type              string `json:"type"`
	Repo              string `json:"repo"`
	Branch            string `json:"branch"`
	AutoDeploy        string `json:"autoDeploy"`
	AutoDeployTrigger string `json:"autoDeployTrigger"`
	Suspended         string `json:"suspended"`
	DashboardURL      string `json:"dashboardUrl"`
}

type RenderDeploy struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Trigger    string `json:"trigger"`
	CreatedAt  string `json:"createdAt"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	Commit     struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		CreatedAt string `json:"createdAt"`
	} `json:"commit"`
}

type renderDeployListEntry struct {
	Deploy RenderDeploy `json:"deploy"`
	Cursor string       `json:"cursor"`
}

type DeploymentState string

const (
	DeploymentUpToDate      DeploymentState = "up_to_date"
	DeploymentUpdatePending DeploymentState = "update_pending"
	DeploymentInProgress    DeploymentState = "in_progress"
	DeploymentFailed        DeploymentState = "failed"
	DeploymentUnknown       DeploymentState = "unknown"
)

type DeploymentProgress string

const (
	ProgressChecking   DeploymentProgress = "checking"
	ProgressWaiting    DeploymentProgress = "waiting"
	ProgressTriggering DeploymentProgress = "triggering"
	ProgressVerifying  DeploymentProgress = "verifying"
)

type RenderSnapshot struct {
	Service         RenderService
	Deploys         []RenderDeploy
	LatestDeploy    *RenderDeploy
	ActiveDeploy    *RenderDeploy
	LatestCommitSHA string
	LiveCommitSHA   string
	DeploymentState DeploymentState
	APILatency      time.Duration
	CheckedAt       time.Time
}

type DeploymentResult struct {
	State           DeploymentState
	LatestCommitSHA string
	LiveCommitSHA   string
	TriggerCount    int
}

type renderAPIResponseError struct {
	statusCode int
	retryAfter time.Duration
}

func (e *renderAPIResponseError) Error() string {
	return fmt.Sprintf("API returned HTTP %d", e.statusCode)
}

type RenderClient struct {
	apiKey       string
	serviceID    string
	githubToken  string
	baseURL      string
	githubURL    string
	http         *http.Client
	pollInterval time.Duration
	triggerMu    sync.Mutex
}

func NewRenderClient(apiKey, serviceID, githubToken string) *RenderClient {
	return &RenderClient{
		apiKey:       strings.TrimSpace(apiKey),
		serviceID:    strings.TrimSpace(serviceID),
		githubToken:  strings.TrimSpace(githubToken),
		baseURL:      renderAPIBaseURL,
		githubURL:    githubAPIBaseURL,
		http:         &http.Client{Timeout: 5 * time.Second},
		pollInterval: defaultDeployPollTime,
	}
}

func (c *RenderClient) Configured() bool {
	return c != nil && c.apiKey != "" && c.serviceID != ""
}

func (c *RenderClient) Snapshot(ctx context.Context) (RenderSnapshot, error) {
	if !c.Configured() {
		return RenderSnapshot{}, ErrRenderNotConfigured
	}
	if c.githubToken == "" {
		return RenderSnapshot{}, ErrGitHubNotConfigured
	}

	started := time.Now()
	snapshot, err := c.inspect(ctx)
	if err != nil {
		return RenderSnapshot{}, err
	}
	snapshot.APILatency = time.Since(started)
	snapshot.CheckedAt = time.Now()
	return snapshot, nil
}

// EnsureLatest waits for an existing deployment, then deploys and verifies the
// current branch head. A second trigger is allowed only if that branch advanced.
func (c *RenderClient) EnsureLatest(ctx context.Context, onProgress func(DeploymentProgress)) (DeploymentResult, error) {
	if !c.Configured() {
		return DeploymentResult{State: DeploymentUnknown}, ErrRenderNotConfigured
	}
	if c.githubToken == "" {
		return DeploymentResult{State: DeploymentUnknown}, ErrGitHubNotConfigured
	}

	c.triggerMu.Lock()
	defer c.triggerMu.Unlock()

	report := func(progress DeploymentProgress) {
		if onProgress != nil {
			onProgress(progress)
		}
	}
	report(ProgressChecking)

	triggerCount := 0
	for {
		snapshot, err := c.Snapshot(ctx)
		if err != nil {
			return deploymentResult(snapshot, triggerCount), err
		}
		if snapshot.Service.Suspended == "suspended" {
			return deploymentResult(snapshot, triggerCount), ErrServiceSuspended
		}

		switch snapshot.DeploymentState {
		case DeploymentUpToDate:
			return deploymentResult(snapshot, triggerCount), nil
		case DeploymentUnknown:
			if snapshot.ActiveDeploy != nil && snapshot.ActiveDeploy.ID != "" {
				report(ProgressWaiting)
				completed, waitErr := c.waitForDeploy(ctx, snapshot, snapshot.ActiveDeploy.ID, "", time.Time{})
				if waitErr != nil {
					return deploymentResult(completed, triggerCount), waitErr
				}
				continue
			}
			return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
		case DeploymentInProgress:
			if snapshot.ActiveDeploy == nil || snapshot.ActiveDeploy.ID == "" {
				return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
			}
			report(ProgressWaiting)
			completed, err := c.waitForDeploy(ctx, snapshot, snapshot.ActiveDeploy.ID, "", time.Time{})
			if err != nil {
				return deploymentResult(completed, triggerCount), err
			}
			continue
		case DeploymentFailed, DeploymentUpdatePending:
			if triggerCount >= maxDeployTriggers {
				return deploymentResult(snapshot, triggerCount), nil
			}
			if snapshot.LatestCommitSHA == "" {
				return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
			}

			report(ProgressTriggering)
			requestedAt := time.Now().UTC()
			deploy, statusCode, err := c.triggerDeploy(ctx, snapshot.LatestCommitSHA)
			if err != nil {
				var apiErr *RenderAPIError
				if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
					fresh, refreshErr := c.Snapshot(ctx)
					if refreshErr != nil {
						return deploymentResult(fresh, triggerCount), refreshErr
					}
					if fresh.DeploymentState == DeploymentUpToDate {
						continue
					}
					if fresh.DeploymentState != DeploymentInProgress || fresh.ActiveDeploy == nil || fresh.ActiveDeploy.ID == "" {
						return deploymentResult(fresh, triggerCount), ErrDeploymentUnknown
					}
					report(ProgressWaiting)
					completed, waitErr := c.waitForDeploy(ctx, fresh, fresh.ActiveDeploy.ID, "", time.Time{})
					if waitErr != nil {
						return deploymentResult(completed, triggerCount), waitErr
					}
					continue
				}
				return deploymentResult(snapshot, triggerCount), err
			}
			triggerCount++
			if statusCode == http.StatusCreated && deploy.ID == "" {
				return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
			}
			if statusCode != http.StatusCreated && statusCode != http.StatusAccepted {
				return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
			}
			report(ProgressWaiting)
			completed, err := c.waitForDeploy(ctx, snapshot, deploy.ID, snapshot.LatestCommitSHA, requestedAt)
			if err != nil {
				return deploymentResult(completed, triggerCount), err
			}
			report(ProgressVerifying)
		default:
			return deploymentResult(snapshot, triggerCount), ErrDeploymentUnknown
		}
	}
}

func (c *RenderClient) inspect(ctx context.Context) (RenderSnapshot, error) {
	var service RenderService
	if err := c.renderRequest(ctx, http.MethodGet, "/services/"+url.PathEscape(c.serviceID), nil, &service); err != nil {
		return RenderSnapshot{}, err
	}
	if service.ID != c.serviceID {
		return RenderSnapshot{}, ErrServiceIDMismatch
	}
	owner, repo, err := githubRepositoryParts(service.Repo)
	if err != nil || strings.TrimSpace(service.Branch) == "" {
		return RenderSnapshot{}, ErrGitSourceMissing
	}

	latestSHA, err := c.githubBranchHead(ctx, owner, repo, service.Branch)
	if err != nil {
		return RenderSnapshot{}, err
	}
	deploys, complete, err := c.listDeploys(ctx)
	if err != nil {
		return RenderSnapshot{}, err
	}

	snapshot := RenderSnapshot{
		Service:         service,
		Deploys:         deploys,
		LatestCommitSHA: latestSHA,
		DeploymentState: DeploymentUnknown,
	}
	latest, latestKnown := newestDeploy(deploys)
	if len(deploys) > 0 && latestKnown {
		snapshot.LatestDeploy = &latest
	}
	for _, deploy := range deploys {
		if isDeployActive(deploy.Status) {
			active := deploy
			snapshot.ActiveDeploy = &active
			break
		}
	}
	liveDeploy, hasLiveDeploy, liveKnown := newestDeployWithStatus(deploys, "live")
	if hasLiveDeploy && liveKnown {
		snapshot.LiveCommitSHA = liveDeploy.Commit.ID
	}

	if !complete || !latestKnown || !liveKnown {
		return snapshot, nil
	}
	if snapshot.LatestDeploy != nil && !isKnownDeployStatus(snapshot.LatestDeploy.Status) {
		return snapshot, nil
	}
	if snapshot.ActiveDeploy != nil {
		snapshot.DeploymentState = DeploymentInProgress
		return snapshot, nil
	}
	if !validCommitSHA(snapshot.LatestCommitSHA) {
		return snapshot, nil
	}
	if len(deploys) == 0 {
		snapshot.DeploymentState = DeploymentUpdatePending
		return snapshot, nil
	}
	if validCommitSHA(snapshot.LiveCommitSHA) && strings.EqualFold(snapshot.LatestCommitSHA, snapshot.LiveCommitSHA) {
		snapshot.DeploymentState = DeploymentUpToDate
		return snapshot, nil
	}
	if snapshot.LatestDeploy != nil && isDeployFailed(snapshot.LatestDeploy.Status) {
		snapshot.DeploymentState = DeploymentFailed
		return snapshot, nil
	}
	if !validCommitSHA(snapshot.LiveCommitSHA) {
		return snapshot, nil
	}
	snapshot.DeploymentState = DeploymentUpdatePending
	return snapshot, nil
}

func (c *RenderClient) listDeploys(ctx context.Context) ([]RenderDeploy, bool, error) {
	deploys := make([]RenderDeploy, 0, 100)
	cursor := ""
	for page := 0; page < maxDeployPages; page++ {
		query := url.Values{"limit": []string{"100"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var entries []renderDeployListEntry
		endpoint := "/services/" + url.PathEscape(c.serviceID) + "/deploys?" + query.Encode()
		if err := c.renderRequest(ctx, http.MethodGet, endpoint, nil, &entries); err != nil {
			return nil, false, err
		}
		for _, entry := range entries {
			deploys = append(deploys, entry.Deploy)
		}
		if len(entries) == 0 || entries[len(entries)-1].Cursor == "" {
			return deploys, true, nil
		}
		cursor = entries[len(entries)-1].Cursor
	}
	return deploys, false, nil
}

func (c *RenderClient) githubBranchHead(ctx context.Context, owner, repo, branch string) (string, error) {
	endpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/commits/" + url.PathEscape(branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.githubURL, "/")+endpoint, nil)
	if err != nil {
		return "", ErrGitHubNotConfigured
	}
	req.Header.Set("Authorization", "Bearer "+c.githubToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	var response struct {
		SHA string `json:"sha"`
	}
	if _, err := c.doJSON(ctx, c.http, req, &response, true); err != nil {
		return "", toGitHubError(ctx, err)
	}
	if !validCommitSHA(response.SHA) {
		return "", ErrDeploymentUnknown
	}
	return strings.ToLower(response.SHA), nil
}

func (c *RenderClient) triggerDeploy(ctx context.Context, commitSHA string) (RenderDeploy, int, error) {
	if !validCommitSHA(commitSHA) {
		return RenderDeploy{}, 0, ErrDeploymentUnknown
	}
	payload, err := json.Marshal(struct {
		CommitID string `json:"commitId"`
	}{CommitID: commitSHA})
	if err != nil {
		return RenderDeploy{}, 0, ErrDeploymentUnknown
	}
	var deploy RenderDeploy
	statusCode, err := c.renderRequestWithStatus(ctx, http.MethodPost,
		"/services/"+url.PathEscape(c.serviceID)+"/deploys",
		bytes.NewReader(payload), &deploy)
	if err != nil {
		return RenderDeploy{}, 0, err
	}
	return deploy, statusCode, nil
}

func (c *RenderClient) waitForDeploy(
	ctx context.Context,
	initial RenderSnapshot,
	deployID, commitSHA string,
	requestedAt time.Time,
) (RenderSnapshot, error) {
	missingPolls := 0
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return initial, ErrDeploymentTimeout
			}
			return initial, err
		}
		deploys, complete, err := c.listDeploys(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return initial, ErrDeploymentTimeout
			}
			return initial, err
		}
		if !complete {
			return initial, ErrDeploymentUnknown
		}
		snapshot := snapshotWithDeploys(initial, deploys)
		deploy, found := findTrackedDeploy(deploys, deployID, commitSHA, requestedAt)
		if found {
			missingPolls = 0
			switch classifyDeployStatus(deploy.Status) {
			case DeploymentInProgress:
			case DeploymentUpToDate:
				snapshot.LiveCommitSHA = deploy.Commit.ID
				return snapshot, nil
			case DeploymentFailed:
				snapshot.DeploymentState = DeploymentFailed
				return snapshot, ErrDeploymentFailed
			default:
				snapshot.DeploymentState = DeploymentUnknown
				return snapshot, ErrDeploymentUnknown
			}
		} else {
			missingPolls++
			if missingPolls >= maxMissingDeployPolls {
				return snapshot, ErrDeploymentUnknown
			}
		}

		timer := time.NewTimer(c.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return snapshot, ErrDeploymentTimeout
			}
			return snapshot, ctx.Err()
		case <-timer.C:
		}
		initial = snapshot
	}
}

func snapshotWithDeploys(snapshot RenderSnapshot, deploys []RenderDeploy) RenderSnapshot {
	snapshot.Deploys = deploys
	snapshot.LatestDeploy = nil
	snapshot.ActiveDeploy = nil
	latest, latestKnown := newestDeploy(deploys)
	if len(deploys) > 0 && latestKnown {
		snapshot.LatestDeploy = &latest
	}
	for _, deploy := range deploys {
		if isDeployActive(deploy.Status) {
			active := deploy
			snapshot.ActiveDeploy = &active
			break
		}
	}
	return snapshot
}

func newestDeploy(deploys []RenderDeploy) (RenderDeploy, bool) {
	if len(deploys) == 0 {
		return RenderDeploy{}, true
	}
	newest, found, known := newestDeployWithStatus(deploys, "")
	return newest, found && known
}

func newestDeployWithStatus(deploys []RenderDeploy, status string) (RenderDeploy, bool, bool) {
	var newest RenderDeploy
	var newestAt time.Time
	found := false
	for _, deploy := range deploys {
		if status != "" && deploy.Status != status {
			continue
		}
		createdAt, err := time.Parse(time.RFC3339Nano, deploy.CreatedAt)
		if err != nil {
			if found || len(deploys) > 1 {
				return RenderDeploy{}, found, false
			}
			return deploy, true, true
		}
		if !found || createdAt.After(newestAt) {
			newest = deploy
			newestAt = createdAt
			found = true
		}
	}
	return newest, found, true
}

func findTrackedDeploy(deploys []RenderDeploy, deployID, commitSHA string, requestedAt time.Time) (RenderDeploy, bool) {
	for _, deploy := range deploys {
		if deployID != "" && deploy.ID == deployID {
			return deploy, true
		}
		if deployID == "" && commitSHA != "" &&
			strings.EqualFold(deploy.Commit.ID, commitSHA) &&
			deployCreatedAfter(deploy.CreatedAt, requestedAt) {
			return deploy, true
		}
	}
	return RenderDeploy{}, false
}

func deployCreatedAfter(createdAt string, requestedAt time.Time) bool {
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil || requestedAt.IsZero() {
		return false
	}
	return !created.Before(requestedAt.Add(-5 * time.Second))
}

func deploymentResult(snapshot RenderSnapshot, triggerCount int) DeploymentResult {
	return DeploymentResult{
		State:           snapshot.DeploymentState,
		LatestCommitSHA: snapshot.LatestCommitSHA,
		LiveCommitSHA:   snapshot.LiveCommitSHA,
		TriggerCount:    triggerCount,
	}
}

func classifyDeployStatus(status string) DeploymentState {
	switch status {
	case "created", "queued", "build_in_progress", "update_in_progress", "pre_deploy_in_progress":
		return DeploymentInProgress
	case "live":
		return DeploymentUpToDate
	case "build_failed", "update_failed", "pre_deploy_failed", "canceled", "deactivated":
		return DeploymentFailed
	default:
		return DeploymentUnknown
	}
}

func isKnownDeployStatus(status string) bool {
	return classifyDeployStatus(status) != DeploymentUnknown
}

func isDeployActive(status string) bool {
	return classifyDeployStatus(status) == DeploymentInProgress
}

func isDeployFailed(status string) bool {
	return classifyDeployStatus(status) == DeploymentFailed
}

func validCommitSHA(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	_, err := hex.DecodeString(sha)
	return err == nil
}

func githubRepositoryParts(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	var repositoryPath string
	if strings.HasPrefix(raw, "git@github.com:") {
		repositoryPath = strings.TrimPrefix(raw, "git@github.com:")
	} else {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") ||
			parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", "", ErrGitSourceMissing
		}
		repositoryPath = parsed.Path
	}
	parts := strings.Split(strings.Trim(path.Clean("/"+repositoryPath), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrGitSourceMissing
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	if repo == "" {
		return "", "", ErrGitSourceMissing
	}
	return parts[0], repo, nil
}

func (c *RenderClient) renderRequest(ctx context.Context, method, endpoint string, body io.Reader, result any) error {
	_, err := c.renderRequestWithStatus(ctx, method, endpoint, body, result)
	return err
}

func (c *RenderClient) renderRequestWithStatus(ctx context.Context, method, endpoint string, body io.Reader, result any) (int, error) {
	if !c.Configured() {
		return 0, ErrRenderNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+endpoint, body)
	if err != nil {
		return 0, ErrDeploymentUnknown
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	statusCode, err := c.doJSON(ctx, c.http, req, result, method == http.MethodGet)
	if err != nil {
		return statusCode, toRenderError(ctx, err)
	}
	return statusCode, nil
}

func (c *RenderClient) doJSON(ctx context.Context, client *http.Client, req *http.Request, result any, retry bool) (int, error) {
	attempts := 1
	if retry {
		attempts = maxReadAttempts
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		statusCode, err := doJSONRequest(ctx, client, req, result)
		if err == nil {
			return statusCode, nil
		}
		lastErr = err
		if !retry || !isTransientAPIError(err) || attempt+1 == attempts {
			return 0, err
		}
		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		var apiErr *renderAPIResponseError
		if errors.As(err, &apiErr) && apiErr.retryAfter > delay {
			delay = apiErr.retryAfter
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
	return 0, lastErr
}

func doJSONRequest(ctx context.Context, client *http.Client, req *http.Request, result any) (int, error) {
	request := req.Clone(ctx)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return 0, err
		}
		request.Body = body
	}
	resp, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 0, context.DeadlineExceeded
		}
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := &renderAPIResponseError{statusCode: resp.StatusCode}
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, parseErr := time.ParseDuration(retryAfter + "s"); parseErr == nil {
				apiErr.retryAfter = seconds
			}
		}
		return resp.StatusCode, apiErr
	}
	if result == nil {
		_, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxRenderBody))
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusAccepted && resp.ContentLength == 0 {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRenderBody)).Decode(result); err != nil {
		if resp.StatusCode == http.StatusAccepted && errors.Is(err, io.EOF) {
			return resp.StatusCode, nil
		}
		return resp.StatusCode, errors.New("API response could not be decoded")
	}
	return resp.StatusCode, nil
}

func isTransientAPIError(err error) bool {
	var apiErr *renderAPIResponseError
	if errors.As(err, &apiErr) {
		return apiErr.statusCode == http.StatusTooManyRequests || apiErr.statusCode >= http.StatusInternalServerError
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func toGitHubError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ErrGitHubTimeout
	}
	var apiErr *renderAPIResponseError
	if errors.As(err, &apiErr) {
		return &GitHubAPIError{StatusCode: apiErr.statusCode}
	}
	return errors.New("GitHub API request failed")
}

func toRenderError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return ErrRenderTimeout
	}
	var apiErr *renderAPIResponseError
	if errors.As(err, &apiErr) {
		return &RenderAPIError{StatusCode: apiErr.statusCode}
	}
	return errors.New("Render API request failed")
}
