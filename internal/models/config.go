package models

import "time"

const GlobalConfigID = "global"

type GlobalConfig struct {
	ID                  string           `bson:"_id"`
	WhitelistedChannels []string         `bson:"whitelisted_channels"`
	AdminRoles          []string         `bson:"admin_roles"`
	IsLimitEnabled      bool             `bson:"is_limit_enabled"`
	DailyBetLimit       int64            `bson:"daily_bet_limit"`
	DebtRoleLimits      map[string]int64 `bson:"debt_role_limits"`
	Prefix              string           `bson:"prefix"`
	UpdatedAt           time.Time        `bson:"updated_at"`
}
