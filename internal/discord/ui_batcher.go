package discord

import (
	"sync"
	"time"
)

// Ghi chú: In-memory batcher này chỉ an toàn và hoạt động chính xác
// khi bot chạy trên Single Process (1 instance Render/VPS).
// Nếu nâng cấp lên Multi-instance, cần chuyển sang kiến trúc Redis/PubSub.

const (
	debounceDelay = 1500 * time.Millisecond
	maxWait       = 3000 * time.Millisecond
)

// GameUIEvent đại diện cho một sự kiện cần hiển thị lên UI
type GameUIEvent struct {
	UserID  string
	Message string
}

// TableBatchState lưu trữ trạng thái buffer của một bàn chơi
type TableBatchState struct {
	mu         sync.Mutex
	events     []GameUIEvent
	timer      *time.Timer
	firstEvent time.Time
	isClosed   bool
	flushFn    func([]GameUIEvent) // Lưu lại hàm render UI mới nhất
}

// GameUIBatcher quản lý toàn bộ các bàn chơi đang active
type GameUIBatcher struct {
	mu     sync.Mutex
	tables map[string]*TableBatchState
}

func NewGameUIBatcher() *GameUIBatcher {
	return &GameUIBatcher{
		tables: make(map[string]*TableBatchState),
	}
}

func (b *GameUIBatcher) getTableState(tableID string) *TableBatchState {
	b.mu.Lock()
	defer b.mu.Unlock()
	state, exists := b.tables[tableID]
	if !exists {
		state = &TableBatchState{}
		b.tables[tableID] = state
	}
	return state
}

// QueueEvent thêm sự kiện vào buffer và lên lịch Flush UI
func (b *GameUIBatcher) QueueEvent(tableID string, event GameUIEvent, flushFn func([]GameUIEvent)) {
	state := b.getTableState(tableID)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.isClosed {
		return
	}

	state.events = append(state.events, event)
	state.flushFn = flushFn // Cập nhật hàm Render

	if state.firstEvent.IsZero() {
		state.firstEvent = time.Now()
	}

	if state.timer != nil {
		state.timer.Stop()
	}

	// Nếu đã chờ quá giới hạn maxWait, ép buộc flush ngay lập tức
	if time.Since(state.firstEvent) >= maxWait {
		b.triggerFlush(state, flushFn)
		return
	}

	// Reset timer theo debounceDelay
	state.timer = time.AfterFunc(debounceDelay, func() {
		state.mu.Lock()
		b.triggerFlush(state, flushFn)
		state.mu.Unlock()
	})
}

func (b *GameUIBatcher) triggerFlush(state *TableBatchState, flushFn func([]GameUIEvent)) {
	if len(state.events) == 0 || state.isClosed {
		return
	}
	eventsCopy := make([]GameUIEvent, len(state.events))
	copy(eventsCopy, state.events)
	state.events = nil
	state.firstEvent = time.Time{}

	// Thực thi flush async để không block luồng nhận event
	go flushFn(eventsCopy)
}

// ForceFlushAndClose ép gửi toàn bộ tin nhắn Rút/Dằn còn kẹt lại trước khi hủy bàn
func (b *GameUIBatcher) ForceFlushAndClose(tableID string) {
	state := b.getTableState(tableID)
	state.mu.Lock()

	if state.isClosed {
		state.mu.Unlock()
		return
	}
	state.isClosed = true
	if state.timer != nil {
		state.timer.Stop()
	}

	eventsCopy := make([]GameUIEvent, len(state.events))
	copy(eventsCopy, state.events)
	flush := state.flushFn
	state.events = nil
	state.mu.Unlock()

	// Nếu còn thao tác kẹt lại (như lượt Dằn cuối cùng), ép gửi ngay lập tức
	if len(eventsCopy) > 0 && flush != nil {
		flush(eventsCopy)
	}

	b.mu.Lock()
	delete(b.tables, tableID)
	b.mu.Unlock()
}
