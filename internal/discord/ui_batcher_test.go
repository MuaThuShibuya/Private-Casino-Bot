package discord

import (
	"sync"
	"testing"
	"time"
)

func TestXidachUIBatcher_Debounce(t *testing.T) {
	batcher := NewGameUIBatcher()
	tableID := "test_table_1"

	var mu sync.Mutex
	flushCount := 0
	totalEventsFlushed := 0

	mockFlush := func(events []GameUIEvent) {
		mu.Lock()
		flushCount++
		totalEventsFlushed += len(events)
		mu.Unlock()
	}

	// Gửi 5 sự kiện cách nhau rất ngắn (0.2s) -> Cần gom lại thành 1 flush duy nhất
	for i := 0; i < 5; i++ {
		batcher.QueueEvent(tableID, GameUIEvent{UserID: "user1", Message: "rút bài"}, mockFlush)
		time.Sleep(200 * time.Millisecond)
	}

	// Đợi quá debounceDelay (1.5s) để timer được kích hoạt
	time.Sleep(1600 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if flushCount != 1 {
		t.Errorf("Kỳ vọng Flush 1 lần, thực tế: %d", flushCount)
	}
	if totalEventsFlushed != 5 {
		t.Errorf("Kỳ vọng Gom 5 events, thực tế: %d", totalEventsFlushed)
	}
}

func TestXidachUIBatcher_MaxWait(t *testing.T) {
	batcher := NewGameUIBatcher()
	tableID := "test_table_2"

	var mu sync.Mutex
	flushCount := 0

	mockFlush := func(events []GameUIEvent) {
		mu.Lock()
		flushCount++
		mu.Unlock()
	}

	// Gửi liên tục mỗi giây 1 sự kiện (làm timer bị reset liên tục).
	// MaxWait là 3s nên đến sự kiện thứ 4 (tổng 3s) bắt buộc phải Flush 1 lần dù timer vừa bị reset.
	for i := 0; i < 4; i++ {
		batcher.QueueEvent(tableID, GameUIEvent{UserID: "user1", Message: "spam"}, mockFlush)
		time.Sleep(1000 * time.Millisecond)
	}

	mu.Lock()
	if flushCount == 0 {
		t.Errorf("Lỗi: Đã quá maxWait (3s) nhưng không chịu Flush!")
	}
	mu.Unlock()
}

func TestXidachUIBatcher_ForceFlushAndClose(t *testing.T) {
	batcher := NewGameUIBatcher()
	tableID := "test_table_3"

	var mu sync.Mutex
	flushCount := 0

	batcher.QueueEvent(tableID, GameUIEvent{UserID: "u1", Message: "test"}, func(ev []GameUIEvent) {
		mu.Lock()
		flushCount++
		mu.Unlock()
	})

	batcher.ForceFlushAndClose(tableID) // Sẽ ép Flush ngay lập tức các sự kiện đang chờ

	mu.Lock()
	defer mu.Unlock()
	if flushCount != 1 {
		t.Errorf("Kỳ vọng Flush được gọi 1 lần khi ForceFlushAndClose, thực tế: %d", flushCount)
	}
}
