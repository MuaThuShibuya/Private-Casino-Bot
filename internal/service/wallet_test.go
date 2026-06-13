package service_test

// wallet_test.go — Test tính an toàn tài chính và race condition
//
// Chạy test thường:  go test ./internal/service/ -v
// Chạy race detector: go test ./internal/service/ -race -v
// Chạy concurrent:   go test ./internal/service/ -race -run TestConcurrent -count=5

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// ─── In-memory wallet giả lập hành vi MongoDB atomic update ──────────────────
//
// MongoDB: filter { balance: { $gte: amount } } + update { $inc: { balance: -amount } }
// Hai bước này được MongoDB thực thi atomic (document-level lock).
// Ở đây ta giả lập bằng sync.Mutex để kiểm tra logic tương đương.

var errInsufficientBalance = errors.New("số dư không đủ")

type atomicWallet struct {
	mu      sync.Mutex
	balance map[string]int64
}

func newAtomicWallet() *atomicWallet {
	return &atomicWallet{balance: make(map[string]int64)}
}

func (w *atomicWallet) set(userID string, amount int64) {
	w.mu.Lock()
	w.balance[userID] = amount
	w.mu.Unlock()
}

func (w *atomicWallet) get(userID string) int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.balance[userID]
}

// deduct giả lập MongoDB atomic: kiểm tra và trừ trong cùng một lock.
// Nếu không đủ tiền → trả lỗi, số dư không thay đổi.
func (w *atomicWallet) deduct(_ context.Context, userID string, amount int64, debtLimit int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	available := w.balance[userID] + debtLimit
	if available < amount {
		return errInsufficientBalance
	}
	w.balance[userID] -= amount
	return nil
}

// credit cộng tiền thắng cược.
func (w *atomicWallet) credit(_ context.Context, userID string, amount int64) {
	w.mu.Lock()
	w.balance[userID] += amount
	w.mu.Unlock()
}

// transfer giả lập MongoDB session transaction: trừ nguồn + cộng đích atomic.
func (w *atomicWallet) transfer(_ context.Context, fromID, toID string, amount int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.balance[fromID] < amount {
		return errInsufficientBalance
	}
	// Cả hai thao tác trong cùng một lock — không thể bị interleave
	w.balance[fromID] -= amount
	w.balance[toID] += amount
	return nil
}

// ─── Test case cơ bản ────────────────────────────────────────────────────────

func TestBetDeductBasic(t *testing.T) {
	tests := []struct {
		name        string
		initial     int64
		amount      int64
		wantBalance int64
		wantErr     bool
	}{
		{"Trừ bình thường", 1000, 500, 500, false},
		{"Trừ hết số dư", 1000, 1000, 0, false},
		{"Không đủ tiền — balance không đổi", 500, 600, 500, true},
		{"Số dư bằng 0", 0, 1, 0, true},
		{"Trừ 1 xu cuối cùng", 1, 1, 0, false},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newAtomicWallet()
			w.set("user", tt.initial)

			err := w.deduct(ctx, "user", tt.amount, 0)

			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v nhưng err=%v", tt.wantErr, err)
			}
			if got := w.get("user"); got != tt.wantBalance {
				t.Errorf("balance = %d, muốn %d", got, tt.wantBalance)
			}
		})
	}
}

// ─── Test Bet Deduct Kèm Debt Limit ──────────────────────────────────────────

func TestBetDeductWithDebt(t *testing.T) {
	tests := []struct {
		name        string
		initial     int64
		debtLimit   int64
		amount      int64
		wantBalance int64
		wantErr     bool
	}{
		{"Balance 0, Debt 10M, Bet 500k -> OK", 0, 10000000, 500000, -500000, false},
		{"Balance -9.8M, Debt 10M, Bet 500k -> FAIL", -9800000, 10000000, 500000, -9800000, true},
		{"Balance 2M, Debt 5M, Bet 6M -> OK", 2000000, 5000000, 6000000, -4000000, false},
		{"Balance -5M, Debt 5M, Bet 1 -> FAIL", -5000000, 5000000, 1, -5000000, true},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newAtomicWallet()
			w.set("user", tt.initial)
			err := w.deduct(ctx, "user", tt.amount, tt.debtLimit)
			if (err != nil) != tt.wantErr {
				t.Errorf("wantErr=%v nhưng err=%v", tt.wantErr, err)
			}
			if got := w.get("user"); got != tt.wantBalance {
				t.Errorf("balance = %d, muốn %d", got, tt.wantBalance)
			}
		})
	}
}

// ─── TestConcurrentDeductions ────────────────────────────────────────────────
//
// Kịch bản: 1000 goroutine đồng thời cố trừ tiền từ cùng một tài khoản.
// Luật bảo toàn: tiền_đã_trừ + số_dư_còn_lại == số_dư_ban_đầu
// Không bao giờ cho phép số dư âm.

func TestConcurrentDeductions(t *testing.T) {
	const (
		userID         = "player_concurrent"
		initialBalance = int64(1000)
		workers        = 1000
		betPerWorker   = int64(1)
	)

	w := newAtomicWallet()
	w.set(userID, initialBalance)

	var (
		wg           sync.WaitGroup
		successCount int64 // dùng atomic để đếm không cần mutex riêng
	)

	// Mở 1000 goroutine cùng lúc, mỗi cái cố trừ 1 xu
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.deduct(context.Background(), userID, betPerWorker, 0); err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	finalBalance := w.get(userID)
	totalDeducted := atomic.LoadInt64(&successCount) * betPerWorker

	// Kiểm tra 1: số dư không bao giờ âm
	if finalBalance < 0 {
		t.Errorf("🚨 RACE BUG: số dư âm = %d (tiền bị nhân đôi)", finalBalance)
	}

	// Kiểm tra 2: bảo toàn tổng tiền
	if totalDeducted+finalBalance != initialBalance {
		t.Errorf("🚨 TIỀN BỊ DUPE: trừ(%d) + còn(%d) = %d ≠ ban_đầu(%d)",
			totalDeducted, finalBalance, totalDeducted+finalBalance, initialBalance)
	}

	t.Logf("✅ %d/%d yêu cầu thành công | balance còn: %d",
		atomic.LoadInt64(&successCount), workers, finalBalance)
}

// ─── TestConcurrentHighLoad ──────────────────────────────────────────────────
//
// Kịch bản tải cao hơn: 1000 goroutine, mỗi cái cược 10 xu.
// Chỉ 100 goroutine đầu thành công (1000/10=100), 900 còn lại phải thất bại.

func TestConcurrentHighLoad(t *testing.T) {
	const (
		userID         = "highload_user"
		initialBalance = int64(1000)
		workers        = 1000
		betPerWorker   = int64(10)
		expectedWins   = initialBalance / betPerWorker // 100
	)

	w := newAtomicWallet()
	w.set(userID, initialBalance)

	var (
		wg           sync.WaitGroup
		successCount int64
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.deduct(context.Background(), userID, betPerWorker, 0); err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	wins := atomic.LoadInt64(&successCount)
	finalBalance := w.get(userID)

	if finalBalance < 0 {
		t.Errorf("🚨 số dư âm = %d", finalBalance)
	}
	if wins*betPerWorker+finalBalance != initialBalance {
		t.Errorf("🚨 bảo toàn tiền thất bại: %d*%d + %d ≠ %d",
			wins, betPerWorker, finalBalance, initialBalance)
	}
	if wins > expectedWins {
		t.Errorf("🚨 quá nhiều thành công: %d > tối đa %d (tiền bị dupe)", wins, expectedWins)
	}

	t.Logf("✅ %d/%d thành công (tối đa %d) | balance còn: %d",
		wins, workers, expectedWins, finalBalance)
}

// ─── TestConcurrentMultiUser ─────────────────────────────────────────────────
//
// Kịch bản: 100 user đặt cược và thắng đồng thời.
// Mỗi user: bắt đầu 200 xu, cược 100, thắng → nhận về 200. Kết quả: 300 xu.

func TestConcurrentMultiUser(t *testing.T) {
	const (
		players   = 100
		initial   = int64(200)
		bet       = int64(100)
		payout    = bet * 2                // thắng 2×
		wantFinal = initial - bet + payout // 200 - 100 + 200 = 300
	)

	w := newAtomicWallet()
	for i := 0; i < players; i++ {
		w.set(fmt.Sprintf("p%d", i), initial)
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < players; i++ {
		wg.Add(1)
		uid := fmt.Sprintf("p%d", i)
		go func(id string) {
			defer wg.Done()
			if err := w.deduct(ctx, id, bet, 0); err != nil {
				t.Errorf("deduct failed cho %s: %v", id, err)
				return
			}
			w.credit(ctx, id, payout)
		}(uid)
	}
	wg.Wait()

	for i := 0; i < players; i++ {
		uid := fmt.Sprintf("p%d", i)
		if got := w.get(uid); got != wantFinal {
			t.Errorf("player %s: balance = %d, muốn %d", uid, got, wantFinal)
		}
	}
}

// ─── TestTransferAtomicity ───────────────────────────────────────────────────
//
// Kịch bản: A và B chuyển tiền cho nhau đồng thời.
// Tổng tiền trong hệ thống phải bảo toàn (không được tạo hoặc mất tiền).

func TestTransferAtomicity(t *testing.T) {
	const (
		userA          = "alice"
		userB          = "bob"
		initialA       = int64(1000)
		initialB       = int64(1000)
		transferAmount = int64(100)
		rounds         = 200 // mỗi chiều 100 round
	)

	w := newAtomicWallet()
	w.set(userA, initialA)
	w.set(userB, initialB)

	ctx := context.Background()
	var wg sync.WaitGroup

	// 100 goroutine A→B và 100 goroutine B→A chạy cùng lúc
	for i := 0; i < rounds/2; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = w.transfer(ctx, userA, userB, transferAmount)
		}()
		go func() {
			defer wg.Done()
			_ = w.transfer(ctx, userB, userA, transferAmount)
		}()
	}
	wg.Wait()

	balA := w.get(userA)
	balB := w.get(userB)
	total := balA + balB

	// Tổng tiền phải bằng tổng ban đầu — không được tạo hoặc mất tiền
	if total != initialA+initialB {
		t.Errorf("🚨 TIỀN BỊ LEAK: A(%d) + B(%d) = %d ≠ ban_đầu(%d)",
			balA, balB, total, initialA+initialB)
	}
	// Không được âm
	if balA < 0 || balB < 0 {
		t.Errorf("🚨 số dư âm: A=%d, B=%d", balA, balB)
	}

	t.Logf("✅ Alice: %d xu | Bob: %d xu | Tổng: %d xu", balA, balB, total)
}

// ─── Test Idempotency (Chống Spam Chuyển Tiền 2 Lần) ─────────────────────────
//
// Kịch bản: Client cố gửi 2 giao dịch chuyển tiền cùng RefID/IdempotencyKey

func TestIdempotentTransfer(t *testing.T) {
	// Mô phỏng bộ đếm refID đã xử lý để chứng minh cơ chế an toàn
	processedRefs := make(map[string]bool)
	var mu sync.Mutex

	// Hàm fake transfer an toàn
	safeTransfer := func(refID string, w *atomicWallet, from, to string, amount int64) error {
		mu.Lock()
		if processedRefs[refID] {
			mu.Unlock()
			return nil // Đã xử lý, báo thành công (Idempotent)
		}
		processedRefs[refID] = true
		mu.Unlock()
		return w.transfer(context.Background(), from, to, amount)
	}

	w := newAtomicWallet()
	w.set("Alice", 1000)
	w.set("Bob", 0)

	var wg sync.WaitGroup
	// Spam 100 request cùng 1 RefID (Do lag button hoặc cố tình hack)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = safeTransfer("REF_MSG_1234", w, "Alice", "Bob", 500)
		}()
	}
	wg.Wait()

	// Kết quả: Alice chỉ bị trừ đúng 500, Bob nhận đúng 500
	if w.get("Alice") != 500 || w.get("Bob") != 500 {
		t.Errorf("LỖI Idempotency: Tiền bị trừ nhiều lần do spam. Alice=%d, Bob=%d", w.get("Alice"), w.get("Bob"))
	}
	t.Logf("✅ Chống spam thành công: Alice=%d, Bob=%d", w.get("Alice"), w.get("Bob"))
}

// ─── Test Format Số Dư Âm ───────────────────────────────────────────────────

func TestFormatCoinsNegative(t *testing.T) {
	// Helper copy từ util.go để test độc lập
	formatCoins := func(n int64) string {
		sign := ""
		if n < 0 {
			sign = "-"
			n = -n
		}
		s := fmt.Sprintf("%d", n)
		if len(s) <= 3 {
			return sign + s
		}
		var sb strings.Builder
		sb.WriteString(sign)
		mod := len(s) % 3
		for i, ch := range s {
			if i > 0 && (i-mod)%3 == 0 && mod != 0 || (i > 0 && mod == 0 && i%3 == 0) {
				sb.WriteRune('.')
			}
			sb.WriteRune(ch)
		}
		return sb.String()
	}

	if got := formatCoins(-500000); got != "-500.000" {
		t.Errorf("LỖI: formatCoins(-500000) = %s, expected -500.000", got)
	}
	if got := formatCoins(0); got != "0" {
		t.Errorf("LỖI: formatCoins(0) = %s, expected 0", got)
	}
	if got := formatCoins(-123456789); got != "-123.456.789" {
		t.Errorf("LỖI: formatCoins(-123456789) = %s, expected -123.456.789", got)
	}
}
