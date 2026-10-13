package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/discordcasino/v2/internal/monitoring"
)

func TestHealthAndReadinessEndpoints(t *testing.T) {
	monitor := monitoring.NewMonitor()
	handler := newHealthHandler(monitor)

	liveness := httptest.NewRecorder()
	handler.ServeHTTP(liveness, httptest.NewRequest(http.MethodGet, "/health", nil))
	if liveness.Code != http.StatusOK || liveness.Body.String() != `{"status":"ok"}` {
		t.Fatalf("unexpected liveness response: %d %q", liveness.Code, liveness.Body.String())
	}

	readiness := httptest.NewRecorder()
	handler.ServeHTTP(readiness, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if readiness.Code != http.StatusServiceUnavailable || readiness.Body.String() != `{"status":"not_ready"}` {
		t.Fatalf("unexpected not-ready response: %d %q", readiness.Code, readiness.Body.String())
	}

	monitor.SetProcessStatus("running")
	monitor.SetGatewayStatus("connected", 25*time.Millisecond)
	monitor.SetDatabaseStatus("connected", 7*time.Millisecond)
	readiness = httptest.NewRecorder()
	handler.ServeHTTP(readiness, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if readiness.Code != http.StatusOK || readiness.Body.String() != `{"status":"ready"}` {
		t.Fatalf("unexpected ready response: %d %q", readiness.Code, readiness.Body.String())
	}
}
