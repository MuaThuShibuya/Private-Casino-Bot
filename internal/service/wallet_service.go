package service

import (
	"context"
	"fmt"
	"time"

	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
)

type WalletService struct {
	wallet *repository.WalletRepository
	user   *repository.UserRepository
}

func NewWalletService(wallet *repository.WalletRepository, user *repository.UserRepository) *WalletService {
	return &WalletService{wallet: wallet, user: user}
}

// ExecuteTx chạy một hàm ẩn danh bên trong MongoDB Transaction.
func (s *WalletService) ExecuteTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.wallet.ExecuteTx(ctx, fn)
}

type BalanceResult struct {
	UserID  string
	Balance int64
}

func (s *WalletService) GetBalance(ctx context.Context, userID string) (*BalanceResult, error) {
	u, err := s.user.FindOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &BalanceResult{UserID: userID, Balance: u.Balance}, nil
}

func (s *WalletService) Deposit(ctx context.Context, targetID, adminID string, amount int64) error {
	desc := fmt.Sprintf("Admin deposit by %s", adminID)
	return s.wallet.Deposit(ctx, targetID, amount, desc)
}

func (s *WalletService) Withdraw(ctx context.Context, targetID, adminID string, amount int64) error {
	desc := fmt.Sprintf("Admin withdrawal by %s", adminID)
	return s.wallet.Withdraw(ctx, targetID, amount, desc)
}

func (s *WalletService) Transfer(ctx context.Context, fromID, toID string, amount int64, refID string) error {
	// Ensure both users exist before transferring
	if _, err := s.user.FindOrCreate(ctx, fromID); err != nil {
		return err
	}
	if _, err := s.user.FindOrCreate(ctx, toID); err != nil {
		return err
	}
	return s.wallet.TransferMoney(ctx, fromID, toID, amount, refID)
}

// BetDeduct removes bet amount from user — returns balance before deduction for audit.
func (s *WalletService) BetDeduct(ctx context.Context, userID string, betAmount int64) (int64, error) {
	before, err := s.wallet.GetBalance(ctx, userID)
	if err != nil {
		return 0, err
	}
	err = s.wallet.Withdraw(ctx, userID, betAmount, fmt.Sprintf("Bet placed: %d coins", betAmount))
	return before, err
}

// BetDeductWithDebt removes bet amount from user considering debt limit.
func (s *WalletService) BetDeductWithDebt(ctx context.Context, userID string, betAmount int64, debtLimit int64) error {
	desc := fmt.Sprintf("Bet placed: %d coins", betAmount)
	// Bắt buộc dùng atomic update ở repo để chống race condition (Hack tiền)
	return s.wallet.WithdrawWithCredit(ctx, userID, betAmount, debtLimit, desc)
}

// SettleShortfallTx trừ khoản nợ Đền bàn/Thua lố, tôn trọng tuyệt đối Hạn mức Tín Dụng.
func (s *WalletService) SettleShortfallTx(ctx context.Context, userID string, amount int64, debtLimit int64) error {
	desc := fmt.Sprintf("Thanh toán nợ âm: %d coins", amount)
	return s.wallet.WithdrawWithCredit(ctx, userID, amount, debtLimit, desc)
}

// Refund trả lại tiền cho người dùng (Hủy bàn, Rollback khi lỗi).
func (s *WalletService) Refund(ctx context.Context, userID string, amount int64, reason string) error {
	desc := fmt.Sprintf("Refund: %s", reason)
	return s.wallet.AdjustBalance(ctx, userID, amount, models.TxRefund, desc)
}

// BetSettle credits winnings or logs a loss based on outcome.
func (s *WalletService) BetSettle(ctx context.Context, userID string, payout int64, won bool) error {
	if won && payout > 0 {
		desc := fmt.Sprintf("Bet win: +%d coins", payout)
		return s.wallet.AdjustBalance(ctx, userID, payout, models.TxBetWin, desc)
	}
	// Loss is already deducted via BetDeduct; just log it
	return s.wallet.LogBetLoss(ctx, userID, payout)
}

func (s *WalletService) History(ctx context.Context, userID string, limit int64) ([]models.TransactionLog, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.wallet.GetTransactionHistory(ctx, userID, limit)
}

// HistoryPaginated lấy lịch sử có phân trang cho UI /cash
func (s *WalletService) HistoryPaginated(ctx context.Context, userID string, page, pageSize int) ([]models.TransactionLog, int64, error) {
	if page < 1 {
		page = 1
	}
	return s.wallet.GetTransactionHistoryPaginated(ctx, userID, page, pageSize)
}

// AdminGive adds coins without a deduction source (owner privilege).
func (s *WalletService) AdminGive(ctx context.Context, targetID, byAdminID string, amount int64) error {
	desc := fmt.Sprintf("Given by admin %s at %s", byAdminID, time.Now().Format("2006-01-02 15:04"))
	return s.wallet.AdjustBalance(ctx, targetID, amount, models.TxAdminGive, desc)
}

// AdminTake removes coins (owner privilege).
func (s *WalletService) AdminTake(ctx context.Context, targetID, byAdminID string, amount int64) error {
	desc := fmt.Sprintf("Taken by admin %s at %s", byAdminID, time.Now().Format("2006-01-02 15:04"))
	return s.wallet.AdjustBalance(ctx, targetID, -amount, models.TxAdminTake, desc)
}
