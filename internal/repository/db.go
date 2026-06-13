package repository

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Tên các collection trong MongoDB
const (
	colUsers             = "users"
	colConfig            = "config"
	colTransactions      = "transaction_logs"
	colGameHistory       = "game_history"
	colCasinoConfigs     = "casino_configs"
	colCasinoConfigAudit = "casino_config_audit"
	colSettlements       = "game_round_settlements"
)

// Database bọc mongo.Client và mongo.Database để tái sử dụng connection.
type Database struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewDatabase kết nối tới MongoDB Atlas và đảm bảo các index cần thiết.
func NewDatabase(uri, dbName string) (*Database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	log.Println("[DB] Đã kết nối MongoDB Atlas")

	d := &Database{
		client: client,
		db:     client.Database(dbName),
	}

	if err := d.ensureIndexes(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// ensureIndexes tạo các index cần thiết nếu chưa có.
// Index trên transaction_logs giúp truy vấn lịch sử nhanh hơn.
func (d *Database) ensureIndexes(ctx context.Context) error {
	// Đánh index balance giảm dần để lấy Leaderboard phú hộ cực nhanh
	usersCol := d.db.Collection(colUsers)
	_, err := usersCol.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "balance", Value: -1}},
		},
		{
			// Tối ưu query tìm Owner/Admin
			Keys: bson.D{{Key: "role", Value: 1}},
		},
	})
	if err != nil {
		return err
	}

	txCol := d.db.Collection(colTransactions)
	_, err = txCol.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			// Index kép: user_id tăng dần + created_at giảm dần — tối ưu cho !history
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "created_at", Value: -1},
			},
		},
		{
			// Sparse index trên ref_id — chỉ index document có trường này (transfer)
			Keys:    bson.D{{Key: "ref_id", Value: 1}},
			Options: options.Index().SetSparse(true),
		},
	})
	if err != nil {
		return err
	}

	ghCol := d.db.Collection(colGameHistory)
	_, _ = ghCol.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "game", Value: 1},
				{Key: "profit", Value: -1},
			},
		},
	})

	log.Println("[DB] Indexes đã sẵn sàng")
	return nil
}

func (d *Database) Name() string          { return d.db.Name() }
func (d *Database) Client() *mongo.Client { return d.client }

func (d *Database) Users() *mongo.Collection         { return d.db.Collection(colUsers) }
func (d *Database) Config() *mongo.Collection        { return d.db.Collection(colConfig) }
func (d *Database) Transactions() *mongo.Collection  { return d.db.Collection(colTransactions) }
func (d *Database) GameHistory() *mongo.Collection   { return d.db.Collection(colGameHistory) }
func (d *Database) CasinoConfigs() *mongo.Collection { return d.db.Collection(colCasinoConfigs) }
func (d *Database) CasinoConfigAudit() *mongo.Collection {
	return d.db.Collection(colCasinoConfigAudit)
}
func (d *Database) Settlements() *mongo.Collection { return d.db.Collection(colSettlements) }

// Disconnect đóng kết nối MongoDB một cách an toàn khi bot tắt.
func (d *Database) Disconnect() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.client.Disconnect(ctx); err != nil {
		log.Printf("[DB] Lỗi khi đóng kết nối: %v", err)
	}
}
