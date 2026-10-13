package monitoring

import (
	"fmt"
	"testing"
	"time"
)

func TestHealthDoesNotInventLatencyAndReadinessRequiresDependencies(t *testing.T) {
	monitor := NewMonitor()
	monitor.SetProcessStatus("running")

	if monitor.Ready() {
		t.Fatal("monitor must not be ready before dependencies are connected")
	}
	health := monitor.Health()
	if health.GatewayLatencySeen || health.DatabaseLatencySeen {
		t.Fatal("latency must remain unobserved until a measurement is recorded")
	}

	monitor.SetGatewayStatus("connected", 45*time.Millisecond)
	monitor.SetDatabaseStatus("connected", 8*time.Millisecond)
	if !monitor.Ready() {
		t.Fatal("monitor should be ready after all required dependencies connect")
	}
}

func TestCommandLatencyPercentilesAndBoundedSamples(t *testing.T) {
	monitor := NewMonitor()
	for i := 1; i <= maxCommandTimings+1; i++ {
		monitor.RecordCommand(time.Duration(i) * time.Millisecond)
	}

	health := monitor.Health()
	if health.CommandCount != maxCommandTimings+1 {
		t.Fatalf("expected %d commands, got %d", maxCommandTimings+1, health.CommandCount)
	}
	if health.CommandLatency.Count != maxCommandTimings {
		t.Fatalf("expected bounded sample count %d, got %d", maxCommandTimings, health.CommandLatency.Count)
	}
	if health.CommandLatency.P50 != 257*time.Millisecond ||
		health.CommandLatency.P95 != 488*time.Millisecond ||
		health.CommandLatency.P99 != 508*time.Millisecond {
		t.Fatalf("unexpected latency percentiles: %+v", health.CommandLatency)
	}
}

func TestRecentErrorsAreBounded(t *testing.T) {
	monitor := NewMonitor()
	for i := 0; i < maxRecentEvents+5; i++ {
		monitor.RecordError("test", fmt.Sprintf("event-%d", i))
	}

	health := monitor.Health()
	if len(health.RecentErrors) != maxRecentEvents {
		t.Fatalf("expected %d errors, got %d", maxRecentEvents, len(health.RecentErrors))
	}
	if got := health.RecentErrors[0].Message; got != "event-5" {
		t.Fatalf("oldest retained event should be event-5, got %q", got)
	}
}

func TestErrorCountUsesWindowBucketsIndependentlyOfRecentLogLimit(t *testing.T) {
	monitor := NewMonitor()
	oldSecond := time.Now().Add(-16 * time.Minute).Unix()
	for i := range monitor.errorBuckets {
		monitor.errorBuckets[i] = errorBucket{second: oldSecond, count: 1}
	}
	monitor.RecordError("current", "new event")

	if got := monitor.Health().Errors15m; got != 1 {
		t.Fatalf("expected only current-window error to be counted, got %d", got)
	}
}

func TestReadinessRejectsStaleDependencyState(t *testing.T) {
	monitor := NewMonitor()
	monitor.SetProcessStatus("running")
	monitor.SetGatewayStatus("connected", time.Millisecond)
	monitor.SetDatabaseStatus("connected", time.Millisecond)
	monitor.mu.Lock()
	monitor.gatewayUpdated = time.Now().Add(-2 * staleAfter)
	monitor.mu.Unlock()

	if monitor.Ready() {
		t.Fatal("readiness should fail when gateway status is stale")
	}
}

func TestNamedMetricsHaveBoundedCardinality(t *testing.T) {
	monitor := NewMonitor()
	for i := 0; i < maxNamedMetrics; i++ {
		if !monitor.RecordGauge(fmt.Sprintf("metric-%d", i), float64(i)) {
			t.Fatalf("failed to record metric %d before limit", i)
		}
	}
	if monitor.RecordGauge("overflow", 1) {
		t.Fatal("metric cardinality limit was not enforced")
	}
}
