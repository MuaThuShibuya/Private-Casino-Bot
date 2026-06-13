package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/discordcasino/v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type WalletRepository struct {
	db *Database
}

func NewWalletRepository(db *Database) *WalletRepository {
	return &WalletRepository{db: db}
}

// ExecuteTx thực thi một transaction MongoDB an toàn (All or Nothing)
func (r *WalletRepository) ExecuteTx(ctx context.Context, fn func(ctx context.Context) error) error {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		return nil, fn(sc)
	})
	return err
}

// Deposit adds funds to a user's balance atomically.
// Uses upsert so the user doc is created if it doesn't exist yet.
func (r *WalletRepository) Deposit(ctx context.Context, userID string, amount int64, description string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	before, err := r.GetBalance(ctx, userID)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return err
	}

	now := time.Now()
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$inc": bson.M{"balance": amount},
		"$set": bson.M{"updated_at": now},
		"$setOnInsert": bson.M{
			"role":       models.RoleUser,
			"created_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)

	_, err = r.db.Users().UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("deposit: %w", err)
	}

	return r.logTransaction(ctx, &models.TransactionLog{
		UserID:        userID,
		Type:          models.TxDeposit,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  before + amount,
		Description:   description,
		CreatedAt:     now,
	})
}

// Withdraw deducts funds atomically using $inc + $gte filter.
// The filter ensures the update only matches if balance >= amount,
// making it impossible to go negative — no lock or check-then-act needed.
func (r *WalletRepository) Withdraw(ctx context.Context, userID string, amount int64, description string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	before, err := r.GetBalance(ctx, userID)
	if err != nil {
		return err
	}

	now := time.Now()
	filter := bson.M{
		"_id":     userID,
		"balance": bson.M{"$gte": amount}, // atomic guard: only update if funds exist
	}
	update := bson.M{
		"$inc": bson.M{"balance": -amount},
		"$set": bson.M{"updated_at": now},
	}

	result, err := r.db.Users().UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("withdraw: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInsufficientBalance
	}

	return r.logTransaction(ctx, &models.TransactionLog{
		UserID:        userID,
		Type:          models.TxWithdraw,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  before - amount,
		Description:   description,
		CreatedAt:     now,
	})
}

// TransferMoney moves funds from one user to another inside a MongoDB session
// transaction (Saga pattern). Either both debit+credit succeed or both roll back.
// Thêm param refID làm Idempotency Key chống spam lệnh chuyển tiền nhân đôi tài sản.
func (r *WalletRepository) TransferMoney(ctx context.Context, fromID, toID string, amount int64, refID string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if fromID == toID {
		return ErrSelfTransfer
	}

	// IDEMPOTENCY CHECK: Nếu giao dịch với refID này đã tồn tại, bỏ qua an toàn
	if refID != "" {
		count, _ := r.db.Transactions().CountDocuments(ctx, bson.M{"ref_id": refID})
		if count > 0 {
			return nil // Giao dịch đã được xử lý trước đó, báo thành công luôn
		}
	} else {
		refID = primitive.NewObjectID().Hex() // Fallback nếu không truyền
	}

	session, err := r.db.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.EndSession(ctx)

	now := time.Now()

	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Step 1: Debit sender — atomic $gte guard prevents negative balance
		senderBefore, err := r.getBalanceInSession(sc, fromID)
		if err != nil {
			return nil, err
		}

		debitFilter := bson.M{
			"_id":     fromID,
			"balance": bson.M{"$gte": amount},
		}
		debitUpdate := bson.M{
			"$inc": bson.M{"balance": -amount},
			"$set": bson.M{"updated_at": now},
		}
		res, err := r.db.Users().UpdateOne(sc, debitFilter, debitUpdate)
		if err != nil {
			return nil, fmt.Errorf("debit sender: %w", err)
		}
		if res.MatchedCount == 0 {
			return nil, ErrInsufficientBalance
		}

		// Step 2: Credit receiver — upsert creates account if not exists
		receiverBefore, err := r.getBalanceInSession(sc, toID)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}

		creditFilter := bson.M{"_id": toID}
		creditUpdate := bson.M{
			"$inc": bson.M{"balance": amount},
			"$set": bson.M{"updated_at": now},
			"$setOnInsert": bson.M{
				"role":       models.RoleUser,
				"created_at": now,
			},
		}
		_, err = r.db.Users().UpdateOne(sc, creditFilter, creditUpdate, options.Update().SetUpsert(true))
		if err != nil {
			return nil, fmt.Errorf("credit receiver: %w", err)
		}

		// Step 3: Write both transaction logs inside the same session
		logs := []interface{}{
			models.TransactionLog{
				ID:            primitive.NewObjectID(),
				UserID:        fromID,
				Type:          models.TxTransferOut,
				Amount:        amount,
				BalanceBefore: senderBefore,
				BalanceAfter:  senderBefore - amount,
				Description:   fmt.Sprintf("Transfer to %s", toID),
				RefID:         refID,
				CreatedAt:     now,
			},
			models.TransactionLog{
				ID:            primitive.NewObjectID(),
				UserID:        toID,
				Type:          models.TxTransferIn,
				Amount:        amount,
				BalanceBefore: receiverBefore,
				BalanceAfter:  receiverBefore + amount,
				Description:   fmt.Sprintf("Transfer from %s", fromID),
				RefID:         refID,
				CreatedAt:     now,
			},
		}
		_, err = r.db.Transactions().InsertMany(sc, logs)
		return nil, err
	})

	return err
}

// AdjustBalance is used by admins to give/take money — logs with appropriate type.
func (r *WalletRepository) AdjustBalance(ctx context.Context, userID string, amount int64, txType, description string) error {
	if amount == 0 {
		return ErrInvalidAmount
	}

	before, err := r.GetBalance(ctx, userID)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return err
	}

	now := time.Now()

	// For negative adjustments (take), still guard against going below zero
	var filter bson.M
	if amount < 0 {
		filter = bson.M{
			"_id":     userID,
			"balance": bson.M{"$gte": -amount},
		}
	} else {
		filter = bson.M{"_id": userID}
	}

	update := bson.M{
		"$inc": bson.M{"balance": amount},
		"$set": bson.M{"updated_at": now},
		"$setOnInsert": bson.M{
			"role":       models.RoleUser,
			"created_at": now,
		},
	}

	result, err := r.db.Users().UpdateOne(ctx, filter, update, options.Update().SetUpsert(amount > 0))
	if err != nil {
		return fmt.Errorf("adjust balance: %w", err)
	}
	if amount < 0 && result.MatchedCount == 0 {
		return ErrInsufficientBalance
	}

	after := before + amount
	return r.logTransaction(ctx, &models.TransactionLog{
		UserID:        userID,
		Type:          txType,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
		Description:   description,
		CreatedAt:     now,
	})
}

func (r *WalletRepository) GetBalance(ctx context.Context, userID string) (int64, error) {
	return r.getBalanceInSession(ctx, userID)
}

func (r *WalletRepository) getBalanceInSession(ctx context.Context, userID string) (int64, error) {
	var user models.User
	err := r.db.Users().FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, ErrUserNotFound
	}
	if err != nil {
		return 0, err
	}
	return user.Balance, nil
}

func (r *WalletRepository) GetTransactionHistory(ctx context.Context, userID string, limit int64) ([]models.TransactionLog, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(limit)

	// Lọc bỏ lịch sử cược (bet_win, bet_lose), chỉ lấy giao dịch Nạp/Rút/Chuyển
	filter := bson.M{
		"user_id": userID,
		"type": bson.M{
			"$in": []string{
				models.TxTransferIn,
				models.TxTransferOut,
				models.TxAdminGive,
				models.TxAdminTake,
				models.TxDeposit,
				models.TxWithdraw,
				models.TxRefund,
			},
		},
	}
	cursor, err := r.db.Transactions().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []models.TransactionLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// GetTransactionHistoryPaginated truy vấn lịch sử phân trang, trả về danh sách và tổng số record
func (r *WalletRepository) GetTransactionHistoryPaginated(ctx context.Context, userID string, page, pageSize int) ([]models.TransactionLog, int64, error) {
	// Lọc bỏ lịch sử cược (bet_win, bet_lose), chỉ lấy giao dịch Nạp/Rút/Chuyển
	filter := bson.M{
		"user_id": userID,
		"type": bson.M{
			"$in": []string{
				models.TxTransferIn,
				models.TxTransferOut,
				models.TxAdminGive,
				models.TxAdminTake,
				models.TxDeposit,
				models.TxWithdraw,
				models.TxRefund,
			},
		},
	}
	total, err := r.db.Transactions().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize))

	cursor, err := r.db.Transactions().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var logs []models.TransactionLog
	err = cursor.All(ctx, &logs)
	return logs, total, err
}

// ClearAllTransactions xóa toàn bộ log giao dịch khi Factory Reset
func (r *WalletRepository) ClearAllTransactions(ctx context.Context) (int64, error) {
	res, err := r.db.Transactions().DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// LogBetLoss records a loss event without changing balance (balance already deducted via Withdraw).
func (r *WalletRepository) LogBetLoss(ctx context.Context, userID string, amount int64) error {
	balance, _ := r.GetBalance(ctx, userID)
	return r.logTransaction(ctx, &models.TransactionLog{
		UserID:        userID,
		Type:          models.TxBetLose,
		Amount:        amount,
		BalanceBefore: balance + amount, // reconstructed pre-loss value
		BalanceAfter:  balance,
		Description:   fmt.Sprintf("Bet lost: %d coins", amount),
		CreatedAt:     time.Now(),
	})
}

func (r *WalletRepository) logTransaction(ctx context.Context, tx *models.TransactionLog) error {
	if tx.ID.IsZero() {
		tx.ID = primitive.NewObjectID()
	}
	_, err := r.db.Transactions().InsertOne(ctx, tx)
	return err
}

// WithdrawWithCredit trừ tiền cược với hạn mức tín dụng. Atomic qua MongoDB filter.
// Filter: balance >= betAmount - debtLimit
func (r *WalletRepository) WithdrawWithCredit(ctx context.Context, userID string, betAmount, debtLimit int64, description string) error {
	if betAmount <= 0 {
		return ErrInvalidAmount
	}

	before, err := r.GetBalance(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInsufficientBalance
		}
		return err
	}

	now := time.Now()
	// Atomic guard: chỉ cho phép nếu số tiền sau khi trừ không âm hơn debtLimit
	minBalance := betAmount - debtLimit
	filter := bson.M{
		"_id":     userID,
		"balance": bson.M{"$gte": minBalance},
	}
	update := bson.M{
		"$inc": bson.M{"balance": -betAmount},
		"$set": bson.M{"updated_at": now},
	}

	result, err := r.db.Users().UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("withdraw with credit: %w", err)
	}
	if result.MatchedCount == 0 {
		return ErrInsufficientBalance
	}

	return r.logTransaction(ctx, &models.TransactionLog{
		UserID:        userID,
		Type:          models.TxBetLose,
		Amount:        betAmount,
		BalanceBefore: before,
		BalanceAfter:  before - betAmount,
		Description:   description,
		CreatedAt:     now,
	})
}
