package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/discordcasino/v2/internal/games"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
)

type SimulationResult struct {
	N         int
	RTP       float64
	HouseEdge float64
	Histogram []string
}

type CasinoConfigService struct {
	repo  *repository.GameConfigRepository
	cache sync.Map
}

func NewCasinoConfigService(repo *repository.GameConfigRepository) *CasinoConfigService {
	return &CasinoConfigService{repo: repo}
}

func (s *CasinoConfigService) LoadAll(ctx context.Context) error {
	for _, game := range models.GameNames {
		cfg, err := s.repo.GetConfig(ctx, game)
		if err != nil || cfg == nil {
			cfg = models.DefaultGameConfig(game)
			_ = s.repo.SaveConfig(ctx, cfg)
		}
		s.cache.Store(game, cfg)
	}
	return nil
}

func (s *CasinoConfigService) GetConfig(game string) *models.CasinoGameConfig {
	if val, ok := s.cache.Load(game); ok {
		return val.(*models.CasinoGameConfig)
	}
	return models.DefaultGameConfig(game)
}

func (s *CasinoConfigService) ToggleGame(ctx context.Context, game, userID string) (bool, error) {
	cfg := s.GetConfig(game)
	cfg.Enabled = !cfg.Enabled
	cfg.UpdatedBy = userID
	s.cache.Store(game, cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: game, ChangedBy: userID, Action: "toggle", NewValue: fmt.Sprintf("%v", cfg.Enabled)})
	return cfg.Enabled, err
}

func (s *CasinoConfigService) UpdateTaixiuPayout(ctx context.Context, normal, triple, hu float64, userID string) error {
	cfg := s.GetConfig("taixiu")
	if cfg.TaixiuCfg == nil {
		d := models.DefaultTaixiuConfig
		cfg.TaixiuCfg = &d
	}
	cfg.TaixiuCfg.PayoutNormal = normal
	cfg.TaixiuCfg.PayoutTriple = triple
	cfg.TaixiuCfg.PayoutHu = hu
	cfg.UpdatedBy = userID
	s.cache.Store("taixiu", cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: "taixiu", ChangedBy: userID, Action: "update_payout"})
	return err
}

func (s *CasinoConfigService) UpdateBaucuaPayout(ctx context.Context, ppm float64, userID string) error {
	cfg := s.GetConfig("baucua")
	if cfg.BaucuaCfg == nil {
		d := models.DefaultBaucuaConfig
		cfg.BaucuaCfg = &d
	}
	cfg.BaucuaCfg.PayoutPerMatch = ppm
	cfg.UpdatedBy = userID
	s.cache.Store("baucua", cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: "baucua", ChangedBy: userID, Action: "update_payout"})
	return err
}

func (s *CasinoConfigService) UpdateXidachPayout(ctx context.Context, bj, xb, nl float64, userID string) error {
	cfg := s.GetConfig("xidach")
	if cfg.XidachCfg == nil {
		d := models.DefaultXidachConfig
		cfg.XidachCfg = &d
	}
	cfg.XidachCfg.PayoutBlackjack = bj
	cfg.XidachCfg.PayoutXiBan = xb
	cfg.XidachCfg.PayoutNguLinh = nl
	cfg.UpdatedBy = userID
	s.cache.Store("xidach", cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: "xidach", ChangedBy: userID, Action: "update_payout"})
	return err
}

func (s *CasinoConfigService) UpdateBaicaoPayout(ctx context.Context, normal, sap float64, peek bool, userID string) error {
	cfg := s.GetConfig("baicao")
	if cfg.BaicaoCfg == nil {
		d := models.DefaultBaicaoConfig
		cfg.BaicaoCfg = &d
	}
	cfg.BaicaoCfg.PayoutNormal = normal
	cfg.BaicaoCfg.PayoutSap = sap
	cfg.BaicaoCfg.HostPeekEnabled = peek
	cfg.UpdatedBy = userID
	s.cache.Store("baicao", cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: "baicao", ChangedBy: userID, Action: "update_payout"})
	return err
}

func (s *CasinoConfigService) SetRigMode(ctx context.Context, game string, mode models.RigMode, userID string) error {
	cfg := s.GetConfig(game)
	cfg.RigMode = mode
	cfg.UpdatedBy = userID
	s.cache.Store(game, cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: game, ChangedBy: userID, Action: "set_rig", NewValue: string(mode)})
	return err
}

func (s *CasinoConfigService) ApplyPreset(ctx context.Context, game string, preset models.GamePreset, userID string) error {
	cfg := s.GetConfig(game)
	switch preset {
	case models.PresetFair:
		cfg.RigMode = models.RigOff
	case models.PresetCasino:
		cfg.RigMode = models.RigNormal
	case models.PresetEvent:
		cfg.RigMode = models.RigOff
	case models.PresetBrutal:
		cfg.RigMode = models.RigNightmare
	}
	cfg.UpdatedBy = userID
	s.cache.Store(game, cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: game, ChangedBy: userID, Action: "apply_preset", NewValue: string(preset)})
	return err
}

func (s *CasinoConfigService) ResetToDefault(ctx context.Context, game string, userID string) error {
	cfg := models.DefaultGameConfig(game)
	cfg.UpdatedBy = userID
	s.cache.Store(game, cfg)
	err := s.repo.SaveConfig(ctx, cfg)
	_ = s.repo.LogAudit(ctx, &models.ConfigAuditLog{Game: game, ChangedBy: userID, Action: "reset"})
	return err
}

func (s *CasinoConfigService) GetRecentLogs(ctx context.Context, game string, limit int64) ([]models.ConfigAuditLog, error) {
	return s.repo.GetRecentLogs(ctx, game, limit)
}

func (s *CasinoConfigService) SimulateTaixiu(n int) SimulationResult {
	if n <= 0 {
		n = 1000
	}
	cfg := s.GetConfig("taixiu")
	txc := models.DefaultTaixiuConfig
	if cfg.TaixiuCfg != nil {
		txc = *cfg.TaixiuCfg
	}

	taiCount, xiuCount, huCount := 0, 0, 0
	var totalBet float64 = float64(n)
	var totalReturn float64 = 0

	for i := 0; i < n; i++ {
		r := games.PlayTaixiu()
		isHu := r.Dice1 == r.Dice2 && r.Dice2 == r.Dice3 && (r.Dice1 == 1 || r.Dice1 == 6)
		if isHu {
			huCount++
		} else if r.IsTriple {
			// Các bộ ba khác, cược Tài/Xỉu thua sạch
		} else if r.Sum >= 11 {
			taiCount++
			totalReturn += 1.0 + txc.PayoutNormal // Thắng cược Tài
		} else {
			xiuCount++
		}
	}

	rtp := (totalReturn / totalBet) * 100
	houseEdge := 100.0 - rtp

	return SimulationResult{
		N:         n,
		RTP:       rtp,
		HouseEdge: houseEdge,
		Histogram: []string{
			fmt.Sprintf("Tài: %.2f%%", float64(taiCount)/float64(n)*100),
			fmt.Sprintf("Xỉu: %.2f%%", float64(xiuCount)/float64(n)*100),
			fmt.Sprintf("Hũ (111/666): %.2f%%", float64(huCount)/float64(n)*100),
		},
	}
}

func (s *CasinoConfigService) SimulateBaucua(n int) SimulationResult {
	if n <= 0 {
		n = 1000
	}
	cfg := s.GetConfig("baucua")
	bcCfg := models.DefaultBaucuaConfig
	if cfg.BaucuaCfg != nil {
		bcCfg = *cfg.BaucuaCfg
	}

	match0, match1, match2, match3 := 0, 0, 0, 0
	var totalBet float64 = float64(n)
	var totalReturn float64 = 0

	targetSide := string(games.SideBau)

	for i := 0; i < n; i++ {
		r := games.PlayBaucua()
		matches := r.MatchCount(targetSide)
		switch matches {
		case 0:
			match0++
		case 1:
			match1++
			totalReturn += 1.0 + bcCfg.PayoutPerMatch
		case 2:
			match2++
			totalReturn += 1.0 + bcCfg.PayoutPerMatch*2
		case 3:
			match3++
			totalReturn += 1.0 + bcCfg.PayoutPerMatch*3
		}
	}

	rtp := (totalReturn / totalBet) * 100
	houseEdge := 100.0 - rtp

	return SimulationResult{
		N:         n,
		RTP:       rtp,
		HouseEdge: houseEdge,
		Histogram: []string{
			fmt.Sprintf("0 khớp: %.2f%%", float64(match0)/float64(n)*100),
			fmt.Sprintf("1 khớp: %.2f%%", float64(match1)/float64(n)*100),
			fmt.Sprintf("2 khớp: %.2f%%", float64(match2)/float64(n)*100),
			fmt.Sprintf("3 khớp: %.2f%%", float64(match3)/float64(n)*100),
		},
	}
}

func CalcTaixiuHouseEdge(txc models.TaixiuConfig) (float64, float64) {
	rtp := 48.6*txc.PayoutNormal + 48.6*txc.PayoutNormal + 2.78*txc.PayoutTriple
	if rtp > 100 {
		rtp = 100
	}
	return rtp, 100 - rtp
}
