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

type BaucuaRound struct {
	mu          sync.Mutex
	ChannelID   string
	MessageID   string
	HostID      string
	MinBet      int64
	MaxBet      int64
	Bets        []ui.BaucuaBet
	Betted      map[string]map[string]bool // userID -> side -> true
	Closed      bool
	Deadline    time.Time
	Tip         string
	NeedsUpdate bool
	LastRefresh time.Time
}

var baucuaRounds sync.Map

func (b *Bot) cmdBaucua(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, _ := b.configRepo.IsChannelWhitelisted(ctx, m.ChannelID)
	if !ok {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Lệnh casino không được phép ở kênh này.", 3*time.Second)
		return
	}

	if len(args) > 0 && strings.ToLower(args[0]) == "stop" {
		if series := GetAutoSeries(m.ChannelID, "baucua"); series != nil {
			if series.OwnerID == m.Author.ID || b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member) >= RoleAdmin {
				StopAutoSeries(m.ChannelID, "baucua")
				sendTemp(s, m.ChannelID, ui.Emojis.Success+" Đã dừng chuỗi tự động tạo bàn Bầu Cua.", 5*time.Second)
			} else {
				sendTemp(s, m.ChannelID, ui.Emojis.Error+" Bạn không có quyền dừng chuỗi này.", 5*time.Second)
			}
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có chuỗi tự động nào đang chạy.", 5*time.Second)
		}
		return
	}

	if _, exists := baucuaRounds.Load(m.ChannelID); exists {
		sendTemp(s, m.ChannelID, ui.Emojis.Loading+" Đang có ván Bầu Cua. Chờ ván này kết thúc.", 4*time.Second)
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
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Cú pháp không hợp lệ. Dùng: `"+b.GetPrefix(ctx)+"bc 45s 500 3000` hoặc `"+b.GetPrefix(ctx)+"bc 45s 500 3000 5`", 5*time.Second)
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
			SeriesID: m.ChannelID + "_baucua", GameType: "baucua", ChannelID: m.ChannelID,
			OwnerID: m.Author.ID, GuildID: m.GuildID, TotalCount: count,
			Params: GameCreateParams{MinBet: minBet, MaxBet: maxBet, DurationSec: durationSec},
		}
		if !StartAutoSeries(series) {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Kênh này đang có chuỗi tự động Bầu Cua chạy rồi.", 5*time.Second)
			return
		}
		b.startBaucuaRound(s, m.ChannelID, m.Author.ID, m.GuildID, minBet, maxBet, durationSec, series)
	} else {
		b.startBaucuaRound(s, m.ChannelID, m.Author.ID, m.GuildID, minBet, maxBet, durationSec, nil)
	}
}

func (b *Bot) startBaucuaRound(s *discordgo.Session, channelID, hostID, guildID string, minBet, maxBet int64, durationSec int, series *AutoGameSeries) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, exists := baucuaRounds.Load(channelID); exists {
		sendTemp(s, channelID, ui.Emojis.Loading+" Đang có ván Bầu Cua. Chờ ván này kết thúc.", 4*time.Second)
		if series != nil {
			StopAutoSeries(channelID, "baucua")
		}
		return
	}

	hostBalance, err := b.wallet.GetBalance(ctx, hostID)
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi lấy số dư của bạn.", 5*time.Second)
		if series != nil {
			StopAutoSeries(channelID, "baucua")
		}
		return
	}
	debtLimit := b.GetEffectiveDebtLimit(ctx, s, guildID, hostID)
	if hostBalance.Balance+debtLimit < minBet {
		msg := fmt.Sprintf("%s Số dư / hạn mức tín dụng của bạn không đủ để làm Cái (cần ít nhất %s).", ui.Emojis.Take, formatCoins(minBet))
		if series != nil {
			msg += fmt.Sprintf("\n%s **Đã dừng chuỗi tự động tạo bàn (Đã tạo %d/%d bàn).**", ui.Emojis.Warning, series.CurrentIndex, series.TotalCount)
			StopAutoSeries(channelID, "baucua")
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

	round := &BaucuaRound{
		ChannelID:   channelID,
		HostID:      hostID,
		MinBet:      minBet,
		MaxBet:      maxBet,
		Bets:        []ui.BaucuaBet{},
		Betted:      make(map[string]map[string]bool),
		Deadline:    time.Now().Add(time.Duration(durationSec) * time.Second),
		Tip:         tip,
		LastRefresh: time.Now(),
	}
	baucuaRounds.Store(channelID, round)

	embed := ui.BuildBaucuaEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: ui.BaucuaButtons(s, false),
	})
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi tạo bàn.", 5*time.Second)
		baucuaRounds.Delete(channelID)
		if series != nil {
			StopAutoSeries(channelID, "baucua")
		}
		return
	}

	round.MessageID = msg.ID
	go b.runBaucuaCountdown(s, round)
}

func (b *Bot) runBaucuaCountdown(s *discordgo.Session, round *BaucuaRound) {
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
			b.resolveBaucua(s, round)
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
		embed := ui.BuildBaucuaEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
		embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
		round.mu.Unlock()

		if shouldRefresh && oldMsgID != "" {
			_ = s.ChannelMessageDelete(round.ChannelID, oldMsgID)
			msg, err := s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{Embed: embed, Components: ui.BaucuaButtons(s, false)})
			if err == nil {
				round.mu.Lock()
				round.MessageID = msg.ID
				round.mu.Unlock()
			}
		} else if needsUpdate && oldMsgID != "" {
			embeds := []*discordgo.MessageEmbed{embed}
			comps := ui.BaucuaButtons(s, false)
			s.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: oldMsgID, Channel: round.ChannelID, Embeds: &embeds, Components: &comps,
			})
		}
	}
}

func (b *Bot) handleBaucuaButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 2 {
		return
	}
	action := parts[1]
	userID := interactionUserID(i)

	if action == "continue" {
		hostID := parts[2]
		if userID != hostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn mới có thể tạo ván mới.")
			return
		}

		minBet, _ := strconv.ParseInt(parts[3], 10, 64)
		maxBet, _ := strconv.ParseInt(parts[4], 10, 64)

		if _, exists := baucuaRounds.Load(i.ChannelID); exists {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang có ván Bầu Cua khác trong kênh. Chờ ván đó kết thúc.")
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

		StopAutoSeries(i.ChannelID, "baucua")
		round := &BaucuaRound{ChannelID: i.ChannelID, HostID: hostID, MinBet: minBet, MaxBet: maxBet, Bets: []ui.BaucuaBet{}, Betted: make(map[string]map[string]bool), Deadline: time.Now().Add(30 * time.Second), Tip: GetRandomTip(b.GetPrefix(ctx)), LastRefresh: time.Now()}
		baucuaRounds.Store(i.ChannelID, round)

		embed := ui.BuildBaucuaEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
		embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
		msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
			Embed:      embed,
			Components: ui.BaucuaButtons(s, false),
		})
		if err == nil {
			round.MessageID = msg.ID
			go b.runBaucuaCountdown(s, round)
		}
		return
	}

	val, exists := baucuaRounds.Load(i.ChannelID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Không có ván Bầu Cua nào đang mở.")
		return
	}
	round := val.(*BaucuaRound)
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
		StopAutoSeries(round.ChannelID, "baucua")
		baucuaRounds.Delete(round.ChannelID)

		for _, bet := range round.Bets {
			b.wallet.BetSettle(ctx, bet.UserID, bet.Amount, true)   // Hoàn tiền người chơi
			b.wallet.BetSettle(ctx, round.HostID, bet.Escrow, true) // Hoàn escrow của Host
		}
		embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Bầu Cua đã bị hủy bởi <@" + userID + ">. Tiền cược đã được hoàn lại.")}
		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: round.MessageID, Channel: round.ChannelID, Embeds: &embeds, Components: &comps,
		})
		deleteAfter(s, round.ChannelID, round.MessageID, 3*time.Second)
		return
	}

	if action == "bau" || action == "cua" || action == "tom" || action == "ca" || action == "nai" || action == "ga" {
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

		if round.Betted[userID] != nil && round.Betted[userID][action] {
			round.mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn đã cược ô này rồi, hãy chọn ô khác.")
			return
		}

		min := round.MinBet
		max := round.MaxBet
		round.mu.Unlock()

		sideName := ""
		switch action {
		case "bau":
			sideName = "Bầu"
		case "cua":
			sideName = "Cua"
		case "tom":
			sideName = "Tôm"
		case "ca":
			sideName = "Cá"
		case "nai":
			sideName = "Nai"
		case "ga":
			sideName = "Gà"
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "bc:modal:" + action,
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

func (b *Bot) handleBaucuaModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	parts := strings.Split(data.CustomID, ":")
	if len(parts) < 3 {
		return
	}
	side := parts[2]
	userID := interactionUserID(i)

	val, exists := baucuaRounds.Load(i.ChannelID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Không có ván Bầu Cua nào đang mở.")
		return
	}
	round := val.(*BaucuaRound)

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

	if round.Betted[userID] == nil {
		round.Betted[userID] = make(map[string]bool)
	}
	if round.Betted[userID][side] {
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Warning+" Bạn đã cược ô này rồi.")
		return
	}

	if amount < round.MinBet || amount > round.MaxBet {
		round.mu.Unlock()
		followupEphemeral(s, i, fmt.Sprintf("%s Số tiền cược phải từ **%d** đến **%d**.", ui.Emojis.Warning, round.MinBet, round.MaxBet))
		return
	}

	round.Betted[userID][side] = true
	round.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := b.casinoConfigSvc.GetConfig("baucua")
	bcCfg := models.DefaultBaucuaConfig
	if cfg.BaucuaCfg != nil {
		bcCfg = *cfg.BaucuaCfg
	}
	maxMultiplier := bcCfg.PayoutPerMatch * 3
	escrowAmount := int64(math.Round(float64(amount) * maxMultiplier))

	if err := b.SafeBetDeduct(ctx, s, i.GuildID, userID, amount); err != nil {
		round.mu.Lock()
		delete(round.Betted[userID], side)
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Take+" Số dư / hạn mức của bạn không đủ.")
		return
	}
	// Escrow Host
	if err := b.SafeBetDeduct(ctx, s, i.GuildID, round.HostID, escrowAmount); err != nil {
		b.wallet.BetSettle(ctx, userID, amount, true) // Refund
		round.mu.Lock()
		delete(round.Betted[userID], side)
		round.mu.Unlock()
		followupEphemeral(s, i, ui.Emojis.Take+" Nhà cái đã hết vốn tạm thời để nhận thêm cược này.")
		return
	}

	round.mu.Lock()
	if round.Closed {
		round.mu.Unlock()
		b.wallet.BetSettle(ctx, userID, amount, true)
		b.wallet.BetSettle(ctx, round.HostID, escrowAmount, true)
		followupEphemeral(s, i, ui.Emojis.Loading+" Ván đã chốt sổ trong lúc bạn nạp tiền, đã hoàn lại!")
		return
	}

	round.Bets = append(round.Bets, ui.BaucuaBet{UserID: userID, Side: side, Amount: amount, Escrow: escrowAmount, Payout: bcCfg.PayoutPerMatch})
	round.NeedsUpdate = true
	round.mu.Unlock()

	followupEphemeral(s, i, ui.Emojis.Success+" Đặt cược thành công!")
}

func (b *Bot) resolveBaucua(s *discordgo.Session, round *BaucuaRound) {
	round.mu.Lock()
	if round.Closed {
		round.mu.Unlock()
		return
	}
	round.Closed = true
	round.mu.Unlock()
	baucuaRounds.Delete(round.ChannelID)

	// Xóa tin nhắn bảng cược chính để gọn chat
	_ = s.ChannelMessageDelete(round.ChannelID, round.MessageID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	isAdmin, _ := b.userRepo.IsAdmin(ctx, round.HostID)
	isOwner, _ := b.userRepo.IsOwner(ctx, round.HostID)
	isPrivileged := isAdmin || isOwner

	cfg := b.casinoConfigSvc.GetConfig("baucua")
	bcCfg := models.DefaultBaucuaConfig
	if cfg.BaucuaCfg != nil {
		bcCfg = *cfg.BaucuaCfg
	}
	rigProb := getRigProb(cfg.RigMode)

	result := games.PlayBaucua()

	if isPrivileged && len(round.Bets) > 0 && rigProb > 0 && games.RandInt(100) < int64(rigProb) {
		bestResult := result
		var bestHostNet int64

		for _, bet := range round.Bets {
			matches := bestResult.MatchCount(bet.Side)
			if matches > 0 {
				profit := int64(math.Round(float64(bet.Amount) * float64(matches) * bcCfg.PayoutPerMatch))
				bestHostNet -= profit
			} else {
				bestHostNet += bet.Amount
			}
		}

		for i := 0; i < 20; i++ {
			tempResult := games.PlayBaucua()

			if tempResult.Face1 == tempResult.Face2 && tempResult.Face2 == tempResult.Face3 {
				continue
			}

			var tempHostNet int64
			for _, bet := range round.Bets {
				matches := tempResult.MatchCount(bet.Side)
				if matches > 0 {
					profit := int64(math.Round(float64(bet.Amount) * float64(matches) * bcCfg.PayoutPerMatch))
					tempHostNet -= profit
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

	animEmbed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s NHÀ CÁI ĐANG MỞ BÁT...", ui.Emojis.Dice),
		Color: ui.Theme.Colors.Game,
	}
	animContent := fmt.Sprintf("%s %s %s %s %s", ui.Emojis.AnimalRolling, ui.Emojis.SpaceAnimation, ui.Emojis.AnimalRolling, ui.Emojis.SpaceAnimation, ui.Emojis.AnimalRolling)
	animMsg, err := s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{
		Content: animContent,
		Embed:   animEmbed,
	})

	if err == nil {
		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.GetBaucuaEmoji(string(result.Face1)), ui.Emojis.SpaceAnimation, ui.Emojis.AnimalRolling, ui.Emojis.SpaceAnimation, ui.Emojis.AnimalRolling)
		embeds1 := []*discordgo.MessageEmbed{animEmbed}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &embeds1,
		})

		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.GetBaucuaEmoji(string(result.Face1)), ui.Emojis.SpaceAnimation, ui.GetBaucuaEmoji(string(result.Face2)), ui.Emojis.SpaceAnimation, ui.Emojis.AnimalRolling)
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &embeds1,
		})

		time.Sleep(2 * time.Second)
		animContent = fmt.Sprintf("%s %s %s %s %s", ui.GetBaucuaEmoji(string(result.Face1)), ui.Emojis.SpaceAnimation, ui.GetBaucuaEmoji(string(result.Face2)), ui.Emojis.SpaceAnimation, ui.GetBaucuaEmoji(string(result.Face3)))
		emptyEmbeds := []*discordgo.MessageEmbed{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: round.ChannelID, Content: &animContent, Embeds: &emptyEmbeds,
		})
		time.Sleep(1 * time.Second)
	}

	var hostNet int64
	var highestWin int64
	var bestPlayer string

	for _, bet := range round.Bets {
		matches := result.MatchCount(bet.Side)
		escrow := bet.Escrow
		profitNet := int64(math.Round(float64(bet.Amount) * float64(matches) * bet.Payout))

		if matches > 0 {
			// Thắng: Trả lại tiền cược + tiền ăn
			b.wallet.BetSettle(ctx, bet.UserID, bet.Amount+profitNet, true)

			// Host nhận lại phần escrow còn dư
			hostRefund := escrow - profitNet
			b.wallet.BetSettle(ctx, round.HostID, hostRefund, true)

			hostNet -= profitNet

			winAmt := profitNet
			if winAmt > highestWin {
				highestWin = winAmt
				bestPlayer = bet.UserID
			}
		} else {
			// Thua: Host thu tiền cược của khách + nhận lại toàn bộ escrow
			b.wallet.BetSettle(ctx, round.HostID, escrow+bet.Amount, true)
			hostNet += bet.Amount
		}

		profit := float64(-bet.Amount) / 1000000.0
		if matches > 0 {
			profit = float64(profitNet) / 1000000.0
		}
		_ = b.userRepo.UpdateGameHistory(ctx, bet.UserID, "baucua", profit)
	}

	_ = b.userRepo.UpdateGameHistory(ctx, round.HostID, "baucua", float64(hostNet)/1000000.0)

	npcDialog := ""
	if hostNet > highestWin*2 && hostNet > 0 {
		dialogue.GetManager().MarkActivity(round.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.DealerDominating, map[string]string{"game": "Bầu Cua", "channel": round.ChannelID})
	} else if highestWin > 0 {
		dialogue.GetManager().MarkActivity(round.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.WinBig, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Bầu Cua", "channel": round.ChannelID})
	}

	embed := ui.BuildBaucuaResultEmbed(round.HostID, result, round.Bets, hostNet, bcCfg.PayoutPerMatch)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Ván kết thúc! | " + round.Tip}

	// CACHE KẾT QUẢ ĐỂ KHÔI PHỤC KHI BỊ RATELIMIT
	CacheGameResult(round.ChannelID, "Bầu Cua", npcDialog, embed, ui.BaucuaResultButtons(round.HostID, round.MinBet, round.MaxBet, s))

	s.ChannelMessageSendComplex(round.ChannelID, &discordgo.MessageSend{
		Content:    npcDialog,
		Embed:      embed,
		Components: ui.BaucuaResultButtons(round.HostID, round.MinBet, round.MaxBet, s),
	})

	// Xử lý tạo ván tiếp theo nếu có chuỗi Auto
	series := GetAutoSeries(round.ChannelID, "baucua")
	if series != nil && series.Active && series.OwnerID == round.HostID {
		if series.CurrentIndex < series.TotalCount {
			go func() {
				time.Sleep(6 * time.Second)
				time.Sleep(3 * time.Second) //Thời gian giữa những lần tự động tạo
				if GetAutoSeries(round.ChannelID, "baucua") != nil {
					b.startBaucuaRound(s, round.ChannelID, round.HostID, series.GuildID, series.Params.MinBet, series.Params.MaxBet, series.Params.DurationSec, series)
				}
			}()
		} else {
			StopAutoSeries(round.ChannelID, "baucua")
			s.ChannelMessageSend(round.ChannelID, fmt.Sprintf("%s <@%s> Đã hoàn thành chuỗi tự động tạo bàn Bầu Cua.", ui.Emojis.Success, round.HostID))
		}
	}
}

// Repost helper hỗ trợ lệnh !host
func (b *Bot) repostBaucuaTable(s *discordgo.Session, channelID, userID string) bool {
	val, exists := baucuaRounds.Load(channelID)
	if !exists {
		return false
	}
	round := val.(*BaucuaRound)

	round.mu.Lock()
	defer round.mu.Unlock()
	if round.HostID != userID || round.Closed {
		return false
	}

	oldMsgID := round.MessageID
	embed := ui.BuildBaucuaEmbed(round.HostID, round.MinBet, round.MaxBet, round.Deadline.Unix(), round.Bets)
	embed.Footer = &discordgo.MessageEmbedFooter{Text: round.Tip}
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{Embed: embed, Components: ui.BaucuaButtons(s, false)})
	if err == nil {
		round.MessageID = msg.ID
		if oldMsgID != "" {
			_ = s.ChannelMessageDelete(channelID, oldMsgID)
		}
		return true
	}
	return false
}
