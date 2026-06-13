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

type BaicaoPlayer struct {
	Hand      []games.Card
	Status    string // "playing", "folded", "showed"
	HasBuffed bool
}

type BaicaoTable struct {
	Mu           sync.Mutex
	TableID      string
	ChannelID    string
	MessageID    string
	HostID       string
	BetAmount    int64
	EscrowAmount int64
	PayoutNormal float64
	PlayerIDs    []string
	Players      map[string]*BaicaoPlayer
	Status       string // "waiting", "dealing", "playing", "finished", "canceled"
	Deck         []games.Card
	Deadline     time.Time
	ControlID    string
}

var baicaoTables sync.Map

func (b *Bot) cmdBaicao(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, _ := b.configRepo.IsChannelWhitelisted(ctx, m.ChannelID)
	if !ok {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Lệnh casino không được phép ở kênh này.", 3*time.Second)
		return
	}

	if len(args) > 0 && strings.ToLower(args[0]) == "stop" {
		if series := GetAutoSeries(m.ChannelID, "baicao"); series != nil {
			if series.OwnerID == m.Author.ID || b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member) >= RoleAdmin {
				StopAutoSeries(m.ChannelID, "baicao")
				sendTemp(s, m.ChannelID, ui.Emojis.Success+" Đã dừng chuỗi tự động tạo bàn Bài Cào.", 5*time.Second)
			} else {
				sendTemp(s, m.ChannelID, ui.Emojis.Error+" Bạn không có quyền dừng chuỗi này.", 5*time.Second)
			}
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có chuỗi tự động nào đang chạy.", 5*time.Second)
		}
		return
	}

	var betAmount int64
	var count int = 1
	var err error

	parsedArgs := []string{}
	if len(args) > 0 && strings.HasSuffix(strings.ToLower(args[0]), "s") {
		parsedArgs = args[1:]
	} else if len(args) > 0 {
		parsedArgs = args
	}

	if len(parsedArgs) >= 1 {
		betAmount, err = strconv.ParseInt(parsedArgs[0], 10, 64)
	} else {
		err = fmt.Errorf("invalid")
	}

	if len(parsedArgs) >= 2 {
		count, err = strconv.Atoi(parsedArgs[1])
	}

	if err != nil || betAmount <= 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Cú pháp không hợp lệ. Ví dụ: `"+b.GetPrefix(ctx)+"bcao 1000` hoặc `"+b.GetPrefix(ctx)+"bcao 1000 5`", 5*time.Second)
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
			SeriesID: m.ChannelID + "_baicao", GameType: "baicao", ChannelID: m.ChannelID,
			OwnerID: m.Author.ID, GuildID: m.GuildID, TotalCount: count,
			Params: GameCreateParams{BetAmount: betAmount},
		}
		if !StartAutoSeries(series) {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Kênh này đang có chuỗi tự động Bài Cào chạy rồi.", 5*time.Second)
			return
		}
		b.startBaicaoRound(s, m.ChannelID, m.Author.ID, m.GuildID, betAmount, series)
	} else {
		b.startBaicaoRound(s, m.ChannelID, m.Author.ID, m.GuildID, betAmount, nil)
	}
}

func (b *Bot) startBaicaoRound(s *discordgo.Session, channelID, hostID, guildID string, betAmount int64, series *AutoGameSeries) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := b.casinoConfigSvc.GetConfig("baicao")
	bcCfg := models.DefaultBaicaoConfig
	if cfg.BaicaoCfg != nil {
		bcCfg = *cfg.BaicaoCfg
	}
	reqAmount := int64(math.Round(float64(betAmount) * bcCfg.PayoutSap))

	if err := b.SafeBetDeduct(ctx, s, guildID, hostID, reqAmount); err != nil {
		msg := fmt.Sprintf("%s Bạn cần ít nhất %s (x%v tiền cược) để tiếp tục làm cái phòng đền Sáp.", ui.Emojis.Take, coinDisplay(reqAmount), bcCfg.PayoutSap)
		if series != nil {
			msg += fmt.Sprintf("\n%s **Đã dừng chuỗi tự động tạo bàn (Đã tạo %d/%d bàn).**", ui.Emojis.Warning, series.CurrentIndex, series.TotalCount)
			StopAutoSeries(channelID, "baicao")
			s.ChannelMessageSend(channelID, msg)
		} else {
			sendTemp(s, channelID, msg, 5*time.Second)
		}
		return
	}

	deck := games.BuildDeck()
	games.ShuffleDeck(deck)

	tableIDSuffix := "Auto"
	if series != nil {
		series.CurrentIndex++
		tableIDSuffix = fmt.Sprintf("Auto-%d", series.CurrentIndex)
	}

	t := &BaicaoTable{
		TableID:      fmt.Sprintf("BC-%s-%d", tableIDSuffix, time.Now().UnixNano()),
		ChannelID:    channelID,
		HostID:       hostID,
		BetAmount:    betAmount,
		EscrowAmount: reqAmount,
		PayoutNormal: bcCfg.PayoutNormal,
		PlayerIDs:    []string{hostID},
		Players:      make(map[string]*BaicaoPlayer),
		Status:       "waiting",
		Deck:         deck,
	}
	t.Players[hostID] = &BaicaoPlayer{Status: "playing"}
	baicaoTables.Store(t.TableID, t)

	embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, nil, nil, "waiting", 0)
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: ui.BaicaoWaitingButtons(t.TableID, s),
	})
	if err != nil {
		baicaoTables.Delete(t.TableID)
		b.wallet.BetSettle(ctx, hostID, reqAmount, true)
		if series != nil {
			StopAutoSeries(channelID, "baicao")
		}
	} else {
		t.MessageID = msg.ID
		go func(table *BaicaoTable) {
			time.Sleep(3 * time.Minute)
			table.Mu.Lock()
			if table.Status == "waiting" {
				table.Status = "canceled"
				table.Mu.Unlock()
				baicaoTables.Delete(table.TableID)
				b.wallet.BetSettle(context.Background(), table.HostID, table.BetAmount*2, true)
				embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Bài Cào đã tự động hủy do hết thời gian chờ.")}
				comps := []discordgo.MessageComponent{}
				s.ChannelMessageEditComplex(&discordgo.MessageEdit{
					ID: table.MessageID, Channel: table.ChannelID, Embeds: &embeds, Components: &comps,
				})
			} else {
				table.Mu.Unlock()
			}
		}(t)
	}
}

func (b *Bot) handleBaicaoButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 3 {
		return
	}
	action := parts[1]
	tableID := parts[2]
	userID := interactionUserID(i)

	if action == "continue" {
		hostID := parts[2]
		if userID != hostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn mới có thể tạo ván mới.")
			return
		}

		betAmount, _ := strconv.ParseInt(parts[3], 10, 64)

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cfg := b.casinoConfigSvc.GetConfig("baicao")
		bcCfg := models.DefaultBaicaoConfig
		if cfg.BaicaoCfg != nil {
			bcCfg = *cfg.BaicaoCfg
		}
		reqAmount := int64(math.Round(float64(betAmount) * bcCfg.PayoutSap))

		if err := b.SafeBetDeduct(ctx, s, i.GuildID, hostID, reqAmount); err != nil {
			followupEphemeral(s, i, ui.Emojis.Take+" Số dư của bạn không đủ "+coinDisplay(reqAmount)+" để tiếp tục làm cái.")
			return
		}

		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Components: &comps})

		StopAutoSeries(i.ChannelID, "baicao")
		deck := games.BuildDeck()
		games.ShuffleDeck(deck)
		newT := &BaicaoTable{TableID: fmt.Sprintf("BC-%d-%s", time.Now().UnixNano(), hostID), ChannelID: i.ChannelID, HostID: hostID, BetAmount: betAmount, EscrowAmount: reqAmount, PayoutNormal: bcCfg.PayoutNormal, PlayerIDs: []string{hostID}, Players: make(map[string]*BaicaoPlayer), Status: "waiting", Deck: deck}
		newT.Players[hostID] = &BaicaoPlayer{Status: "playing"}
		baicaoTables.Store(newT.TableID, newT)

		embed := ui.BuildBaicaoEmbed(newT.TableID, newT.HostID, newT.BetAmount, 0, newT.PlayerIDs, nil, nil, "waiting", 0)
		msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{Embed: embed, Components: ui.BaicaoWaitingButtons(newT.TableID, s)})
		if err != nil {
			baicaoTables.Delete(newT.TableID)
			b.wallet.BetSettle(ctx, hostID, reqAmount, true)
		} else {
			newT.MessageID = msg.ID
		}
		return
	}

	val, exists := baicaoTables.Load(tableID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Bàn Bài Cào đã kết thúc hoặc không tồn tại.")
		return
	}
	t := val.(*BaicaoTable)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch action {
	case "join":
		lockKey := fmt.Sprintf("join:%s:%s", t.TableID, userID)
		if !b.TryLockAction(lockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý, vui lòng không bấm spam.")
			return
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		t.Mu.Lock()
		if t.Status != "waiting" {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Loading+" Bàn đã đóng/bắt đầu.")
			return
		}
		if len(t.PlayerIDs) >= 6 {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Block+" Bàn đã đầy (Tối đa 6 người).")
			return
		}
		for _, uid := range t.PlayerIDs {
			if uid == userID {
				t.Mu.Unlock()
				followupEphemeral(s, i, ui.Emojis.Warning+" Bạn đã tham gia bàn này rồi.")
				return
			}
		}
		t.Mu.Unlock()

		reqAmount := t.EscrowAmount

		if err := b.SafeBetDeduct(ctx, s, i.GuildID, userID, reqAmount); err != nil {
			followupEphemeral(s, i, fmt.Sprintf("%s Số dư của bạn không đủ %s tiền cược để tham gia phòng đền Sáp.", ui.Emojis.Take, coinDisplay(reqAmount)))
			return
		}

		t.Mu.Lock()
		if t.Status != "waiting" { // Chống double click lọt lúc host ấn Bắt đầu
			t.Mu.Unlock()
			b.wallet.BetSettle(ctx, userID, reqAmount, true)
			followupEphemeral(s, i, ui.Emojis.Warning+" Bàn đã bắt đầu, đã hoàn lại tiền!")
			return
		}
		t.PlayerIDs = append(t.PlayerIDs, userID)
		t.Players[userID] = &BaicaoPlayer{Status: "playing"}
		embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, nil, nil, "waiting", 0)
		t.Mu.Unlock()

		embeds := []*discordgo.MessageEmbed{embed}
		comps := ui.BaicaoWaitingButtons(t.TableID, s)
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &comps,
		})

	case "start":
		if userID != t.HostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn mới có quyền Chia Bài.")
			return
		}
		t.Mu.Lock()
		if len(t.PlayerIDs) < 2 {
			t.Mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Phải có ít nhất 2 người để chơi.")
			return
		}
		if t.Status != "waiting" {
			t.Mu.Unlock()
			return
		}
		t.Status = "dealing" // Khóa bàn
		embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, nil, nil, "dealing", 0)
		t.Mu.Unlock()
		updateEmbed(s, i, embed, []discordgo.MessageComponent{}) // Xóa nút

		go b.runBaicaoDealing(s, t)

	case "view":
		t.Mu.Lock()
		p, exists := t.Players[userID]
		t.Mu.Unlock()
		if !exists {
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn không tham gia bàn này.")
			return
		}
		if len(p.Hand) == 0 {
			replyEphemeral(s, i, ui.Emojis.Loading+" Bài chưa được chia.")
			return
		}
		score := (p.Hand[0].Value() + p.Hand[1].Value()) % 10
		scoreStr := fmt.Sprintf("%d Điểm", score)
		if score == 0 {
			scoreStr = "Bù (0 Điểm)"
		}
		firstTwo := fmt.Sprintf("%s %s %s", ui.CardString(p.Hand[0]), ui.CardString(p.Hand[1]), ui.Emojis.BaicaoHiddenCard)
		replyEphemeral(s, i, fmt.Sprintf("%s  Bài của bạn:\n# %s\n> Tổng 2 lá đầu: **%s**", ui.Emojis.BaicaoViewTitle, firstTwo, scoreStr))

	case "reveal":
		lockKey := fmt.Sprintf("reveal:%s:%s", t.TableID, userID)
		if !b.TryLockAction(lockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý, vui lòng không bấm spam.")
			return
		}

		// Kiểm tra quyền Admin/Owner ngầm trước khi Lock để tránh giật lag server
		isPrivileged := false
		if userID == t.HostID {
			ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 2*time.Second)
			isAdmin, _ := b.userRepo.IsAdmin(ctxCheck, userID)
			isOwner, _ := b.userRepo.IsOwner(ctxCheck, userID)
			cancelCheck()
			isPrivileged = isAdmin || isOwner
		}

		cfg := b.casinoConfigSvc.GetConfig("baicao")
		bcCfg := models.DefaultBaicaoConfig
		if cfg.BaicaoCfg != nil {
			bcCfg = *cfg.BaicaoCfg
		}

		t.Mu.Lock()
		p, exists := t.Players[userID]
		if !exists {
			t.Mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn không tham gia bàn này.")
			return
		}
		if p.Status != "playing" {
			t.Mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn đã Show hoặc Bỏ bài rồi.")
			return
		}

		if userID == t.HostID && isPrivileged && bcCfg.HostPeekEnabled && !p.HasBuffed && len(t.Deck) >= 3 {
			p.HasBuffed = true

			var maxOppRank games.BaicaoRank
			maxOppRank.Type = -1
			for uid, opp := range t.Players {
				if uid != t.HostID && opp.Status != "folded" {
					r := games.GetBaicaoRank(opp.Hand)
					if r.Type > maxOppRank.Type || (r.Type == maxOppRank.Type && r.Value > maxOppRank.Value) {
						maxOppRank = r
					}
				}
			}

			hostRank := games.GetBaicaoRank(p.Hand)
			// Chỉ kích hoạt nếu Host đang thua hoặc hòa người có rank cao nhất
			if maxOppRank.Type > hostRank.Type || (maxOppRank.Type == hostRank.Type && maxOppRank.Value >= hostRank.Value) {
				tempDeck := make([]games.Card, len(t.Deck))
				copy(tempDeck, t.Deck)

				// Thử xào nọc 50 lần để tìm ra bài lớn hơn maxOppRank (Chống Infinite Loop nếu khách có Sáp quá cao)
				for attempt := 0; attempt < 50; attempt++ {
					games.ShuffleDeck(tempDeck)
					testHand := tempDeck[:3]
					testRank := games.GetBaicaoRank(testHand)

					if testRank.Type > maxOppRank.Type || (testRank.Type == maxOppRank.Type && testRank.Value > maxOppRank.Value) {
						oldHand := p.Hand
						p.Hand = make([]games.Card, 3)
						copy(p.Hand, testHand)
						t.Deck = append(tempDeck[3:], oldHand...) // Trả bài cũ của Host lại vào cuối Nọc
						break
					}
				}
			}
		}
		// -----------------------------------------------------------

		t.Mu.Unlock()

		firstTwo := fmt.Sprintf("%s %s", ui.CardString(p.Hand[0]), ui.CardString(p.Hand[1]))
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("%s  Bài của bạn:\n# %s %s\n> *Đang nặn lá cuối...*", ui.Emojis.BaicaoViewTitle, firstTwo, ui.Emojis.BaicaoBlink),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		go func(interaction *discordgo.Interaction, table *BaicaoTable, uID string) {
			time.Sleep(3 * time.Second)

			table.Mu.Lock()
			player, ok := table.Players[uID]
			if !ok || table.Status == "canceled" {
				table.Mu.Unlock()
				return
			}

			rank := games.GetBaicaoRank(player.Hand)
			content := fmt.Sprintf("%s  Bài của bạn:\n# %s\n> Kết quả: **%s**\n\n*Nhấn Show Bài hoặc Bỏ Bài ở kênh chat chính để chốt lượt!*", ui.Emojis.BaicaoViewTitle, ui.HandString(player.Hand), rank.Label)
			table.Mu.Unlock()

			_, _ = s.InteractionResponseEdit(interaction, &discordgo.WebhookEdit{
				Content: &content,
			})
		}(i.Interaction, t, userID)

	case "show", "fold":
		lockKey := fmt.Sprintf("%s:%s:%s", action, t.TableID, userID)
		if !b.TryLockAction(lockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý, vui lòng không bấm spam.")
			return
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
		t.Mu.Lock()
		p, exists := t.Players[userID]
		if !exists {
			t.Mu.Unlock()
			return
		}
		if p.Status != "playing" {
			t.Mu.Unlock()
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: ui.Emojis.Warning + " Bạn đã Show hoặc Bỏ bài rồi.", Flags: discordgo.MessageFlagsEphemeral})
			return
		}
		if action == "show" {
			p.Status = "showed"
		} else {
			p.Status = "folded"
		}

		allDone := true
		playerStatus := make(map[string]string)
		hands := make(map[string][]games.Card)
		for uid, player := range t.Players {
			playerStatus[uid] = player.Status
			hands[uid] = player.Hand
			if player.Status == "playing" {
				allDone = false
			}
		}
		embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, playerStatus, hands, "playing", t.Deadline.Unix())
		oldMsgID := t.MessageID
		var handToShow []games.Card
		if action == "show" {
			handToShow = p.Hand
		}
		t.Mu.Unlock()

		if action == "show" {
			rank := games.GetBaicaoRank(handToShow)
			showEmbed := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("# %s\n> Kết quả: **%s**", ui.HandString(handToShow), rank.Label),
				Color:       0xFF3333,
			}
			_, _ = s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
				Content: fmt.Sprintf("%s <@%s> vừa show bài:", ui.Emojis.BaicaoViewTitle, userID),
				Embed:   showEmbed,
			})
		}

		if !allDone {
			if oldMsgID != "" {
				// Chỉ edit lại tin nhắn bảng chính, KHÔNG gửi lại Nút bấm để tránh spam.
				// Nút bấm sẽ được ticker tự động kéo xuống mỗi 10 giây.
				_, _ = s.ChannelMessageEditEmbed(t.ChannelID, oldMsgID, embed)
			}
		} else {
			b.resolveBaicao(s, t)
		}

	case "cancel":
		if userID != t.HostID {
			if !HasAdminAccess(b, ctx, s, i) {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn hoặc Admin mới có quyền hủy.")
				return
			}
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
		t.Mu.Lock()
		if t.Status != "waiting" {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Warning+" Bàn đang xử lý, không thể hủy.")
			return
		}
		t.Status = "canceled"
		ctrlID := t.ControlID
		t.Mu.Unlock()
		baicaoTables.Delete(t.TableID)
		StopAutoSeries(t.ChannelID, "baicao")

		escrowAmount := t.EscrowAmount

		// Refund 2x escrow cho mọi người
		for _, uid := range t.PlayerIDs {
			b.wallet.BetSettle(ctx, uid, escrowAmount, true)
		}

		embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Bài Cào đã bị hủy. Mọi người đã được hoàn lại tiền.")}
		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &comps,
		})
		if ctrlID != "" {
			s.ChannelMessageDelete(t.ChannelID, ctrlID)
		}
		deleteAfter(s, i.ChannelID, i.Message.ID, 3*time.Second)
	}
}

func (b *Bot) runBaicaoDealing(s *discordgo.Session, t *BaicaoTable) {
	time.Sleep(3 * time.Second) // Tạo animation úp bài hồi hộp

	t.Mu.Lock()
	if t.Status == "canceled" {
		t.Mu.Unlock()
		return
	}
	t.Status = "playing"
	t.Deadline = time.Now().Add(45 * time.Second) // Thời gian nặn bài là 45s

	for _, uid := range t.PlayerIDs {
		t.Players[uid].Hand = t.Deck[:3]
		t.Deck = t.Deck[3:]
	}

	// ── ĐỒNG BỘ RIG MODE TỪ BẢNG CONFIG ──
	ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 2*time.Second)
	isAdmin, _ := b.userRepo.IsAdmin(ctxCheck, t.HostID)
	isOwner, _ := b.userRepo.IsOwner(ctxCheck, t.HostID)
	cancelCheck()
	isPrivileged := isAdmin || isOwner

	cfg := b.casinoConfigSvc.GetConfig("baicao")
	rigProb := getRigProb(cfg.RigMode)

	if isPrivileged && len(t.PlayerIDs) > 1 && rigProb > 0 && games.RandInt(100) < int64(rigProb) {
		bestUid := t.HostID
		var maxRank games.BaicaoRank
		maxRank.Type = -1

		for _, uid := range t.PlayerIDs {
			rank := games.GetBaicaoRank(t.Players[uid].Hand)
			if rank.Type > maxRank.Type || (rank.Type == maxRank.Type && rank.Value > maxRank.Value) {
				maxRank = rank
				bestUid = uid
			}
		}

		if bestUid != t.HostID {
			t.Players[t.HostID].Hand, t.Players[bestUid].Hand = t.Players[bestUid].Hand, t.Players[t.HostID].Hand
		}
	}
	// ─────────────────────────────────────

	playerStatus := make(map[string]string)
	hands := make(map[string][]games.Card)
	for uid, player := range t.Players {
		playerStatus[uid] = player.Status
		hands[uid] = player.Hand
	}
	embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, playerStatus, hands, "playing", t.Deadline.Unix())
	oldMsgID := t.MessageID
	t.Mu.Unlock()

	if oldMsgID != "" {
		_ = s.ChannelMessageDelete(t.ChannelID, oldMsgID)
	}

	msg, err := s.ChannelMessageSendEmbed(t.ChannelID, embed)
	if err == nil {
		t.Mu.Lock()
		t.MessageID = msg.ID
		t.Mu.Unlock()
	}

	ctrlMsg, err := s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
		Content:    "\u200B",
		Components: ui.BaicaoPlayingButtons(t.TableID, s),
	})
	if err == nil {
		t.Mu.Lock()
		t.ControlID = ctrlMsg.ID
		t.Mu.Unlock()
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	lastResend := time.Now()

	for range ticker.C {
		t.Mu.Lock()
		if t.Status != "playing" {
			t.Mu.Unlock()
			return
		}
		if time.Now().After(t.Deadline) {
			t.Mu.Unlock()
			b.resolveBaicao(s, t)
			return
		}

		var oldCtrlID string
		var shouldResend bool
		if time.Since(lastResend) >= GameMessageRefreshInterval {
			oldCtrlID = t.ControlID
			shouldResend = true
			lastResend = time.Now()
		}
		t.Mu.Unlock()

		if shouldResend {
			if oldCtrlID != "" {
				s.ChannelMessageDelete(t.ChannelID, oldCtrlID)
			}
			newCtrlMsg, err := s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
				Content:    "\u200B",
				Components: ui.BaicaoPlayingButtons(t.TableID, s),
			})
			if err == nil {
				t.Mu.Lock()
				t.ControlID = newCtrlMsg.ID
				t.Mu.Unlock()
			}
		}
	}
}

func (b *Bot) resolveBaicao(s *discordgo.Session, t *BaicaoTable) {
	t.Mu.Lock()
	if t.Status == "finished" || t.Status == "canceled" {
		t.Mu.Unlock()
		return
	}
	t.Status = "finished"
	ctrlID := t.ControlID
	oldMsgID := t.MessageID
	t.Mu.Unlock()

	baicaoTables.Delete(t.TableID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if ctrlID != "" {
		s.ChannelMessageDelete(t.ChannelID, ctrlID)
	}

	// 0. Auto-Fold: Nếu timeout mà người chơi chưa show/bỏ bài thì tự động ép về trạng thái "Bỏ bài"
	for _, p := range t.Players {
		if p.Status == "playing" {
			p.Status = "folded"
		}
	}

	// Cập nhật lại khung tin nhắn đang chơi thành "Đã Kết Thúc" và show toàn bộ bài
	if oldMsgID != "" {
		playerStatus := make(map[string]string)
		hands := make(map[string][]games.Card)
		for uid, p := range t.Players {
			playerStatus[uid] = p.Status
			hands[uid] = p.Hand
		}
		finalEmbed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, playerStatus, hands, "finished", -1)
		s.ChannelMessageEditEmbed(t.ChannelID, oldMsgID, finalEmbed)
	}

	// 1. Chia bài và tìm hạng cao nhất
	var maxRank games.BaicaoRank
	maxRank.Type = -1
	validCount := 0

	for _, uid := range t.PlayerIDs {
		if t.Players[uid].Status != "folded" {
			validCount++
			rank := games.GetBaicaoRank(t.Players[uid].Hand)
			if rank.Type > maxRank.Type || (rank.Type == maxRank.Type && rank.Value > maxRank.Value) {
				maxRank = rank
			}
		}
	}

	escrowAmount := t.EscrowAmount

	if validCount == 0 { // Tất cả đều bỏ bài
		for _, uid := range t.PlayerIDs {
			b.wallet.BetSettle(ctx, uid, escrowAmount, true)
		}
		s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
			Embed:      errorEmbed("Tất cả người chơi đều bỏ bài. Đã hoàn lại toàn bộ tiền cược."),
			Components: ui.BaicaoResultButtons(t.HostID, t.BetAmount, s),
		})
		return
	}

	// 2. Lọc danh sách những người Win (Nếu bằng hạng nhau -> Split Pot)
	winners := []string{}
	results := make(map[string]ui.BaicaoPlayerResult)
	for _, uid := range t.PlayerIDs {
		if t.Players[uid].Status != "folded" {
			rank := games.GetBaicaoRank(t.Players[uid].Hand)
			if rank.Type == maxRank.Type && rank.Value == maxRank.Value {
				winners = append(winners, uid)
			}
		}
	}

	// 3. Xử lý kinh tế
	isSapWin := maxRank.Type == games.HandTypeSap
	loserPenalty := int64(math.Round(float64(t.BetAmount) * t.PayoutNormal))
	if isSapWin {
		loserPenalty = escrowAmount
	}

	// Quỹ tiền từ người thua
	totalPot := int64(len(t.PlayerIDs)-len(winners)) * loserPenalty
	winShare := totalPot / int64(len(winners)) // Chia đều nếu có nhiều người nhất
	remainder := totalPot % int64(len(winners))

	var highestWin int64
	var bestPlayer string

	for _, uid := range t.PlayerIDs {
		isWin := false
		for _, w := range winners {
			if w == uid {
				isWin = true
				break
			}
		}

		p := t.Players[uid]
		rank := games.GetBaicaoRank(p.Hand)
		if isWin {
			share := winShare
			if remainder > 0 {
				share++
				remainder--
			}

			// Thắng: Trả lại 2x tiền Ký quỹ + Tiền ăn chia
			b.wallet.BetSettle(ctx, uid, escrowAmount+share, true)

			bal, _ := b.wallet.GetBalance(ctx, uid)
			finalBal := int64(0)
			if bal != nil {
				finalBal = bal.Balance
			}
			results[uid] = ui.BaicaoPlayerResult{Rank: rank, Net: share, Balance: finalBal, Status: p.Status}
			_ = b.userRepo.UpdateGameHistory(ctx, uid, "baicao", float64(share)/1000000.0)

			if share > highestWin {
				highestWin = share
				bestPlayer = uid
			}
		} else {
			// Thua: Trả lại số Ký quỹ thừa (nếu có)
			refund := escrowAmount - loserPenalty
			if refund > 0 {
				b.wallet.BetSettle(ctx, uid, refund, true)
			}

			bal, _ := b.wallet.GetBalance(ctx, uid)
			finalBal := int64(0)
			if bal != nil {
				finalBal = bal.Balance
			}
			results[uid] = ui.BaicaoPlayerResult{Rank: rank, Net: -loserPenalty, Balance: finalBal, Status: p.Status}
			_ = b.userRepo.UpdateGameHistory(ctx, uid, "baicao", float64(-loserPenalty)/1000000.0)
		}
	}

	npcDialog := ""
	if isSapWin {
		dialogue.GetManager().MarkActivity(t.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.Jackpot, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Bài Cào", "channel": t.ChannelID})
	} else if highestWin > 0 {
		dialogue.GetManager().MarkActivity(t.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.WinBig, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Bài Cào", "channel": t.ChannelID})
	}

	// Render KQ
	hands := make(map[string][]games.Card)
	for uid, p := range t.Players {
		hands[uid] = p.Hand
	}
	resultEmbed := ui.BuildBaicaoResultEmbed(t.HostID, t.PlayerIDs, hands, results)

	animEmbed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s ĐANG LẬT BÀI...", ui.Emojis.BaicaoBet),
		Description: fmt.Sprintf("\n%s **Hồi hộp quá...** %s %s %s %s\n> *Đang xét bài các nhà con...*", ui.Emojis.BaicaoSkull, ui.Emojis.BaicaoCatRed, ui.Emojis.BaicaoCatBlack, ui.Emojis.BaicaoCatRed, ui.Emojis.BaicaoCatBlack),
		Color:       0xFF3333,
	}
	animMsg, err := s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
		Embed: animEmbed,
	})

	// CACHE KẾT QUẢ ĐỂ KHÔI PHỤC KHI BỊ RATELIMIT
	CacheGameResult(t.ChannelID, "Bài Cào", npcDialog, resultEmbed, ui.BaicaoResultButtons(t.HostID, t.BetAmount, s))

	if err == nil {
		time.Sleep(2500 * time.Millisecond) // Animation sleep lật bài
		embeds1 := []*discordgo.MessageEmbed{resultEmbed}
		comps1 := ui.BaicaoResultButtons(t.HostID, t.BetAmount, s)
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: animMsg.ID, Channel: t.ChannelID, Content: &npcDialog, Embeds: &embeds1, Components: &comps1,
		})
	} else {
		s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
			Content: npcDialog, Embed: resultEmbed, Components: ui.BaicaoResultButtons(t.HostID, t.BetAmount, s),
		})
	}

	// Xử lý tạo ván tiếp theo nếu có chuỗi Auto
	series := GetAutoSeries(t.ChannelID, "baicao")
	if series != nil && series.Active && series.OwnerID == t.HostID {
		if series.CurrentIndex < series.TotalCount {
			go func() {
				time.Sleep(6 * time.Second)
				time.Sleep(3 * time.Second)
				if GetAutoSeries(t.ChannelID, "baicao") != nil {
					b.startBaicaoRound(s, t.ChannelID, t.HostID, series.GuildID, series.Params.BetAmount, series)
				}
			}()
		} else {
			StopAutoSeries(t.ChannelID, "baicao")
			s.ChannelMessageSend(t.ChannelID, fmt.Sprintf("%s <@%s> Đã hoàn thành chuỗi tự động tạo bàn Bài Cào.", ui.Emojis.Success, t.HostID))
		}
	}
}

// Repost helper cho Bài Cào
func (b *Bot) repostBaicaoTable(s *discordgo.Session, channelID, userID string) bool {
	var targetTable *BaicaoTable

	baicaoTables.Range(func(key, value interface{}) bool {
		table := value.(*BaicaoTable)
		if table.ChannelID == channelID && table.HostID == userID && (table.Status == "waiting" || table.Status == "playing") {
			targetTable = table
			return false
		}
		return true
	})

	if targetTable == nil {
		return false
	}
	t := targetTable

	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.HostID != userID || t.Status == "canceled" || t.Status == "finished" || t.Status == "dealing" {
		return false
	}

	playerStatus := make(map[string]string)
	hands := make(map[string][]games.Card)
	for uid, player := range t.Players {
		playerStatus[uid] = player.Status
		hands[uid] = player.Hand
	}

	oldMsgID := t.MessageID
	oldCtrlID := t.ControlID
	embed := ui.BuildBaicaoEmbed(t.TableID, t.HostID, t.BetAmount, 0, t.PlayerIDs, playerStatus, hands, t.Status, t.Deadline.Unix())

	if t.Status == "waiting" {
		msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embed: embed, Components: ui.BaicaoWaitingButtons(t.TableID, s),
		})
		if err == nil {
			t.MessageID = msg.ID
			if oldMsgID != "" {
				_ = s.ChannelMessageDelete(channelID, oldMsgID)
			}
			return true
		}
	} else {
		msg, err := s.ChannelMessageSendEmbed(channelID, embed)
		if err == nil {
			t.MessageID = msg.ID
			if oldMsgID != "" {
				_ = s.ChannelMessageDelete(channelID, oldMsgID)
			}
			ctrlMsg, errCtrl := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content:    "\u200B",
				Components: ui.BaicaoPlayingButtons(t.TableID, s),
			})
			if errCtrl == nil {
				t.ControlID = ctrlMsg.ID
				if oldCtrlID != "" {
					_ = s.ChannelMessageDelete(channelID, oldCtrlID)
				}
			}
			return true
		}
	}
	return false
}
