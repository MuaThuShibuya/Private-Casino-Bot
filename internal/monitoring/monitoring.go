package monitoring

import (
	"log"
	"math"
	"runtime/metrics"
	"sort"
	"sync"
	"time"
)

const (
	maxNamedMetrics   = 128
	maxCommandTimings = 512
	maxRecentEvents   = 20
	errorBucketCount  = 900
	staleAfter        = 90 * time.Second
)

type Event struct {
	Time      time.Time
	Component string
	Message   string
}

type LatencySummary struct {
	Count int
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
}

type HealthSnapshot struct {
	StartedAt           time.Time
	LastUpdated         time.Time
	ProcessStatus       string
	GatewayStatus       string
	GatewayStale        bool
	GatewayLatency      time.Duration
	GatewayLatencySeen  bool
	DatabaseStatus      string
	DatabaseStale       bool
	DatabaseLatency     time.Duration
	DatabaseLatencySeen bool
	Reconnects          uint64
	CommandCount        uint64
	CommandLatency      LatencySummary
	ProcessCPUPercent   float64
	ProcessCPUSeen      bool
	Errors15m           int
	RecentErrors        []Event
}

type errorBucket struct {
	second int64
	count  int
}

// Monitor stores bounded, process-local runtime health and performance data.
type Monitor struct {
	mu sync.RWMutex

	startedAt           time.Time
	lastUpdated         time.Time
	processStatus       string
	gatewayStatus       string
	gatewayUpdated      time.Time
	gatewayLatency      time.Duration
	gatewayLatencySeen  bool
	databaseStatus      string
	databaseUpdated     time.Time
	databaseLatency     time.Duration
	databaseLatencySeen bool
	reconnects          uint64
	commandCount        uint64
	commandTimings      []time.Duration
	events              []Event
	errorBuckets        [errorBucketCount]errorBucket
	cpuLastAt           time.Time
	cpuLastSeconds      float64
	processCPUPercent   float64
	processCPUSeen      bool
	metrics             map[string]float64
}

func NewMonitor() *Monitor {
	now := time.Now()
	return &Monitor{
		startedAt:      now,
		lastUpdated:    now,
		processStatus:  "starting",
		gatewayStatus:  "starting",
		databaseStatus: "starting",
		metrics:        make(map[string]float64),
	}
}

var defaultMonitor = NewMonitor()

func Default() *Monitor {
	return defaultMonitor
}

func (m *Monitor) SetProcessStatus(status string) {
	m.mu.Lock()
	m.processStatus = status
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

func (m *Monitor) SetGatewayStatus(status string, latency time.Duration) {
	m.mu.Lock()
	m.gatewayStatus = status
	m.gatewayUpdated = time.Now()
	if latency > 0 {
		m.gatewayLatency = latency
		m.gatewayLatencySeen = true
	}
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

func (m *Monitor) RecordReconnect() {
	m.mu.Lock()
	m.reconnects++
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

func (m *Monitor) SetDatabaseStatus(status string, latency time.Duration) {
	m.mu.Lock()
	m.databaseStatus = status
	m.databaseUpdated = time.Now()
	if latency > 0 {
		m.databaseLatency = latency
		m.databaseLatencySeen = true
	}
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

func (m *Monitor) RecordCommand(duration time.Duration) {
	if duration < 0 {
		return
	}
	m.mu.Lock()
	m.commandCount++
	if len(m.commandTimings) == maxCommandTimings {
		copy(m.commandTimings, m.commandTimings[1:])
		m.commandTimings[len(m.commandTimings)-1] = duration
	} else {
		m.commandTimings = append(m.commandTimings, duration)
	}
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

// RecordError only accepts short, pre-sanitized diagnostic text from callers.
func (m *Monitor) RecordError(component, message string) {
	if len(component) > 48 {
		component = component[:48]
	}
	if len(message) > 160 {
		message = message[:160]
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	event := Event{Time: now, Component: component, Message: message}
	second := now.Unix()
	bucketIndex := second % int64(len(m.errorBuckets))
	bucket := &m.errorBuckets[bucketIndex]
	if bucket.second != second {
		*bucket = errorBucket{second: second}
	}
	bucket.count++
	if len(m.events) == maxRecentEvents {
		copy(m.events, m.events[1:])
		m.events[len(m.events)-1] = event
	} else {
		m.events = append(m.events, event)
	}
	m.lastUpdated = event.Time
}

func (m *Monitor) SampleProcessCPU() bool {
	samples := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 {
		return false
	}

	now := time.Now()
	cpuSeconds := samples[0].Value.Float64()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cpuLastAt.IsZero() {
		elapsed := now.Sub(m.cpuLastAt).Seconds()
		if elapsed > 0 && cpuSeconds >= m.cpuLastSeconds {
			m.processCPUPercent = (cpuSeconds - m.cpuLastSeconds) / elapsed * 100
			m.processCPUSeen = true
		}
	}
	m.cpuLastAt = now
	m.cpuLastSeconds = cpuSeconds
	m.lastUpdated = now
	return m.processCPUSeen
}

// RecordGauge stores a named gauge, returning false if the bounded metric set is full.
func (m *Monitor) RecordGauge(name string, value float64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.metrics[name]; !ok && len(m.metrics) >= maxNamedMetrics {
		return false
	}
	m.metrics[name] = value
	m.lastUpdated = time.Now()
	return true
}

// IncrementCounter stores a named counter, returning false if the bounded metric set is full.
func (m *Monitor) IncrementCounter(name string, delta float64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.metrics[name]; !ok && len(m.metrics) >= maxNamedMetrics {
		return false
	}
	m.metrics[name] += delta
	m.lastUpdated = time.Now()
	return true
}

func (m *Monitor) Snapshot() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]float64, len(m.metrics))
	for name, value := range m.metrics {
		copy[name] = value
	}
	return copy
}

func (m *Monitor) StartHeartBeat(interval time.Duration) {
	if interval <= 0 {
		log.Printf("[monitoring] heartbeat interval must be positive")
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			log.Printf("[monitoring] heartbeat metrics=%v", m.Snapshot())
		}
	}()
}

func (m *Monitor) Health() HealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	events := append([]Event(nil), m.events...)
	errorCount := 0
	nowSecond := now.Unix()
	for _, bucket := range m.errorBuckets {
		if bucket.count > 0 && nowSecond-bucket.second >= 0 && nowSecond-bucket.second < 15*60 {
			errorCount += bucket.count
		}
	}
	return HealthSnapshot{
		StartedAt:           m.startedAt,
		LastUpdated:         m.lastUpdated,
		ProcessStatus:       m.processStatus,
		GatewayStatus:       m.gatewayStatus,
		GatewayStale:        m.gatewayUpdated.IsZero() || now.Sub(m.gatewayUpdated) > staleAfter,
		GatewayLatency:      m.gatewayLatency,
		GatewayLatencySeen:  m.gatewayLatencySeen,
		DatabaseStatus:      m.databaseStatus,
		DatabaseStale:       m.databaseUpdated.IsZero() || now.Sub(m.databaseUpdated) > staleAfter,
		DatabaseLatency:     m.databaseLatency,
		DatabaseLatencySeen: m.databaseLatencySeen,
		Reconnects:          m.reconnects,
		CommandCount:        m.commandCount,
		CommandLatency:      summarize(m.commandTimings),
		ProcessCPUPercent:   m.processCPUPercent,
		ProcessCPUSeen:      m.processCPUSeen,
		Errors15m:           errorCount,
		RecentErrors:        events,
	}
}

func (m *Monitor) Ready() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	return m.processStatus == "running" &&
		m.gatewayStatus == "connected" &&
		m.databaseStatus == "connected" &&
		!m.gatewayUpdated.IsZero() && now.Sub(m.gatewayUpdated) <= staleAfter &&
		!m.databaseUpdated.IsZero() && now.Sub(m.databaseUpdated) <= staleAfter
}

func summarize(values []time.Duration) LatencySummary {
	summary := LatencySummary{Count: len(values)}
	if len(values) == 0 {
		return summary
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	summary.P50 = percentile(sorted, 0.50)
	summary.P95 = percentile(sorted, 0.95)
	summary.P99 = percentile(sorted, 0.99)
	return summary
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
