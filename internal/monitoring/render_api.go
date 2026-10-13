package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	renderAPIBaseURL = "https://api.render.com/v1"
	maxRenderBody    = 1 << 20
)

var (
	ErrRenderNotConfigured = errors.New("Render API is not configured")
	ErrDeployInProgress    = errors.New("a Render deployment is already in progress")
	ErrGitSourceMissing    = errors.New("Render service is not connected to a Git repository and branch")
	ErrServiceSuspended    = errors.New("Render service is suspended")
	ErrRenderTimeout       = errors.New("Render API request timed out")
)

type RenderAPIError struct {
	StatusCode int
}

func (e *RenderAPIError) Error() string {
	return fmt.Sprintf("Render API returned HTTP %d", e.StatusCode)
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
}

type RenderSnapshot struct {
	Service    RenderService
	Deploys    []RenderDeploy
	APILatency time.Duration
	CheckedAt  time.Time
}

type RenderClient struct {
	apiKey    string
	serviceID string
	baseURL   string
	http      *http.Client
	triggerMu sync.Mutex
}

func NewRenderClient(apiKey, serviceID string) *RenderClient {
	return &RenderClient{
		apiKey:    strings.TrimSpace(apiKey),
		serviceID: strings.TrimSpace(serviceID),
		baseURL:   renderAPIBaseURL,
		http:      &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *RenderClient) Configured() bool {
	return c != nil && c.apiKey != "" && c.serviceID != ""
}

func (c *RenderClient) Snapshot(ctx context.Context) (RenderSnapshot, error) {
	if !c.Configured() {
		return RenderSnapshot{}, ErrRenderNotConfigured
	}
	started := time.Now()
	var service RenderService
	if err := c.get(ctx, "/services/"+url.PathEscape(c.serviceID), &service); err != nil {
		return RenderSnapshot{}, err
	}
	var entries []renderDeployListEntry
	if err := c.get(ctx, "/services/"+url.PathEscape(c.serviceID)+"/deploys?limit=5", &entries); err != nil {
		return RenderSnapshot{}, err
	}
	deploys := make([]RenderDeploy, 0, len(entries))
	for _, entry := range entries {
		deploys = append(deploys, entry.Deploy)
	}
	return RenderSnapshot{
		Service:    service,
		Deploys:    deploys,
		APILatency: time.Since(started),
		CheckedAt:  time.Now(),
	}, nil
}

func (c *RenderClient) TriggerDeploy(ctx context.Context) (RenderDeploy, error) {
	if !c.Configured() {
		return RenderDeploy{}, ErrRenderNotConfigured
	}

	c.triggerMu.Lock()
	defer c.triggerMu.Unlock()

	snapshot, err := c.Snapshot(ctx)
	if err != nil {
		return RenderDeploy{}, err
	}
	if snapshot.Service.Repo == "" || snapshot.Service.Branch == "" {
		return RenderDeploy{}, ErrGitSourceMissing
	}
	if snapshot.Service.Suspended == "suspended" {
		return RenderDeploy{}, ErrServiceSuspended
	}
	for _, deploy := range snapshot.Deploys {
		if isDeployActive(deploy.Status) {
			return RenderDeploy{}, ErrDeployInProgress
		}
	}

	var deploy RenderDeploy
	if err := c.request(ctx, http.MethodPost, "/services/"+url.PathEscape(c.serviceID)+"/deploys", bytes.NewReader([]byte("{}")), &deploy); err != nil {
		return RenderDeploy{}, err
	}
	return deploy, nil
}

func isDeployActive(status string) bool {
	switch status {
	case "created", "queued", "build_in_progress", "update_in_progress", "pre_deploy_in_progress":
		return true
	default:
		return false
	}
}

func (c *RenderClient) get(ctx context.Context, path string, result any) error {
	return c.request(ctx, http.MethodGet, path, nil, result)
}

func (c *RenderClient) request(ctx context.Context, method, path string, body io.Reader, result any) error {
	if !c.Configured() {
		return ErrRenderNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, body)
	if err != nil {
		return fmt.Errorf("create Render API request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrRenderTimeout
		}
		return errors.New("Render API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &RenderAPIError{StatusCode: resp.StatusCode}
	}
	if result == nil {
		_, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxRenderBody))
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxRenderBody))
	if err := decoder.Decode(result); err != nil {
		return fmt.Errorf("decode Render API response: %w", err)
	}
	return nil
}
