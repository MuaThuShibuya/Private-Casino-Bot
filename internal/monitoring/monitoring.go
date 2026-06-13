package monitoring

import (
	"log"
	"sync"
	"time"
)

// Monitor cung cấp các biểu đồ, log và dữ liệu tiêu chuẩn.
type Monitor struct {
	mu      sync.RWMutex
	metrics map[string]float64
}

// NewMonitor khởi tạo monitor đơn giản.
func NewMonitor() *Monitor {
	return &Monitor{metrics: make(map[string]float64)}
}

// RecordGauge lưu giá trị metrics dạng gauge.
func (m *Monitor) RecordGauge(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[name] = value
}

// IncrementCounter tăng counter.
func (m *Monitor) IncrementCounter(name string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[name] += delta
}

// Snapshot trả về một bản sao metric hiện tại.
func (m *Monitor) Snapshot() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]float64, len(m.metrics))
	for k, v := range m.metrics {
		copy[k] = v
	}
	return copy
}

// StartHeartBeat sẽ log định kỳ thông tin hệ thống.
func (m *Monitor) StartHeartBeat(interval time.Duration) {
	go func() {
		for range time.Tick(interval) {
			log.Printf("[monitoring] heartbeat metrics=%v", m.Snapshot())
		}
	}()
}
