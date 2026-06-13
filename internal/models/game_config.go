package models

import "time"

type RigMode string

const (
	RigOff       RigMode = "off"
	RigLight     RigMode = "light"
	RigNormal    RigMode = "normal"
	RigHard      RigMode = "hard"
	RigNightmare RigMode = "nightmare"
)

func (r RigMode) Label() string {
	switch r {
	case RigOff:
		return "TẮT (0%)"
	case RigLight:
		return "NHẸ (40%)"
	case RigNormal:
		return "BÌNH THƯỜNG (65%)"
	case RigHard:
		return "NẶNG (80%)"
	case RigNightmare:
		return "ÁC MỘNG (95%)"
	}
	return string(r)
}

type GamePreset string

const (
	PresetFair   GamePreset = "fair"
	PresetCasino GamePreset = "casino"
	PresetEvent  GamePreset = "event"
	PresetBrutal GamePreset = "brutal"
)

func (p GamePreset) Label() string {
	switch p {
	case PresetFair:
		return "Công Bằng"
	case PresetCasino:
		return "Casino (Mặc Định)"
	case PresetEvent:
		return "Sự Kiện"
	case PresetBrutal:
		return "Tàn Bạo"
	}
	return string(p)
}

var GameNames = []string{"taixiu", "baucua", "xidach", "baicao"}

type CasinoGameConfig struct {
	ID        string        `bson:"_id"`
	Enabled   bool          `bson:"enabled"`
	RigMode   RigMode       `bson:"rig_mode"`
	UpdatedBy string        `bson:"updated_by"`
	UpdatedAt time.Time     `bson:"updated_at"`
	TaixiuCfg *TaixiuConfig `bson:"taixiu_cfg,omitempty"`
	BaucuaCfg *BaucuaConfig `bson:"baucua_cfg,omitempty"`
	XidachCfg *XidachConfig `bson:"xidach_cfg,omitempty"`
	BaicaoCfg *BaicaoConfig `bson:"baicao_cfg,omitempty"`
}

type TaixiuConfig struct {
	PayoutNormal float64 `bson:"payout_normal"`
	PayoutTriple float64 `bson:"payout_triple"`
	PayoutHu     float64 `bson:"payout_hu"`
	MaxStreak    int     `bson:"max_streak"`
}

type BaucuaConfig struct {
	PayoutPerMatch float64 `bson:"payout_per_match"`
}

type XidachConfig struct {
	DealerStandAt   int     `bson:"dealer_stand_at"`
	PayoutBlackjack float64 `bson:"payout_blackjack"`
	PayoutXiBan     float64 `bson:"payout_xi_ban"`
	PayoutNguLinh   float64 `bson:"payout_ngu_linh"`
}

type BaicaoConfig struct {
	PayoutNormal    float64 `bson:"payout_normal"`
	PayoutSap       float64 `bson:"payout_sap"`
	HostPeekEnabled bool    `bson:"host_peek_enabled"`
}

type ConfigAuditLog struct {
	Game      string    `bson:"game"`
	ChangedBy string    `bson:"changed_by"`
	Action    string    `bson:"action"`
	NewValue  string    `bson:"new_value"`
	ChangedAt time.Time `bson:"changed_at"`
}

var DefaultTaixiuConfig = TaixiuConfig{PayoutNormal: 1.95, PayoutTriple: 24.0, PayoutHu: 10.0, MaxStreak: 5}
var DefaultBaucuaConfig = BaucuaConfig{PayoutPerMatch: 1.0}
var DefaultXidachConfig = XidachConfig{DealerStandAt: 16, PayoutBlackjack: 2.0, PayoutXiBan: 3.0, PayoutNguLinh: 2.0}
var DefaultBaicaoConfig = BaicaoConfig{PayoutNormal: 1.0, PayoutSap: 3.0, HostPeekEnabled: false}

func DefaultGameConfig(game string) *CasinoGameConfig {
	cfg := &CasinoGameConfig{ID: game, Enabled: true, RigMode: RigNormal}
	switch game {
	case "taixiu":
		cfg.TaixiuCfg = &DefaultTaixiuConfig
	case "baucua":
		cfg.BaucuaCfg = &DefaultBaucuaConfig
	case "xidach":
		cfg.XidachCfg = &DefaultXidachConfig
	case "baicao":
		cfg.BaicaoCfg = &DefaultBaicaoConfig
	}
	return cfg
}
