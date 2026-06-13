package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/discordcasino/v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ConfigRepository struct {
	db *Database
}

func NewConfigRepository(db *Database) *ConfigRepository {
	return &ConfigRepository{db: db}
}

var discordIDRegex = regexp.MustCompile(`\d{17,20}`)

func normalizeDiscordID(input string) string {
	matches := discordIDRegex.FindAllString(input, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return strings.TrimSpace(input)
}

func (r *ConfigRepository) Get(ctx context.Context) (*models.GlobalConfig, error) {
	var cfg models.GlobalConfig
	err := r.db.Config().FindOne(ctx, bson.M{"_id": models.GlobalConfigID}).Decode(&cfg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		// Return a safe default — no whitelisted channels, limits off
		return &models.GlobalConfig{
			ID:                  models.GlobalConfigID,
			WhitelistedChannels: []string{},
			IsLimitEnabled:      false,
			DailyBetLimit:       0,
			Prefix:              "!",
		}, nil
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "!"
	}
	return &cfg, err
}

func (r *ConfigRepository) Save(ctx context.Context, cfg *models.GlobalConfig) error {
	cfg.ID = models.GlobalConfigID
	cfg.UpdatedAt = time.Now()

	update := bson.M{
		"$set": bson.M{
			"whitelisted_channels": cfg.WhitelistedChannels,
			"admin_roles":          cfg.AdminRoles,
			"is_limit_enabled":     cfg.IsLimitEnabled,
			"daily_bet_limit":      cfg.DailyBetLimit,
			"debt_role_limits":     cfg.DebtRoleLimits,
			"prefix":               cfg.Prefix,
			"updated_at":           cfg.UpdatedAt,
		},
	}

	_, err := r.db.Config().UpdateOne(
		ctx,
		bson.M{"_id": models.GlobalConfigID},
		update,
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *ConfigRepository) AddWhitelistedChannel(ctx context.Context, channelID string) error {
	cleanID := normalizeDiscordID(channelID)
	if cleanID == "" {
		return errors.New("invalid channel ID")
	}
	_, err := r.db.Config().UpdateOne(
		ctx,
		bson.M{"_id": models.GlobalConfigID},
		bson.M{
			"$addToSet": bson.M{"whitelisted_channels": cleanID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *ConfigRepository) RemoveWhitelistedChannel(ctx context.Context, channelID string) error {
	cleanID := normalizeDiscordID(channelID)
	if cleanID == "" {
		return errors.New("invalid channel ID")
	}

	_, err := r.db.Config().UpdateOne(
		ctx,
		bson.M{"_id": models.GlobalConfigID},
		bson.M{
			"$pull": bson.M{"whitelisted_channels": cleanID},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

func (r *ConfigRepository) IsChannelWhitelisted(ctx context.Context, channelID string) (bool, error) {
	cfg, err := r.Get(ctx)
	if err != nil {
		return false, err
	}
	// If no channels are configured, allow all (open mode)
	if len(cfg.WhitelistedChannels) == 0 {
		return true, nil
	}

	targetClean := normalizeDiscordID(channelID)
	for _, id := range cfg.WhitelistedChannels {
		// So sánh chuỗi đã được làm sạch để bỏ qua data rác trong DB
		if normalizeDiscordID(id) == targetClean && targetClean != "" {
			return true, nil
		}
	}
	return false, nil
}

// AddAdminRole thêm một Discord Role ID vào danh sách quản trị viên trên DB
func (r *ConfigRepository) AddAdminRole(ctx context.Context, roleID string) error {
	_, err := r.db.Config().UpdateOne(
		ctx,
		bson.M{"_id": models.GlobalConfigID},
		bson.M{
			"$addToSet": bson.M{"admin_roles": roleID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

// RemoveAdminRole xóa một Discord Role khỏi danh sách quản trị
func (r *ConfigRepository) RemoveAdminRole(ctx context.Context, roleID string) error {
	_, err := r.db.Config().UpdateOne(
		ctx,
		bson.M{"_id": models.GlobalConfigID},
		bson.M{
			"$pull": bson.M{"admin_roles": roleID},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	return err
}

// GetAdminRoles lấy danh sách các Role được phép quản trị
func (r *ConfigRepository) GetAdminRoles(ctx context.Context) ([]string, error) {
	var doc struct {
		AdminRoles []string `bson:"admin_roles"`
	}
	err := r.db.Config().FindOne(ctx, bson.M{"_id": models.GlobalConfigID}).Decode(&doc)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, err
	}
	return doc.AdminRoles, nil
}

// SetDebtRoleLimit gắn giới hạn nợ mặc định cho một Role cụ thể
func (r *ConfigRepository) SetDebtRoleLimit(ctx context.Context, roleID string, limit int64) error {
	if roleID == "" {
		return errors.New("invalid empty role ID")
	}

	dbKey := "debt_role_limits." + roleID
	var update bson.M

	if limit > 0 {
		update = bson.M{
			"$set": bson.M{dbKey: limit, "updated_at": time.Now()},
		}
	} else {
		update = bson.M{
			"$unset": bson.M{dbKey: ""},
			"$set":   bson.M{"updated_at": time.Now()},
		}
	}

	_, err := r.db.Config().UpdateOne(ctx, bson.M{"_id": models.GlobalConfigID}, update, options.Update().SetUpsert(true))
	return err
}
