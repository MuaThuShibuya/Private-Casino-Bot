package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/discordcasino/v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type UserRepository struct {
	db *Database
}

func NewUserRepository(db *Database) *UserRepository {
	return &UserRepository{db: db}
}

// StartTx ghi nhận bắt đầu một transaction quan trọng (Settlement/Cancel).
func (r *UserRepository) StartTx(ctx context.Context, txID string) error {
	filter := bson.M{"_id": txID}
	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":    txID,
			"status": "processing",
		},
	}
	opts := options.Update().SetUpsert(true)
	result, err := r.db.Settlements().UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return err
	}
	if result.UpsertedCount == 0 {
		return ErrRoundAlreadyResolved
	}
	return nil
}

// CompleteTx đánh dấu giao dịch hoàn tất.
func (r *UserRepository) CompleteTx(ctx context.Context, txID string, details interface{}) error {
	filter := bson.M{"_id": txID}
	update := bson.M{
		"$set": bson.M{
			"status":      "resolved",
			"resolved_at": time.Now(),
			"details":     details,
		},
	}
	res, err := r.db.Settlements().UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("tx doc not found: %s", txID)
	}
	return nil
}

func (r *UserRepository) GetTxDoc(ctx context.Context, txID string) (bson.M, error) {
	var doc bson.M
	err := r.db.Settlements().FindOne(ctx, bson.M{"_id": txID}).Decode(&doc)
	return doc, err
}

func (r *UserRepository) GetDBName() string { return r.db.Name() }

func (r *UserRepository) FindOrCreate(ctx context.Context, userID string) (*models.User, error) {
	now := time.Now()
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":        userID,
			"balance":    int64(0),
			"role":       models.RoleUser,
			"created_at": now,
			"updated_at": now,
		},
	}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var user models.User
	err := r.db.Users().FindOneAndUpdate(ctx, filter, update, opts).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := r.db.Users().FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrUserNotFound
	}
	return &user, err
}

func (r *UserRepository) SetRole(ctx context.Context, userID, role string) error {
	_, err := r.db.Users().UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"role": role, "updated_at": time.Now()}},
	)
	return err
}

// SetDebtLimit cấu hình hạn mức đánh nợ của người dùng
func (r *UserRepository) SetDebtLimit(ctx context.Context, userID string, enabled bool, limit int64, roleID, adminID string) error {
	_, err := r.db.Users().UpdateOne(
		ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{
			"debt_enabled":    enabled,
			"debt_limit":      limit,
			"debt_role_id":    roleID,
			"debt_updated_by": adminID,
			"debt_updated_at": time.Now(),
		}},
	)
	return err
}

func (r *UserRepository) GetRole(ctx context.Context, userID string) (string, error) {
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return models.RoleUser, err
	}
	return user.Role, nil
}

func (r *UserRepository) IsOwner(ctx context.Context, userID string) (bool, error) {
	role, err := r.GetRole(ctx, userID)
	if err != nil {
		return false, err
	}
	return role == models.RoleOwner, nil
}

func (r *UserRepository) IsAdmin(ctx context.Context, userID string) (bool, error) {
	role, err := r.GetRole(ctx, userID)
	if err != nil {
		return false, err
	}
	return role == models.RoleAdmin || role == models.RoleOwner, nil
}

// BalanceLeaderboardEntry đại diện cho một người dùng trong bảng xếp hạng số dư.
type BalanceLeaderboardEntry struct {
	UserID  string `bson:"_id"`
	Balance int64  `bson:"balance"`
}

// GetBalanceLeaderboard lấy danh sách top người dùng theo số dư.
func (r *UserRepository) GetBalanceLeaderboard(ctx context.Context, limit int) ([]BalanceLeaderboardEntry, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "balance", Value: -1}}).
		SetLimit(int64(limit))

	// Lấy tất cả mọi người, bao gồm cả những người bị âm tiền
	cursor, err := r.db.Users().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var entries []BalanceLeaderboardEntry
	return entries, cursor.All(ctx, &entries)
}

// GetTopBalanceLeaderboard siêu tốc độ, chuyên dụng cho bảng Top 10 không phân trang
func (r *UserRepository) GetTopBalanceLeaderboard(ctx context.Context, limit int) ([]BalanceLeaderboardEntry, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "balance", Value: -1}}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{"_id": 1, "balance": 1}) // Chỉ kéo _id và balance về RAM để tiết kiệm tối đa

	cursor, err := r.db.Users().Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var entries []BalanceLeaderboardEntry
	return entries, cursor.All(ctx, &entries)
}

// GetBalanceLeaderboardPaginated lấy danh sách top người dùng theo trang chuyên nghiệp (dùng Skip/Limit)
func (r *UserRepository) GetBalanceLeaderboardPaginated(ctx context.Context, page, pageSize int) ([]BalanceLeaderboardEntry, int64, error) {
	// Lọc bỏ những người có số dư = 0 (Chỉ lấy dương và âm)
	filter := bson.M{"balance": bson.M{"$ne": 0}}

	total, err := r.db.Users().CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	// Dùng Projection để chỉ lấy _id (UserID) và balance, tối ưu RAM và Băng thông mạng
	opts := options.Find().
		SetSort(bson.D{{Key: "balance", Value: -1}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize)).
		SetProjection(bson.M{"_id": 1, "balance": 1})

	cursor, err := r.db.Users().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var entries []BalanceLeaderboardEntry
	err = cursor.All(ctx, &entries)
	return entries, total, err
}

// LeaderboardEntry đại diện cho một người chơi trong bảng xếp hạng game
type LeaderboardEntry struct {
	UserID string  `bson:"user_id"`
	Profit float64 `bson:"profit"`
	Plays  int     `bson:"plays"`
}

// GetGameLeaderboard lấy top 20 người chơi có lợi nhuận cao nhất của 1 game
func (r *UserRepository) GetGameLeaderboard(ctx context.Context, game string) ([]LeaderboardEntry, error) {
	opts := options.Find().SetSort(bson.D{{Key: "profit", Value: -1}}).SetLimit(20)
	cursor, err := r.db.GameHistory().Find(ctx, bson.M{"game": game}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var entries []LeaderboardEntry
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// GetGameHistoryForUser lấy thống kê lời/lỗ và lượt chơi của 1 người theo game
func (r *UserRepository) GetGameHistoryForUser(ctx context.Context, game, userID string) (*LeaderboardEntry, error) {
	var entry LeaderboardEntry
	err := r.db.GameHistory().FindOne(ctx, bson.M{"game": game, "user_id": userID}).Decode(&entry)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// ClearGameHistory xóa toàn bộ lịch sử của một game
func (r *UserRepository) ClearGameHistory(ctx context.Context, game string) error {
	_, err := r.db.GameHistory().DeleteMany(ctx, bson.M{"game": game})
	return err
}

// ClearUserGameHistory xóa toàn bộ lịch sử của một người dùng cụ thể trong một game
func (r *UserRepository) ClearUserGameHistory(ctx context.Context, game, userID string) error {
	_, err := r.db.GameHistory().DeleteOne(ctx, bson.M{"game": game, "user_id": userID})
	return err
}

// ClearAllGameHistories xóa sạch toàn bộ lịch sử game (Factory reset)
func (r *UserRepository) ClearAllGameHistories(ctx context.Context) (int64, error) {
	res, err := r.db.GameHistory().DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// ClearAllTransactions xóa toàn bộ log giao dịch khi Factory Reset
func (r *UserRepository) ClearAllTransactions(ctx context.Context) (int64, error) {
	res, err := r.db.Transactions().DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// ResetAllBalances ép toàn bộ số dư user về 0 bằng 1 lệnh duy nhất siêu tốc
func (r *UserRepository) ResetAllBalances(ctx context.Context) (int64, error) {
	res, err := r.db.Users().UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"balance": int64(0), "updated_at": time.Now()}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

// UpdateGameHistory cập nhật số tiền lời/lỗ và tăng 1 lượt chơi cho user
func (r *UserRepository) UpdateGameHistory(ctx context.Context, userID string, game string, profit float64) error {
	filter := bson.M{"user_id": userID, "game": game}
	update := bson.M{
		"$inc": bson.M{
			"profit": profit,
			"plays":  1,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := r.db.GameHistory().UpdateOne(ctx, filter, update, opts)
	return err
}
