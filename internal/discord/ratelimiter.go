package discord

import (
	"sync"
	"time"
)

// RateLimiter giới hạn tần suất lệnh theo từng user.
// Lưu trong RAM → reset khi restart (chấp nhận được cho chống spam).
// Mặc định: 1 lệnh / giây / user (cấu hình trong bot.go).
type RateLimiter struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time // userID → thời điểm lệnh cuối
	interval time.Duration        // khoảng cách tối thiểu giữa 2 lệnh
}

func NewRateLimiter(interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		lastSeen: make(map[string]time.Time),
		interval: interval,
	}
	// Goroutine dọn dẹp định kỳ để tránh rò rỉ bộ nhớ khi có nhiều user
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.gc()
		}
	}()
	return rl
}

// Allow kiểm tra xem user có được phép gửi lệnh không.
// Trả về true nếu đủ thời gian kể từ lệnh trước.
// Tự ghi nhận thời gian lệnh này để lần sau so sánh.
func (r *RateLimiter) Allow(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if last, ok := r.lastSeen[userID]; ok && time.Since(last) < r.interval {
		return false
	}
	r.lastSeen[userID] = time.Now()
	return true
}

// Remaining trả về thời gian user còn phải chờ (0 nếu được phép ngay).
func (r *RateLimiter) Remaining(userID string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if last, ok := r.lastSeen[userID]; ok {
		wait := r.interval - time.Since(last)
		if wait > 0 {
			return wait
		}
	}
	return 0
}

// gc xóa các entry cũ hơn 10 phút để giải phóng bộ nhớ.
func (r *RateLimiter) gc() {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Minute)
	for k, v := range r.lastSeen {
		if v.Before(cutoff) {
			delete(r.lastSeen, k)
		}
	}
}
