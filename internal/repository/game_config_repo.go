package repository

import (
	"context"
	"time"

	"github.com/discordcasino/v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GameConfigRepository struct {
	db *Database
}

func NewGameConfigRepository(db *Database) *GameConfigRepository {
	return &GameConfigRepository{db: db}
}

func (r *GameConfigRepository) GetConfig(ctx context.Context, game string) (*models.CasinoGameConfig, error) {
	var cfg models.CasinoGameConfig
	err := r.db.CasinoConfigs().FindOne(ctx, bson.M{"_id": game}).Decode(&cfg)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &cfg, err
}

func (r *GameConfigRepository) SaveConfig(ctx context.Context, cfg *models.CasinoGameConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := r.db.CasinoConfigs().ReplaceOne(
		ctx,
		bson.M{"_id": cfg.ID},
		cfg,
		options.Replace().SetUpsert(true),
	)
	return err
}

func (r *GameConfigRepository) LogAudit(ctx context.Context, log *models.ConfigAuditLog) error {
	log.ChangedAt = time.Now()
	_, err := r.db.CasinoConfigAudit().InsertOne(ctx, log)
	return err
}

func (r *GameConfigRepository) GetRecentLogs(ctx context.Context, game string, limit int64) ([]models.ConfigAuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "changed_at", Value: -1}}).SetLimit(limit)
	cursor, err := r.db.CasinoConfigAudit().Find(ctx, bson.M{"game": game}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []models.ConfigAuditLog
	err = cursor.All(ctx, &logs)
	return logs, err
}
