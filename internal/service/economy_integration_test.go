package service_test

// Hướng dẫn chạy Test:
// Mở Terminal và chạy lệnh: go test ./internal/service/ -v -run TestEconomy_FullIntegration

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/service"
	"go.mongodb.org/mongo-driver/bson"
)

// setupTestDB khởi tạo kết nối Database dùng riêng cho môi trường Test
func setupTestDB(t *testing.T) (*repository.Database, *service.WalletService, *repository.UserRepository, func()) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017" // URI mặc định nếu chạy local
	}
	dbName := "casino_audit_test_db"

	db, err := repository.NewDatabase(uri, dbName)
	if err != nil {
		t.Skipf("Bỏ qua Integration Test: Không thể kết nối tới MongoDB tại %s", uri)
	}

	userRepo := repository.NewUserRepository(db)
	walletRepo := repository.NewWalletRepository(db)
	walletSvc := service.NewWalletService(walletRepo, userRepo)

	ctx := context.Background()
	// Dọn dẹp rác trước khi Test
	db.Users().DeleteMany(ctx, bson.M{})
	db.Transactions().DeleteMany(ctx, bson.M{})
	db.GameHistory().DeleteMany(ctx, bson.M{})
	db.Settlements().DeleteMany(ctx, bson.M{})

	cleanup := func() {
		// Dọn dẹp rác sau khi Test xong
		db.Users().DeleteMany(ctx, bson.M{})
		db.Transactions().DeleteMany(ctx, bson.M{})
		db.GameHistory().DeleteMany(ctx, bson.M{})
		db.Settlements().DeleteMany(ctx, bson.M{})
		db.Disconnect()
	}

	return db, walletSvc, userRepo, cleanup
}

// TestEconomy_FullIntegration là bộ Test Case mô phỏng toàn bộ hành vi dòng tiền trên Discord
func TestEconomy_FullIntegration(t *testing.T) {
	_, walletSvc, userRepo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice := "111111111111111111" // Khách 1
	bob := "222222222222222222"   // Khách 2
	admin := "999999999999999999" // Admin

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 1: Mô phỏng lệnh /admin give
	t.Run("1. Admin Give Money (Tạo cung tiền)", func(t *testing.T) {
		err := walletSvc.AdminGive(ctx, alice, admin, 1000000)
		if err != nil {
			t.Fatalf("AdminGive thất bại: %v", err)
		}

		bal, _ := walletSvc.GetBalance(ctx, alice)
		if bal.Balance != 1000000 {
			t.Fatalf("Kỳ vọng Alice có 1,000,000, nhưng nhận được %d", bal.Balance)
		}
		t.Logf("✅ Admin bơm thành công 1,000,000 xu cho Alice")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 2: Mô phỏng lệnh !pay và /transfer (Kèm check chống Spam/Double Click)
	t.Run("2. Chuyển tiền & Chống Spam (Idempotency)", func(t *testing.T) {
		// Chuyển 200k từ Alice sang Bob với MessageID "msg_123"
		err := walletSvc.Transfer(ctx, alice, bob, 200000, "msg_123")
		if err != nil {
			t.Fatalf("Chuyển tiền thất bại: %v", err)
		}

		// Giả sử Alice bị lag mạng, request gửi đến Bot 2 lần với cùng MessageID "msg_123"
		err = walletSvc.Transfer(ctx, alice, bob, 200000, "msg_123")
		if err != nil {
			t.Fatalf("Idempotent Transfer không được trả về lỗi")
		}

		balA, _ := walletSvc.GetBalance(ctx, alice)
		balB, _ := walletSvc.GetBalance(ctx, bob)

		if balA.Balance != 800000 || balB.Balance != 200000 {
			t.Errorf("LỖI BẢO TOÀN TÀI SẢN! Alice: %d, Bob: %d", balA.Balance, balB.Balance)
		}
		t.Logf("✅ Chuyển tiền và Chống Spam thành công: Alice(800k), Bob(200k)")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 3: Mô phỏng đánh Game (Tài Xỉu / Bầu Cua)
	t.Run("3. Game Betting (Lời / Lỗ & Đồng bộ Game History)", func(t *testing.T) {
		// Bob cược 50k vào Tài
		err := walletSvc.BetDeductWithDebt(ctx, bob, 50000, 0)
		if err != nil {
			t.Fatalf("Trừ tiền cược thất bại: %v", err)
		}

		// Lúc này tiền Bob đang treo, DB phải là 150k
		balB, _ := walletSvc.GetBalance(ctx, bob)
		if balB.Balance != 150000 {
			t.Errorf("Kỳ vọng Bob còn 150,000 sau khi cọc, nhưng nhận được %d", balB.Balance)
		}

		// Ván kết thúc, Bob thắng, nhận lại 50k cọc + 50k tiền lời = 100k
		err = walletSvc.BetSettle(ctx, bob, 100000, true)
		if err != nil {
			t.Fatalf("Trả thưởng thất bại: %v", err)
		}

		balB2, _ := walletSvc.GetBalance(ctx, bob)
		if balB2.Balance != 250000 {
			t.Errorf("Kỳ vọng Bob có 250,000 sau khi ăn thưởng, nhưng nhận được %d", balB2.Balance)
		}

		// Đồng thời Update Game History (UI /gamehistory)
		_ = userRepo.UpdateGameHistory(ctx, bob, "taixiu", 50000.0/1000000.0) // Lời 50k = 0.05M
		t.Logf("✅ Game Betting hoạt động chuẩn xác. Tiền sau cược: Bob(250k)")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 4: Mô phỏng hiển thị UI lệnh !cash
	t.Run("4. Ledger Sync (Kiểm toán Sổ Cái Cá Nhân !cash)", func(t *testing.T) {
		// Truy vấn Sổ Cái của Bob
		logs, total, err := walletSvc.HistoryPaginated(ctx, bob, 1, 10)
		if err != nil {
			t.Fatalf("Lỗi truy vấn lịch sử: %v", err)
		}

		// Bob phải có chính xác 3 dòng: 1. Nhận tiền từ Alice, 2. Đặt cược (Trừ tiền), 3. Thắng cược (Cộng tiền)
		if total != 3 {
			t.Errorf("Kỳ vọng Bob có 3 giao dịch, thực tế: %d", total)
		}

		// YÊU CẦU TỐI THƯỢNG: Cột BalanceAfter của giao dịch MỚI NHẤT phải KHỚP 100% với Số dư ví thực tế
		if len(logs) > 0 {
			latestTx := logs[0] // Sorted DESC (Giao dịch mới nhất ở dòng đầu)
			currentBal, _ := walletSvc.GetBalance(ctx, bob)
			if latestTx.BalanceAfter != currentBal.Balance {
				t.Errorf("LỖI SỔ CÁI! BalanceAfter cuối = %d, nhưng Balance thực = %d", latestTx.BalanceAfter, currentBal.Balance)
			}
		}
		t.Logf("✅ Lịch sử ví cá nhân (!cash) đồng bộ tuyệt đối với Số Dư DB.")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 5: Mô phỏng hiển thị UI lệnh /balance
	t.Run("5. Leaderboard Sync (Kiểm toán Bảng Xếp Hạng /balance)", func(t *testing.T) {
		entries, totalUsers, err := userRepo.GetBalanceLeaderboardPaginated(ctx, 1, 10)
		if err != nil {
			t.Fatalf("Lỗi truy vấn Leaderboard: %v", err)
		}

		if totalUsers != 2 {
			t.Errorf("Kỳ vọng có 2 User (Alice, Bob) trong Leaderboard, thực tế: %d", totalUsers)
		}

		var totalSupply int64
		for _, e := range entries {
			totalSupply += e.Balance
		}

		// Alice: 800k, Bob: 250k -> Tổng tiền lưu thông phải CHÍNH XÁC là 1,050,000 xu
		if totalSupply != 1050000 {
			t.Errorf("LỖI TỔNG CUNG SERVER! Kỳ vọng: 1,050,000, Thực tế: %d", totalSupply)
		}
		t.Logf("✅ Bảng Xếp Hạng (/balance) hiển thị tiền chính xác, không xuất hiện User ảo.")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 6: Mô phỏng Số dư Âm và Hạn Mức Tín Dụng
	t.Run("6. Debt System (Tính toàn vẹn của Số dư Âm)", func(t *testing.T) {
		// Alice chỉ có 800k nhưng cố tình cược 900k (Bị âm 100k, hợp lệ vì Hạn Mức Tín Dụng = 200k)
		err := walletSvc.BetDeductWithDebt(ctx, alice, 900000, 200000)
		if err != nil {
			t.Fatalf("Lỗi: Hệ thống Tín Dụng từ chối cược hợp lệ: %v", err)
		}

		balA, _ := walletSvc.GetBalance(ctx, alice)
		if balA.Balance != -100000 {
			t.Errorf("Kỳ vọng Số dư Alice = -100,000, nhận được: %d", balA.Balance)
		}

		// Alice cố tình cược tiếp 200k (Sẽ bị Bot BLOCK vì sẽ đẩy số dư xuống -300k, vượt hạn mức 200k)
		err = walletSvc.BetDeductWithDebt(ctx, alice, 200000, 200000)
		if err == nil {
			t.Errorf("LỖI BẢO MẬT: Hệ thống cho phép User đánh thủng hạn mức tín dụng!")
		}

		// Bob chuyển khoản 150k cho Alice để cứu nợ
		err = walletSvc.Transfer(ctx, bob, alice, 150000, "tx_124")
		balA2, _ := walletSvc.GetBalance(ctx, alice)
		if balA2.Balance != 50000 {
			t.Errorf("Kỳ vọng Số dư Alice = 50,000 sau khi trả nợ, nhận được: %d", balA2.Balance)
		}
		t.Logf("✅ Số dư Âm hiển thị hoàn hảo, không có lỗi phá rào Hạn mức Tín Dụng.")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 7: Mô phỏng Rớt Mạng / Crash Hệ Thống (Saga Rollback)
	t.Run("7. Atomic Rollback (Chống Thất Thoát Tiền Khi Bot Sập)", func(t *testing.T) {
		// Thực thi một chuỗi nhiều tác vụ cược bên trong Transaction, nhưng cố tình tung lỗi giữa chừng
		err := walletSvc.ExecuteTx(ctx, func(sc context.Context) error {
			if e := walletSvc.BetDeductWithDebt(sc, bob, 50000, 0); e != nil {
				return e
			}
			// CỐ TÌNH TRẢ VỀ LỖI ĐỂ KIỂM TRA MONGODB CÓ HOÀN TIỀN CHO BOB HAY KHÔNG
			return fmt.Errorf("Simulated Server Crash")
		})

		if err == nil {
			t.Errorf("Lỗi kịch bản Test: Kỳ vọng phát hiện được lỗi 'Simulated Server Crash' nhưng giao dịch lại báo thành công")
		}

		// Kiểm tra ví Bob, phải đứng im ở mốc 100k (250k - 150k cho Alice lúc nãy), KHÔNG ĐƯỢC BỊ TRỪ 50K
		balB, _ := walletSvc.GetBalance(ctx, bob)
		if balB.Balance != 100000 {
			t.Errorf("LỖI BỐC HƠI TIỀN: Database không Rollback! Tiền Bob = %d", balB.Balance)
		}
		t.Logf("✅ Cấu trúc Atomic DB thành công. Giao dịch sập giữa chừng thì tiền tự động hồi lại 100%%.")
	})
}

// ─── BỘ TEST CHỊU TẢI CAO & RACE CONDITION (Chống Hack / Bug Game) ────────────

func TestEconomy_HighConcurrency(t *testing.T) {
	_, walletSvc, _, cleanup := setupTestDB(t)
	defer cleanup()

	// Cấp timeout cao hơn vì các vòng lặp tạo ra hàng trăm goroutine gọi DB cùng lúc
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin := "999999999999999999"

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 8: Mô phỏng Server đông người, 100 người cược cùng một phần ngàn giây
	t.Run("8. Chịu tải Cược Đồng Loạt (100 Players)", func(t *testing.T) {
		var wg sync.WaitGroup
		playerCount := 100
		initialBal := int64(100000)
		betAmt := int64(10000)

		// Bơm tiền cho 100 người
		for i := 0; i < playerCount; i++ {
			uid := fmt.Sprintf("player_%d", i)
			_ = walletSvc.AdminGive(ctx, uid, admin, initialBal)
		}

		// 100 người kích hoạt nút Cược CÙNG LÚC
		wg.Add(playerCount)
		for i := 0; i < playerCount; i++ {
			go func(idx int) {
				defer wg.Done()
				uid := fmt.Sprintf("player_%d", idx)
				_ = walletSvc.BetDeductWithDebt(ctx, uid, betAmt, 0)
			}(i)
		}
		wg.Wait()

		// Kiểm tra số dư của toàn bộ 100 người
		for i := 0; i < playerCount; i++ {
			uid := fmt.Sprintf("player_%d", i)
			bal, _ := walletSvc.GetBalance(ctx, uid)
			if bal.Balance != initialBal-betAmt {
				t.Errorf("Lỗi: Người chơi %s có %d, kỳ vọng %d", uid, bal.Balance, initialBal-betAmt)
			}
		}
		t.Logf("✅ 100 người chơi cược đồng thời không rớt nhịp nào, Atomic DB hoàn hảo.")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 9: Cross-Transfer (Chuyển khoản chéo đan xen lộn xộn)
	t.Run("9. Cross-Transfer Deadlock Check (Chống treo DB)", func(t *testing.T) {
		userA := "cross_A"
		userB := "cross_B"
		_ = walletSvc.AdminGive(ctx, userA, admin, 50000)
		_ = walletSvc.AdminGive(ctx, userB, admin, 50000)

		var wg sync.WaitGroup
		transactions := 50

		// A chuyển B 50 lần, B chuyển A 50 lần CÙNG LÚC (Gây áp lực tối đa lên Collection Users)
		for i := 0; i < transactions; i++ {
			wg.Add(2)
			go func(idx int) {
				defer wg.Done()
				_ = walletSvc.Transfer(ctx, userA, userB, 1000, fmt.Sprintf("A2B_%d", idx))
			}(i)
			go func(idx int) {
				defer wg.Done()
				_ = walletSvc.Transfer(ctx, userB, userA, 1000, fmt.Sprintf("B2A_%d", idx))
			}(i)
		}
		wg.Wait()

		balA, _ := walletSvc.GetBalance(ctx, userA)
		balB, _ := walletSvc.GetBalance(ctx, userB)

		// Bơm đi bơm lại, tiền 2 người vẫn phải nguyên vẹn 50k
		if balA.Balance != 50000 || balB.Balance != 50000 {
			t.Errorf("Lỗi Deadlock/Mất tiền: A=%d, B=%d", balA.Balance, balB.Balance)
		}
		t.Logf("✅ Chuyển tiền chéo liên tục không bị kẹt Lock DB, bảo toàn 100%%.")
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 10: Hacker cố tình Spam nút để phá rào Tín dụng
	t.Run("10. Hacker Spam Hạn Mức Tín Dụng (Race Condition)", func(t *testing.T) {
		hacker := "hacker_debt"
		// Tiền = 0. Giới hạn nợ = 50,000. Cố cược 50,000 bằng cách spam bot 50 lần/giây
		debtLimit := int64(50000)
		betAmt := int64(50000)

		var wg sync.WaitGroup
		spamCount := 50
		var successCount int32

		for i := 0; i < spamCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := walletSvc.BetDeductWithDebt(ctx, hacker, betAmt, debtLimit)
				if err == nil {
					atomic.AddInt32(&successCount, 1) // Ghi nhận nếu lọt được DB
				}
			}()
		}
		wg.Wait()

		bal, _ := walletSvc.GetBalance(ctx, hacker)
		if successCount != 1 {
			t.Errorf("LỖI BẢO MẬT: Hacker có thể đặt cược %d lần dù chỉ được nợ 1 lần!", successCount)
		}
		if bal.Balance != -50000 {
			t.Errorf("LỖI BẢO MẬT: Số dư Hacker = %d, kỳ vọng bị chặn ở -50000", bal.Balance)
		}
		t.Logf("✅ Hạn mức Tín dụng chống spam hoàn hảo. Block thành công %d/50 request rác.", spamCount-1)
	})

	// ─────────────────────────────────────────────────────────────────────────────
	// CASE 11: Vừa chuyển tiền vừa đặt cược
	t.Run("11. Vừa Cược vừa Chuyển Khoản (Chống nhân đôi tiền)", func(t *testing.T) {
		flash := "flash_user"
		friend := "friend_user"
		_ = walletSvc.AdminGive(ctx, flash, admin, 100000)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			// Cố tình cược hết tiền (100k)
			_ = walletSvc.BetDeductWithDebt(ctx, flash, 100000, 0)
		}()
		go func() {
			defer wg.Done()
			// Cùng lúc đó tẩu tán tài sản cho bạn (100k)
			_ = walletSvc.Transfer(ctx, flash, friend, 100000, "flash_tx")
		}()

		wg.Wait()

		balF, _ := walletSvc.GetBalance(ctx, flash)
		balFr, _ := walletSvc.GetBalance(ctx, friend)

		// Flash tuyệt đối không được thủng mốc 0
		if balF.Balance < 0 {
			t.Errorf("LỖI RACE CONDITION: Flash bị âm tiền (%d) dù không được cấp Hạn Mức Tín Dụng!", balF.Balance)
		}
		if balFr.Balance == 100000 {
			t.Logf("✅ Kết quả: Chuyển tiền vào trước, thao tác Cược tự động bị khóa mõm.")
		} else if balFr.Balance == 0 {
			t.Logf("✅ Kết quả: Tiền cược giữ trước, thao tác Chuyển Tiền tự động bị từ chối.")
		} else {
			t.Errorf("Kết quả phi logic! Vui lòng kiểm tra lại. Friend=%d", balFr.Balance)
		}
	})
}
