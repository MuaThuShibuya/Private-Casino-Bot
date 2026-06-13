package discord

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ai/dialogue"
	"github.com/discordcasino/v2/internal/games"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/ui"
)

type TaixiuRound struct {
	mu          sync.Mutex
	ChannelID   string
	MessageID   string
	HostID      string
	MinBet      int64
	MaxBet      int64
	Bets        []ui.TaixiuBet
	Betted      map[string]bool
	Closed      bool
	Deadline    time.Time
	Tip         string
	NeedsUpdate bool
	LastRefresh time.Time
}

var taixiuRounds sync.Map

func (b *Bot) cmdTaixiu(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, _ := b.configRepo.IsChannelWhitelisted(ctx, m.ChannelID)
	if !ok {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Lệnh casino không được phép ở kênh này.", 3*time.Second)
		return
	}

	cfg := b.casinoConfigSvc.GetConfig("taixiu")
	if !cfg.Enabled {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Game Tài Xỉu hiện đang bị đóng để bảo trì/cấu hình.", 5*time.Second)
		return
	}

	// Tính năng: Hủy tự động tạo bàn
	if len(args) > 0 && strings.ToLower(args[0]) == "stop" {
		if series := GetAutoSeries(m.ChannelID, "taixiu"); series != nil {
			if series.OwnerID == m.Author.ID || b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member) >= RoleAdmin {
				StopAutoSeries(m.ChannelID, "taixiu")
				sendTemp(s, m.ChannelID, ui.Emojis.Success+" Đã dừng chuỗi tự động tạo bàn Tài Xỉu.", 5*time.Second)
			} else {
				sendTemp(s, m.ChannelID, ui.Emojis.Error+" Bạn không có quyền dừng chuỗi này.", 5*time.Second)
			}
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có chuỗi tự động nào đang chạy.", 5*time.Second)
		}
		return
	}

	if _, exists := taixiuRounds.Load(m.ChannelID); exists {
		sendTemp(s, m.ChannelID, ui.Emojis.Loading+" Đang có ván Tài Xỉu. Chờ ván này kết thúc.", 4*time.Second)
		return
	}

	var minBet, maxBet int64
	var durationSec int = 30
	var count int = 1
	var err error

	parsedArgs := []string{}
	isTimeProvided := false
	if len(args) > 0 && strings.HasSuffix(strings.ToLower(args[0]), "s") {
		isTimeProvided = true
		timeStr := strings.TrimSuffix(strings.ToLower(args[0]), "s")
		durationSec, err = strconv.Atoi(timeStr)
		parsedArgs = args[1:]
	} else if len(args) == 3 && len(args[2]) > 2 {
		durationSec, err = strconv.Atoi(args[0])
		parsedArgs = args[1:]
	} else if len(args) == 4 {
		durationSec, err = strconv.Atoi(args[0])
		parsedArgs = args[1:]
	} else {
		parsedArgs = args
	}

	if err == nil && len(parsedArgs) >= 2 {
		minBet, err = strconv.ParseInt(parsedArgs[0], 10, 64)
		if err == nil {
			maxBet, err = strconv.ParseInt(parsedArgs[1], 10, 64)
		}
	} else if !isTimeProvided {
		err = fmt.Errorf("invalid")
	}

	if err == nil && len(parsedArgs) == 3 {
		count, err = strconv.Atoi(parsedArgs[2])
	}

	if err != nil || minBet <= 0 || maxBet < minBet || durationSec < 5 || durationSec > 300 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Cú pháp không hợp lệ. Dùng: `"+b.GetPrefix(ctx)+"tx 45s 500 3000` hoặc `"+b.GetPrefix(ctx)+"tx 45s 500 3000 5`", 5*time.Second)
		return
	}
	if count < 1 {
		count = 1
	}
	if count > 1000 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Số lần tự động tạo bàn tối đa là 1000.", 5*time.Second)
		return
	}

	if count > 1 {
		series := &AutoGameSeries{
			SeriesID: m.ChannelID + "_taixiu", GameType: "taixiu", ChannelID: m.ChannelID,
			OwnerID: m.Author.ID, GuildID: m.GuildID, TotalCount: count,
			Params: GameCreateParams{MinBet: minBet, MaxBet: maxBet, DurationSec: durationSec},
		}
		if !StartAutoSeries(series) {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Kênh này đang có chuỗi tự động Tài Xỉu chạy rồi.", 5*time.Second)
			return
		}
		b.startTaixiuRound(s, m.ChannelID, m.Author.ID, m.GuildID, minBet, maxBet, durationSec, series)
	} else {
		b.startTaixiuRound(s, m.ChannelID, m.Author.ID, m.GuildID, minBet, maxBet, durationSec, nil)
	}
}

func (b *Bot) startTaixiuRound(s *discordgo.Session, channelID, hostID, guildID string, minBet, maxBet int64, durationSec int, series *AutoGameSeries) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, exists := taixiuRounds.Load(channelID); exists {
		sendTemp(s, channelID, ui.Emojis.Loading+" Đang có ván Tài Xỉu. Chờ ván này kết thúc.", 4*time.Second)
		if series != nil {
			StopAutoSeries(channelID, "taixiu")
		}
		return
	}

	hostBalance, err := b.wallet.GetBalance(ctx, hostID)
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi lấy số dư của bạn.", 5*time.Second)
		if series != nil {
			StopAutoSeries(channelID, "taixiu")
		}
		return
	}
	debtLimit := b.GetEffectiveDebtLimit(ctx, s, guildID, hostID)
	if hostBalance.Balance+debtLimit < minBet {
		msg := fmt.Sprintf("%s Số dư / hạn mức tín dụng của bạn không đủ để làm Cái (cần ít nhất %s).", ui.Emojis.Take, formatCoins(minBet))
		if series != nil {
			msg += fmt.Sprintf("\n%s **Đã dừng chuỗi tự động tạo bàn (Đã tạo %d/%d bàn).**", ui.Emojis.Warning, series.CurrentIndex, series.TotalCount)
			StopAutoSeries(channelID, "taixiu")
			s.ChannelMessageSend(channelID, msg)
		} else {
			sendTemp(s, channelID, msg, 5*time.Second)
		}
		return
	}

	tip := GetRandomTip(b.GetPrefix(ctx))
	if series != nil {
		series.CurrentIndex++
		tip = fmt.Sprintf("Tự động tạo bàn: %d/%d | %s", series.CurrentIndex, series.TotalCount, tip)
	}

	round := &TaixiuRound{
		ChannelID:   channelID,
		HostID:      hostID,
		MinBet:      minBet,
		MaxBet:      maxBet,
		Bets:        []ui.TaixiuBet{},
		Betted:      make(map[string]bool),
		Deadline:    time.Now().Add(time.Duration(durationSec) * time.Second),
		Tip:         tip,
		LastRefresh: time.Now(),
	}
	taixiuRounds.Store(channelID, round)

	embed := ui.BuildTaixiuEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: ui.TaixiuButtons(s, false),
	})
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi tạo bàn.", 5*time.Second)
		taixiuRounds.Delete(channelID)
		if series != nil {
			StopAutoSeries(channelID, "taixiu")
		}
		return
	}

	round.MessageID = msg.ID
	go b.runTaixiuCountdown(s, round)
}

func (b *Bot) runTaixiuCountdown(s *discordgo.Session, round *TaixiuRound) {
	ticker := time.NewTicker(ActionDebounceDelay)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		round.mu.Lock()
		if round.Closed {
			round.mu.Unlock()
			return
		}

		if now.After(round.Deadline) {
			round.mu.Unlock()
			b.resolveTaixiu(s, round)
			return
		}

		needsUpdate := round.NeedsUpdate
		shouldRefresh := now.Sub(round.LastRefresh) >= GameMessageRefreshInterval

		if shouldRefresh {
			round.LastRefresh = now
			round.NeedsUpdate = false
			needsUpdate = false
		} else if needsUpdate {
			round.NeedsUpdate = false
		}

		oldMsgID := round.MessageID
		embed := ui.BuildTaixiuEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
		embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
		round.mu.Unlock()

		if shouldRefresh {
			if oldMsgID != "" {
				_ = s.ChannelMessageDelete(round.ChannelID, oldMsgID)
			}
			msg, err := s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{Embed: embed, Components: ui.TaixiuButtons(s, false)})
			if err == nil {
				round.mu.Lock()
				round.MessageID = msg.ID
				round.mu.Unlock()
			}
		} else if needsUpdate && oldMsgID != "" {
			embeds := []*discordgo.MessageEmbed{embed}
			comps := ui.TaixiuButtons(s, false)
			s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: oldMsgID, Channel: round.ChannelID, Embeds: &embeds, Components: &comps,
			})
		}
	}
}

func (b *Bot) handleTaixiuButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 2 {
		return
	}
	action := parts[1]
	userID := interactionUserID(i)

	val, exists := taixiuRounds.Load(i.ChannelID)
	var round *TaixiuRound
	if exists {
		round = val.(*TaixiuRound)
	}

	if action == "continue" {
		hostID := parts[2]
		if userID != hostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn mới có thể tạo ván mới.")
			return
		}

		minBet, _ := strconv.ParseInt(parts[3], 10, 64)
		maxBet, _ := strconv.ParseInt(parts[4], 10, 64)

		if _, exists := taixiuRounds.Load(i.ChannelID); exists {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang có ván Tài Xỉu khác trong kênh. Chờ ván đó kết thúc.")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hostBalance, err := b.wallet.GetBalance(ctx, hostID)
		debtLimit := b.GetEffectiveDebtLimit(ctx, s, i.GuildID, hostID)
		if err != nil || hostBalance.Balance+debtLimit < minBet {
			replyEphemeral(s, i, ui.Emojis.Take+" Số dư hoặc hạn mức tín dụng của bạn không đủ khả dụng để tiếp tục làm Cái.")
			return
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Components: &comps})

		StopAutoSeries(i.ChannelID, "taixiu") // Tắt chuỗi auto nếu có
		round := &TaixiuRound{ChannelID: i.ChannelID, HostID: hostID, MinBet: minBet, MaxBet: maxBet, Bets: []ui.TaixiuBet{}, Betted: make(map[string]bool), Deadline: time.Now().Add(30 * time.Second), Tip: GetRandomTip(b.GetPrefix(ctx))}
		taixiuRounds.Store(i.ChannelID, round)

		embed := ui.BuildTaixiuEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
		embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
		msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
			Embed:      embed,
			Components: ui.TaixiuButtons(s, false),
		})
		if err == nil {
			round.MessageID = msg.ID
			go b.runTaixiuCountdown(s, round)
		}
		return
	}

	val, exists = taixiuRounds.Load(i.ChannelID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Không có ván Tài Xỉu nào đang mở.")
		return
	}
	round = val.(*TaixiuRound)

	// PHẦN 4: Yêu cầu Tài xỉu bắt buộc button nằm dưới message mới nhất
	if i.Message.ID != round.MessageID && action != "cancel" {
		replyEphemeral(s, i, ui.Emojis.Warning+" Ván này đã trôi qua hoặc bàn đã được làm mới. Vui lòng kéo xuống dưới cùng để cược ở bảng mới nhất.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if action == "cancel" {
		if userID != round.HostID {
			if !HasAdminAccess(b, ctx, s, i) {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Nhà cái (Host) hoặc Admin mới có thể hủy bàn.")
				return
			}
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		round.mu.Lock()
		if round.Closed {
			round.mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Warning+" Bàn đã chốt sổ, không thể hủy.")
			return
		}
		round.Closed = true
		round.mu.Unlock()
		StopAutoSeries(round.ChannelID, "taixiu")
		taixiuRounds.Delete(round.ChannelID)

		for _, bet := range round.Bets {
			b.wallet.Refund(ctx, bet.UserID, bet.Amount, "Hủy bàn Tài Xỉu")
			b.wallet.Refund(ctx, round.HostID, bet.Escrow, "Hoàn Escrow Hủy bàn")
		}
		embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Tài Xỉu đã bị hủy bởi <@" + userID + ">. Tiền cược đã được hoàn lại.")}
		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: round.MessageID, Channel: round.ChannelID, Embeds: &embeds, Components: &comps,
		})
		deleteAfter(s, round.ChannelID, round.MessageID, 3*time.Second)
		return
	}

	if action == "tai" || action == "xiu" || action == "chan" || action == "le" || action == "hu" {
		if userID == round.HostID {
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn là Nhà Cái, không thể tự cược.")
			return
		}

		round.mu.Lock()
		if round.Closed {
			round.mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Loading+" Ván đã chốt sổ.")
			return
		}
		if round.Betted[userID] {
			round.mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Mỗi ván chỉ được cược 1 lần.")
			return
		}
		min := round.MinBet
		max := round.MaxBet
		round.mu.Unlock()

		sideName := ""
		switch action {
		case "tai":
			sideName = "Tài"
		case "xiu":
			sideName = "Xỉu"
		case "chan":
			sideName = "Chẵn"
		case "le":
			sideName = "Lẻ"
		case "hu":
			sideName = "Hũ"
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "taixiu:modal:" + action,
				Title:    "Cược " + sideName,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "amount",
							Label:       fmt.Sprintf("Nhập số tiền (Min: %d, Max: %d)", min, max),
							Style:       discordgo.TextInputShort,
							Placeholder: fmt.Sprintf("Ví dụ: %d", min),
							Required:    true,
						},
					}},
				},
			},
		})
		return
	}
}

func (b *Bot) handleTaixiuModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	parts := strings.Split(data.CustomID, ":")
	if len(parts) < 3 {
		return
	}
	side := parts[2]
	userID := interactionUserID(i)

	val, exists := taixiuRounds.Load(i.ChannelID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Không có ván Tài Xỉu nào đang mở.")
		return
	}
	round := val.(*TaixiuRound)

	amountStr := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	amount, err := strconv.ParseInt(strings.TrimSpace(amountStr), 10, 64)
	if err != nil {
		replyEphemeral(s, i, ui.Emojis.Error+" Số tiền không hợp lệ.")
		return
	}

	// Defer Message ngay để fix interaction failed
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

	round.mu.Lock()

	if round.Closed {
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Loading+" Ván đã chốt sổ.")
		return
	}
	if round.Betted[userID] {
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Warning+" Bạn đã cược rồi.")
		return
	}

	if amount < round.MinBet || amount > round.MaxBet {
		round.mu.Unlock()
		followupEphemeral(s, i, fmt.Sprintf("%s Số tiền cược phải từ **%d** đến **%d**.", ui.Emojis.Warning, round.MinBet, round.MaxBet))
		return
	}

	// Khóa tạm thời (Optimistic Lock) để chống bấm 2 lần liên tục
	round.Betted[userID] = true
	round.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := b.casinoConfigSvc.GetConfig("taixiu")
	txCfg := models.DefaultTaixiuConfig
	if cfg.TaixiuCfg != nil {
		txCfg = *cfg.TaixiuCfg
	}
	payout := txCfg.PayoutNormal
	if side == "hu" {
		payout = txCfg.PayoutHu
	}
	escrowAmount := int64(math.Round(float64(amount) * payout))

	if err := b.SafeBetDeduct(ctx, s, i.GuildID, userID, amount); err != nil {
		round.mu.Lock()
		delete(round.Betted, userID) // Revert khóa tạm thời
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Take+" Số dư hoặc hạn mức của bạn không đủ.")
		return
	}
	// Escrow Host
	if err := b.SafeBetDeduct(ctx, s, i.GuildID, round.HostID, escrowAmount); err != nil {
		b.wallet.Refund(ctx, userID, amount, "Trả cược vì Host thiếu Escrow") // Refund
		round.mu.Lock()
		delete(round.Betted, userID) // Revert khóa tạm thời
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Take+" Nhà cái đã hết vốn tạm thời để nhận thêm cược này.")
		return
	}

	round.mu.Lock()
	// Chống lỗi: Trong lúc mạng giật gọi DB, ván cược đã bị hủy hoặc chốt sổ
	if round.Closed {
		round.mu.Unlock()
		b.wallet.Refund(ctx, userID, amount, "Ván đã chốt sổ")
		b.wallet.Refund(ctx, round.HostID, escrowAmount, "Hoàn Escrow (Ván đã chốt)")
		followupEphemeral(s, i, ui.Emojis.Loading+" Ván đã chốt sổ trong lúc bạn nạp tiền, đã hoàn lại!")
		return
	}

	round.Bets = append(round.Bets, ui.TaixiuBet{UserID: userID, Side: side, Amount: amount, Escrow: escrowAmount})
	round.NeedsUpdate = true // Kích hoạt Debounce Auto-edit
	round.mu.Unlock()

	followupEphemeral(s, i, ui.Emojis.Success+" Đặt cược thành công!")
}

func (b *Bot) resolveTaixiu(s *discordgo.Session, round *TaixiuRound) {
	round.mu.Lock()
	if round.Closed {
		round.mu.Unlock()
		return
	}
	round.Closed = true
	round.mu.Unlock()
	taixiuRounds.Delete(round.ChannelID)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	isAdmin, _ := b.userRepo.IsAdmin(ctx, round.HostID)
	isOwner, _ := b.userRepo.IsOwner(ctx, round.HostID)
	isPrivileged := isAdmin || isOwner

	result := games.PlayTaixiu()

	// Helper scope cục bộ dùng cho Rig Mode testing (Tránh capture lỗi biến cũ)
	tempCheckWin := func(r games.TaixiuResult, side string) bool {
		isHu := r.Dice1 == r.Dice2 && r.Dice2 == r.Dice3 && (r.Dice1 == 1 || r.Dice1 == 6)
		if isHu {
			return side == "hu"
		}
		sum := int64(r.Dice1 + r.Dice2 + r.Dice3)
		switch side {
		case "tai":
			return sum >= 11 && sum <= 17
		case "xiu":
			return sum >= 4 && sum <= 10
		case "chan":
			return sum%2 == 0
		case "le":
			return sum%2 != 0
		}
		return false
	}

	cfg := b.casinoConfigSvc.GetConfig("taixiu")
	rigProb := getRigProb(cfg.RigMode)

	txCfg := models.DefaultTaixiuConfig
	if cfg.TaixiuCfg != nil {
		txCfg = *cfg.TaixiuCfg
	}

	if isPrivileged && len(round.Bets) > 0 && rigProb > 0 && games.RandInt(100) < int64(rigProb) {
		bestResult := result
		var bestHostNet int64

		for _, bet := range round.Bets {
			multiplier := txCfg.PayoutNormal
			if bet.Side == "hu" {
				multiplier = txCfg.PayoutHu
			}
			profitNet := int64(math.Round(float64(bet.Amount) * multiplier))
			if tempCheckWin(bestResult, bet.Side) {
				bestHostNet -= profitNet
			} else {
				bestHostNet += bet.Amount
			}
		}

		for i := 0; i < 20; i++ {
			tempResult := games.PlayTaixiu()
			tempIsHuRule := tempResult.Dice1 == tempResult.Dice2 && tempResult.Dice2 == tempResult.Dice3 && (tempResult.Dice1 == 1 || tempResult.Dice1 == 6)

			if tempIsHuRule {
				continue
			}

			var tempHostNet int64
			for _, bet := range round.Bets {
				multiplier := txCfg.PayoutNormal
				if bet.Side == "hu" {
					multiplier = txCfg.PayoutHu
				}
				profitNet := int64(math.Round(float64(bet.Amount) * multiplier))
				if tempCheckWin(tempResult, bet.Side) {
					tempHostNet -= profitNet
				} else {
					tempHostNet += bet.Amount
				}
			}
			if tempHostNet > bestHostNet {
				bestHostNet = tempHostNet
				bestResult = tempResult
			}
		}
		result = bestResult
	}

	// BÂY GIỜ MỚI TÍNH TOÁN sumDice VÀ isHuRule DỰA TRÊN KẾT QUẢ CHÍNH THỨC CUỐI CÙNG
	isHuRule := result.Dice1 == result.Dice2 && result.Dice2 == result.Dice3 && (result.Dice1 == 1 || result.Dice1 == 6)
	sumDice := int64(result.Dice1 + result.Dice2 + result.Dice3)

	checkWinStrict := func(side string) bool {
		if isHuRule {
			return side == "hu"
		}
		switch side {
		case "tai":
			return sumDice >= 11 && sumDice <= 17
		case "xiu":
			return sumDice >= 4 && sumDice <= 10
		case "chan":
			return sumDice%2 == 0
		case "le":
			return sumDice%2 != 0
		}
		return false
	}

	animEmbed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s NHÀ CÁI ĐANG LẮC XÚC XẮC...%s", ui.Emojis.TaixiuHu, ui.Emojis.TaixiuHu),
		Color: 0x000000,
	}
	animContent := fmt.Sprintf("%s %s %s %s %s", ui.Emojis.DiceRolling, ui.Emojis.SpaceAnimation, ui.Emojis.DiceRolling, ui.Emojis.SpaceAnimation, ui.Emojis.DiceRolling)

	// --- MESSAGE LIFECYCLE: Xóa tin nhắn betting cũ và gửi tin Animation mới để kéo bàn xuống ---
	_ = s.ChannelMessageDelete(round.ChannelID, round.MessageID)

	animMsg, err := s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{
		Content: animContent, Embed: animEmbed,
	})

	if err == nil {
		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.Emojis.DiceFace(result.Dice1), ui.Emojis.SpaceAnimation, ui.Emojis.DiceRolling, ui.Emojis.SpaceAnimation, ui.Emojis.DiceRolling)
		embeds1 := []*discordgo.MessageEmbed{animEmbed}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &embeds1,
		})

		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.Emojis.DiceFace(result.Dice1), ui.Emojis.SpaceAnimation, ui.Emojis.DiceFace(result.Dice2), ui.Emojis.SpaceAnimation, ui.Emojis.DiceRolling)
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &embeds1,
		})

		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.Emojis.DiceFace(result.Dice1), ui.Emojis.SpaceAnimation, ui.Emojis.DiceFace(result.Dice2), ui.Emojis.SpaceAnimation, ui.Emojis.DiceFace(result.Dice3))
		emptyEmbeds := []*discordgo.MessageEmbed{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &emptyEmbeds,
		})
		time.Sleep(1 * time.Second)
	}

	var hostNet int64
	var highestWin int64
	var bestPlayer string
	hasHu := false

	// Struct để map kết quả truyền sang UI
	playerResults := make(map[string]ui.TaixiuPlayerResult)

	for _, bet := range round.Bets {
		escrow := bet.Escrow
		profitNet := bet.Escrow // In Taixiu, profitNet always exactly equals escrow

		isWin := checkWinStrict(bet.Side)
		if isWin {
			// Thắng: Trả lại tiền cược + tiền ăn
			b.wallet.BetSettle(ctx, bet.UserID, bet.Amount+profitNet, true)
			hostNet -= profitNet

			winAmt := profitNet
			if bet.Side == "hu" {
				hasHu = true
			}
			if winAmt > highestWin {
				highestWin = winAmt
				bestPlayer = bet.UserID
			}
		} else {
			// Thua: Host thu tiền cược của khách + nhận lại escrow
			b.wallet.BetSettle(ctx, round.HostID, escrow+bet.Amount, true)
			hostNet += bet.Amount
		}

		// Lấy số dư mới nhất
		bal, _ := b.wallet.GetBalance(ctx, bet.UserID)
		newBalance := int64(0)
		if bal != nil {
			newBalance = bal.Balance
		}

		displayNet := profitNet
		if !isWin {
			displayNet = -bet.Amount
		}

		playerResults[bet.UserID] = ui.TaixiuPlayerResult{
			IsWin:   isWin,
			Net:     displayNet,
			Balance: newBalance,
			Side:    bet.Side,
			Amount:  bet.Amount,
		}

		// Cập nhật DB cho người chơi
		_ = b.userRepo.UpdateGameHistory(ctx, bet.UserID, "taixiu", float64(displayNet)/1000000.0)
	}

	_ = b.userRepo.UpdateGameHistory(ctx, round.HostID, "taixiu", float64(hostNet)/1000000.0)
	fmt.Printf("[GAME AUDIT] TAIXIU | Table: %s | Dice: %d-%d-%d (Sum: %d) | HostNet: %d\n", round.MessageID, result.Dice1, result.Dice2, result.Dice3, sumDice, hostNet)

	dialogue.GetManager().MarkActivity(round.ChannelID)
	npcDialog := ""
	if hasHu {
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.Jackpot, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Tài Xỉu", "channel": round.ChannelID})
	} else if hostNet > 0 && len(round.Bets) > 2 {
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.DealerDominating, map[string]string{"game": "Tài Xỉu", "channel": round.ChannelID})
	} else if highestWin > 0 {
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.WinBig, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Tài Xỉu", "channel": round.ChannelID})
	} else if len(round.Bets) > 0 {
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.TableIdle, map[string]string{"game": "Tài Xỉu", "channel": round.ChannelID})
	}

	// Truyền isHuRule vào để UI biết có phải Hũ thật không (vì struct games.TaixiuResult bên trong đang không đáng tin)
	embed := ui.BuildTaixiuResultEmbed(round.HostID, result, playerResults, hostNet, isHuRule)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Ván kết thúc! | " + round.Tip}

	// CACHE KẾT QUẢ ĐỂ KHÔI PHỤC KHI BỊ RATELIMIT
	CacheGameResult(round.ChannelID, "Tài Xỉu", npcDialog, embed, ui.TaixiuResultButtons(round.HostID, round.MinBet, round.MaxBet, s))

	// Luôn gửi tin nhắn mới cho kết quả để nó hiển thị dưới cùng của kênh chat,
	// tránh bị trôi tin nhắn trong lúc chờ animation lắc xúc xắc (kéo dài ~7 giây).
	s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{
		Content:    npcDialog,
		Embed:      embed,
		Components: ui.TaixiuResultButtons(round.HostID, round.MinBet, round.MaxBet, s),
	})

	// Xử lý tạo ván tiếp theo nếu có chuỗi Auto
	series := GetAutoSeries(round.ChannelID, "taixiu")
	if series != nil && series.Active && series.OwnerID == round.HostID {
		if series.CurrentIndex < series.TotalCount {
			go func() {
				time.Sleep(3 * time.Second)
				if GetAutoSeries(round.ChannelID, "taixiu") != nil {
					b.startTaixiuRound(s, round.ChannelID, round.HostID, series.GuildID, series.Params.MinBet, series.Params.MaxBet, series.Params.DurationSec, series)
				}
			}()
		} else {
			StopAutoSeries(round.ChannelID, "taixiu")
			s.ChannelMessageSend(round.ChannelID, fmt.Sprintf("%s <@%s> Đã hoàn thành chuỗi tự động tạo bàn Tài Xỉu.", ui.Emojis.Success, round.HostID))
		}
	}
}

// Repost helper hỗ trợ lệnh !host
func (b *Bot) repostTaixiuTable(s *discordgo.Session, channelID, userID string) bool {
	val, exists := taixiuRounds.Load(channelID)
	if !exists {
		return false
	}
	round := val.(*TaixiuRound)

	round.mu.Lock()
	defer round.mu.Unlock()
	if round.HostID != userID || round.Closed {
		return false
	}

	oldMsgID := round.MessageID
	embed := ui.BuildTaixiuEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{Embed: embed, Components: ui.TaixiuButtons(s, false)})
	if err == nil {
		round.MessageID = msg.ID
		if oldMsgID != "" {
			_ = s.ChannelMessageDelete(channelID, oldMsgID)
		}
		return true
	}
	return false
}
