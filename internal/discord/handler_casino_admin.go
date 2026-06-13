package discord

// ═══════════════════════════════════════════════════════════════════════════════
// handler_casino_admin.go — CASINO CONFIG ADMIN PANEL
//
// CustomID scheme (tối đa 100 ký tự):
//   Buttons  : gcfg:main | gcfg:{game}:menu | gcfg:{game}:payout
//              gcfg:{game}:rig | gcfg:{game}:preset | gcfg:{game}:test:{n}
//              gcfg:{game}:reset | gcfg:{game}:logs | gcfg:{game}:toggle
//   Selects  : gcfg:sel:game | gcfg:{game}:sel:rig | gcfg:{game}:sel:preset
//   Modals   : gcfg:{game}:modal:payout:{channelID}:{messageID}
//              gcfg:{game}:modal:xd:{channelID}:{messageID}
//              gcfg:{game}:modal:baicao:{channelID}:{messageID}
//              gcfg:sys:modal:prefix:0:0
// ═══════════════════════════════════════════════════════════════════════════════

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/service"
	"github.com/discordcasino/v2/internal/ui"
)

// ── THEME CONSTANTS ───────────────────────────────────────────────────────────

const (
	colorCasinoGold = 0xFFD700 // gold — main menu
	colorTaixiu     = 0xC0392B // red — Tài Xỉu
	colorBaucua     = 0x27AE60 // green — Bầu Cua
	colorXidach     = 0x2980B9 // blue — Xì Dách
	colorBaicao     = 0x8E44AD // purple — Bài Cào
	colorSimulation = 0x1ABC9C // teal — test results
	colorAuditLog   = 0x7F8C8D // gray — audit log
	colorDanger     = 0xE74C3C // red — reset/danger
	colorAdmin      = 0xE67E22 // orange — moderation
)

func gameColor(game string) int {
	switch game {
	case "taixiu":
		return colorTaixiu
	case "baucua":
		return colorBaucua
	case "xidach":
		return colorXidach
	case "baicao":
		return colorBaicao
	case "moderation":
		return colorAdmin
	}
	return colorCasinoGold
}

func gameLabel(game string) string {
	switch game {
	case "taixiu":
		return "Tài Xỉu"
	case "baucua":
		return "Bầu Cua"
	case "xidach":
		return "Xì Dách"
	case "baicao":
		return "Bài Cào"
	case "moderation":
		return "Moderation"
	}
	return strings.ToUpper(game)
}

func gameEmojiStr(game string) string {
	switch game {
	case "taixiu":
		return ui.Emojis.KingTX
	case "baucua":
		return ui.Emojis.TitleBC
	case "xidach":
		return ui.Emojis.CardGame
	case "baicao":
		return ui.Emojis.BaicaoKing
	case "moderation":
		return ui.Emojis.Shield
	}
	return ui.Emojis.Settings
}

// ── SLASH COMMAND /cc (EPHEMERAL PANEL TÀNG HÌNH) ─────────────────────────────

func (b *Bot) handleSlashCC(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)
	if role < RoleOwner {
		respondEphemeral(s, i, ui.Emojis.Block+" Whiskey mới có đủ khả năng chạm tới nỗi đau này.")
		return
	}

	embed, components := buildMainMenuUI(s)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral, // TIN NHẮN TÀNG HÌNH CHỈ OWNER THẤY
		},
	})
}

// ── INTERACTION ROUTER ────────────────────────────────────────────────────────

// handleCasinoAdminInteraction xử lý tất cả button + select menu với prefix "gcfg:".
func (b *Bot) handleCasinoAdminInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)
	if role < RoleOwner {
		respondEphemeral(s, i, ui.Emojis.Block+" Whiskey mới có đủ khả năng chạm tới nỗi đau này.")
		return
	}

	data := i.MessageComponentData()
	id := data.CustomID
	parts := strings.Split(id, ":")

	// Tất cả các hành động (trừ việc mở Modal) đều cần Defer ngay lập tức để tránh lỗi Failed Interaction
	needsDefer := true
	if len(parts) == 3 && parts[2] == "payout" {
		needsDefer = false // Mở modal thì phải trả về InteractionResponseModal ngay lập tức
	} else if id == "gcfg:prefix" {
		needsDefer = false // Mở modal để đổi Prefix, không được defer trước
	}

	if needsDefer {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
	}

	// Routing dựa trên customID
	switch {
	// ── Đóng Panel ───────────────────────────────────────────────────────────
	case id == "gcfg:close":
		// Fallback: Xóa Message. Nếu bot mất quyền Delete, sẽ báo lỗi ngầm nhưng không crash
		err := s.ChannelMessageDelete(i.ChannelID, i.Message.ID)
		if err != nil {
			// Fallback edit message thành đã đóng
			embeds := []*discordgo.MessageEmbed{errorEmbed("Panel cấu hình đã được đóng an toàn.")}
			comps := []discordgo.MessageComponent{}
			s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &comps})
		}

	// ── Main menu ────────────────────────────────────────────────────────────
	case id == "gcfg:main":
		b.renderMainMenu(s, i)

	// ── Prefix menu ─────────────────────────────────────────────────────────
	case id == "gcfg:prefix":
		b.openPrefixModal(s, i)

	// ── Game selection (select menu) ─────────────────────────────────────────
	case id == "gcfg:sel:game" && len(data.Values) > 0:
		b.renderGameMenu(s, i, data.Values[0])

	// ── Game menu (button) ───────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "menu":
		b.renderGameMenu(s, i, parts[1])

	// ── Toggle game on/off ───────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "toggle":
		b.handleToggleGame(s, i, ctx, parts[1])

	// ── Open payout modal ────────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "payout":
		b.openPayoutModal(s, i, parts[1])

	// ── Rig mode — show select ────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "rig":
		b.renderRigMenu(s, i, parts[1])

	// ── Rig mode — apply selection ────────────────────────────────────────────
	case len(parts) == 4 && parts[2] == "sel" && parts[3] == "rig" && len(data.Values) > 0:
		b.handleSetRig(s, i, ctx, parts[1], models.RigMode(data.Values[0]))

	// ── Preset — show select ──────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "preset":
		b.renderPresetMenu(s, i, parts[1])

	// ── Preset — apply selection ──────────────────────────────────────────────
	case len(parts) == 4 && parts[2] == "sel" && parts[3] == "preset" && len(data.Values) > 0:
		b.handleApplyPreset(s, i, ctx, parts[1], models.GamePreset(data.Values[0]))

	// ── Simulation test ───────────────────────────────────────────────────────
	case len(parts) == 4 && parts[2] == "test":
		n, _ := strconv.Atoi(parts[3])
		b.handleSimulation(s, i, ctx, parts[1], n)

	// ── Reset to defaults ─────────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "reset":
		b.handleResetConfig(s, i, ctx, parts[1])

	// ── Audit logs ────────────────────────────────────────────────────────────
	case len(parts) == 3 && parts[2] == "logs":
		b.renderAuditLogs(s, i, ctx, parts[1])
	}
}

// handleCasinoAdminModal xử lý modal submit với prefix "gcfg:".
func (b *Bot) handleCasinoAdminModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)
	if role < RoleOwner {
		respondEphemeral(s, i, ui.Emojis.Block+" Whiskey mới có đủ khả năng chạm tới nỗi đau này.")
		return
	}

	data := i.ModalSubmitData()
	id := data.CustomID
	// Format: gcfg:{game}:modal:{type}:{channelID}:{messageID}
	parts := strings.Split(id, ":")
	if len(parts) < 6 {
		respondEphemeral(s, i, "❌ Modal ID không hợp lệ.")
		return
	}

	// 1. NGAY LẬP TỨC ACK TƯƠNG TÁC ĐỂ TRÁNH LỖI "INTERACTION FAILED"
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("[CASINO_ADMIN_MODAL] Lỗi defer interaction: %v", err)
		return
	}

	game := parts[1]
	modalType := parts[3]
	channelID := parts[4]
	messageID := parts[5]

	userID := interactionUserID(i)

	switch modalType {
	case "payout":
		b.handlePayoutModalSubmit(s, i, ctx, game, userID, channelID, messageID, data.Components)
	case "xd":
		b.handleXidachModalSubmit(s, i, ctx, userID, channelID, messageID, data.Components)
	case "baicao":
		b.handleBaicaoModalSubmit(s, i, ctx, userID, channelID, messageID, data.Components)
	case "prefix":
		b.handlePrefixModalSubmit(s, i, ctx, userID, data.Components)
	}
}

// ── RENDER: MAIN MENU ─────────────────────────────────────────────────────────

func buildMainMenuUI(s *discordgo.Session) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	embed := &discordgo.MessageEmbed{
		Title:       ui.Emojis.Settings + " CASINO CONFIG PANEL",
		Description: "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\nChọn game bạn muốn cấu hình:\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		Color:       colorCasinoGold,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Chỉ Admin/Owner mới thao tác được"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{Name: ui.Emojis.KingTX + " Tài Xỉu", Value: "Chỉnh payout, rig mode, test RNG", Inline: true},
			{Name: ui.Emojis.TitleBC + " Bầu Cua", Value: "Chỉnh payout per match, rig mode", Inline: true},
			{Name: ui.Emojis.CardGame + " Xì Dách", Value: "Chỉnh dealer stand, payout tay đặc biệt", Inline: true},
			{Name: ui.Emojis.BaicaoKing + " Bài Cào", Value: "Chỉnh payout Sáp, host peek config", Inline: true},
		},
	}

	menuRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    "gcfg:sel:game",
				Placeholder: "Chọn game để cấu hình...",
				Options: []discordgo.SelectMenuOption{
					{Label: "Tài Xỉu", Value: "taixiu", Description: "Xúc xắc 3 mặt — Tài/Xỉu/Bộ Ba/Hũ", Emoji: ui.ParseEmoji(s, ui.Emojis.KingTX, "🎯")},
					{Label: "Bầu Cua", Value: "baucua", Description: "Bầu Cua Tôm Cá — 6 con vật", Emoji: ui.ParseEmoji(s, ui.Emojis.TitleBC, "🎲")},
					{Label: "Xì Dách", Value: "xidach", Description: "Blackjack — 21 điểm", Emoji: ui.ParseEmoji(s, ui.Emojis.CardGame, "🃏")},
					{Label: "Bài Cào", Value: "baicao", Description: "3 lá bài — Sáp/Ba Cào/Điểm", Emoji: ui.ParseEmoji(s, ui.Emojis.BaicaoKing, "🀄")},
					{Label: "Moderation", Value: "moderation", Description: "Quản lý lệnh Moderation (Delete, Clear)", Emoji: ui.ParseEmoji(s, ui.Emojis.Shield, "🛡️")},
				},
			},
		},
	}

	closeRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: "gcfg:prefix",
				Label:    "Đổi Prefix",
				Style:    discordgo.SecondaryButton,
				Emoji:    ui.ParseEmoji(s, ui.Emojis.Settings, "⚙️"),
			},
			discordgo.Button{
				CustomID: "gcfg:close",
				Label:    "Đóng Bảng",
				Style:    discordgo.DangerButton,
				Emoji:    ui.ParseEmoji(s, ui.Emojis.Error, "✖️"),
			},
		},
	}

	return embed, []discordgo.MessageComponent{menuRow, closeRow}
}

func (b *Bot) renderMainMenu(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed, components := buildMainMenuUI(s)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// ── RENDER: GAME MENU ─────────────────────────────────────────────────────────

func (b *Bot) renderGameMenu(s *discordgo.Session, i *discordgo.InteractionCreate, game string) {
	embed, components := b.buildGameMenuUI(s, game)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func (b *Bot) buildGameMenuUI(s *discordgo.Session, game string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	cfg := b.casinoConfigSvc.GetConfig(game)

	statusStr := ui.Emojis.Success + " Đang Chạy"
	if !cfg.Enabled {
		statusStr = ui.Emojis.Error + " Đã Tắt"
	}

	embed := &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("%s CẤU HÌNH %s", gameEmojiStr(game), strings.ToUpper(gameLabel(game))),
		Color:     gameColor(game),
		Timestamp: time.Now().Format(time.RFC3339),
		Fields:    buildGameConfigFields(game, cfg),
	}

	embed.Fields = append([]*discordgo.MessageEmbedField{
		{Name: ui.Emojis.Tag + " Trạng Thái", Value: statusStr, Inline: true},
		{Name: ui.Emojis.Warning + " Rig Mode", Value: cfg.RigMode.Label(), Inline: true},
	}, embed.Fields...)

	if cfg.UpdatedBy != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Cập nhật bởi <@%s> lúc %s",
				cfg.UpdatedBy, cfg.UpdatedAt.Local().Format("02/01 15:04")),
		}
	}

	var row1 discordgo.ActionsRow
	var row2 discordgo.ActionsRow

	if game == "moderation" {
		row1 = discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{},
		}
	} else {
		// Row 1: Payout + Rig + Preset
		row1 = discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Chỉnh Payout", fmt.Sprintf("gcfg:%s:payout", game), ui.Emojis.Money, "💰", discordgo.PrimaryButton),
				adminBtn(s, "Rig Mode", fmt.Sprintf("gcfg:%s:rig", game), ui.Emojis.Warning, "⚠️", discordgo.SecondaryButton),
				adminBtn(s, "Preset", fmt.Sprintf("gcfg:%s:preset", game), ui.Emojis.Party, "🎉", discordgo.SecondaryButton),
			},
		}

		// Row 2: Test RNG buttons
		row2 = discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Test 100", fmt.Sprintf("gcfg:%s:test:100", game), ui.Emojis.Dice, "🧪", discordgo.SecondaryButton),
				adminBtn(s, "Test 1.000", fmt.Sprintf("gcfg:%s:test:1000", game), ui.Emojis.Dice, "🧪", discordgo.SecondaryButton),
				adminBtn(s, "Test 10.000", fmt.Sprintf("gcfg:%s:test:10000", game), ui.Emojis.Dice, "🧪", discordgo.SecondaryButton),
			},
		}
	}

	// Row 3: Logs + Reset + Toggle + Back
	toggleLabel := "Tắt Game"
	toggleEmoji := ui.Emojis.Error
	toggleStyle := discordgo.DangerButton
	if !cfg.Enabled {
		toggleLabel = "Bật Game"
		toggleEmoji = ui.Emojis.Success
		toggleStyle = discordgo.SuccessButton
	}

	if game == "moderation" {
		toggleLabel = "Tắt Lệnh Xóa"
		if !cfg.Enabled {
			toggleLabel = "Bật Lệnh Xóa"
		}
	}

	var row3Components []discordgo.MessageComponent
	if game != "moderation" {
		row3Components = append(row3Components, adminBtn(s, "Logs", fmt.Sprintf("gcfg:%s:logs", game), ui.Emojis.History, "📋", discordgo.SecondaryButton))
		row3Components = append(row3Components, adminBtn(s, "Khôi Phục Mặc Định", fmt.Sprintf("gcfg:%s:reset", game), ui.Emojis.Restart, "🔄", discordgo.DangerButton))
	}
	row3Components = append(row3Components, adminBtn(s, toggleLabel, fmt.Sprintf("gcfg:%s:toggle", game), toggleEmoji, "🔴", toggleStyle))
	row3Components = append(row3Components, adminBtn(s, "Quay Lại", "gcfg:main", ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton))

	row3 := discordgo.ActionsRow{
		Components: row3Components,
	}

	if game == "moderation" {
		return embed, []discordgo.MessageComponent{row3}
	}
	return embed, []discordgo.MessageComponent{row1, row2, row3}
}

func buildGameConfigFields(game string, cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	switch game {
	case "taixiu":
		return buildTaixiuFields(cfg)
	case "baucua":
		return buildBaucuaFields(cfg)
	case "xidach":
		return buildXidachFields(cfg)
	case "baicao":
		return buildBaicaoFields(cfg)
	case "moderation":
		return buildModerationFields(cfg)
	}
	return nil
}

func buildTaixiuFields(cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	txc := models.DefaultTaixiuConfig
	if cfg.TaixiuCfg != nil {
		txc = *cfg.TaixiuCfg
	}
	rtp, edge := service.CalcTaixiuHouseEdge(txc)
	return []*discordgo.MessageEmbedField{
		{
			Name: ui.Emojis.Money + " Payout Multipliers",
			Value: fmt.Sprintf("Tài/Xỉu: **x%.2f** | Tam Giống: **x%.0f** | Hũ (111/666): **x%.0f**",
				txc.PayoutNormal, txc.PayoutTriple, txc.PayoutHu),
			Inline: false,
		},
		{
			Name:   ui.Emojis.History + " Xác Suất Lý Thuyết",
			Value:  "Tài: **48.6%** | Xỉu: **48.6%** | Tam Giống: **2.78%** | Hũ: **0.93%**",
			Inline: false,
		},
		{
			Name: ui.Emojis.Bank + " House Edge (Lý Thuyết)",
			Value: fmt.Sprintf("RTP: **%.1f%%** | House Edge: **%.1f%%** | Anti-streak: %s",
				rtp, edge, streakStr(txc.MaxStreak)),
			Inline: false,
		},
	}
}

func buildBaucuaFields(cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	bc := models.DefaultBaucuaConfig
	if cfg.BaucuaCfg != nil {
		bc = *cfg.BaucuaCfg
	}
	// House edge lý thuyết: -17/216 ≈ 7.87% (1x per match)
	// Scaled theo payout thực
	houseEdge := (1 - (0 + bc.PayoutPerMatch*0.3472 + bc.PayoutPerMatch*2*0.0694 + bc.PayoutPerMatch*3*0.0046)) * 100
	if houseEdge < 0 {
		houseEdge = 0
	}
	return []*discordgo.MessageEmbedField{
		{
			Name:   ui.Emojis.Money + " Payout Per Match",
			Value:  fmt.Sprintf("Mỗi mặt khớp: **x%.2f** (cộng vào cược gốc)", bc.PayoutPerMatch),
			Inline: false,
		},
		{
			Name:   ui.Emojis.History + " Xác Suất Lý Thuyết",
			Value:  "0 khớp: **57.9%** | 1 khớp: **34.7%** | 2 khớp: **6.9%** | 3 khớp: **0.5%**",
			Inline: false,
		},
		{
			Name: ui.Emojis.Bank + " House Edge (Lý Thuyết)",
			Value: fmt.Sprintf("RTP ≈ **%.1f%%** | House Edge ≈ **%.1f%%**",
				100-houseEdge, houseEdge),
			Inline: false,
		},
	}
}

func buildXidachFields(cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	xd := models.DefaultXidachConfig
	if cfg.XidachCfg != nil {
		xd = *cfg.XidachCfg
	}
	return []*discordgo.MessageEmbedField{
		{
			Name:   ui.Emojis.CardBack + " Dealer",
			Value:  fmt.Sprintf("Đứng tại: **%d điểm**", xd.DealerStandAt),
			Inline: true,
		},
		{
			Name: ui.Emojis.Money + " Payout",
			Value: fmt.Sprintf("Xì Dách: **x%.1f** | Xì Bàn: **x%.1f** | Ngũ Linh: **x%.1f** | Thường: **x1.0**",
				xd.PayoutBlackjack, xd.PayoutXiBan, xd.PayoutNguLinh),
			Inline: false,
		},
		{
			Name:   ui.Emojis.Bank + " House Edge",
			Value:  "≈ **0.5%** (Standard Blackjack với Basic Strategy)",
			Inline: false,
		},
	}
}

func buildBaicaoFields(cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	bc := models.DefaultBaicaoConfig
	if cfg.BaicaoCfg != nil {
		bc = *cfg.BaicaoCfg
	}
	peekStr := ui.Emojis.Error + " Tắt"
	if bc.HostPeekEnabled {
		peekStr = ui.Emojis.Success + " Bật"
	}
	return []*discordgo.MessageEmbedField{
		{
			Name: ui.Emojis.Money + " Payout",
			Value: fmt.Sprintf("Thường: **x%.1f** | Sáp (3 lá cùng số): **x%.1f** | Ba Cào (3 hình): **x1.0**",
				bc.PayoutNormal, bc.PayoutSap),
			Inline: false,
		},
		{
			Name:   ui.Emojis.CardBack + " Đặc Quyền Cái",
			Value:  fmt.Sprintf("Xem bài trước khi rút quân 3: **%s**", peekStr),
			Inline: false,
		},
		{
			Name:   ui.Emojis.Bank + " House Edge",
			Value:  "≈ **5%** (phụ thuộc vào host peek + rig mode)",
			Inline: false,
		},
	}
}

func buildModerationFields(cfg *models.CasinoGameConfig) []*discordgo.MessageEmbedField {
	return []*discordgo.MessageEmbedField{
		{
			Name:   ui.Emojis.Shield + " Lệnh Moderation",
			Value:  "Hỗ trợ: `delete`, `clear`, `purge`\nQuyền: Admin/Owner hoặc Manage Messages",
			Inline: false,
		},
	}
}

func streakStr(max int) string {
	if max == 0 {
		return "Tắt"
	}
	return fmt.Sprintf("Max %d liên tiếp", max)
}

// ── TOGGLE GAME ───────────────────────────────────────────────────────────────

func (b *Bot) handleToggleGame(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string) {
	userID := interactionUserID(i)
	enabled, err := b.casinoConfigSvc.ToggleGame(ctx, game, userID)
	if err != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi cập nhật: "+err.Error())
		return
	}
	status := ui.Emojis.Success + " BẬT"
	if !enabled {
		status = ui.Emojis.Error + " TẮT"
	}

	msg := fmt.Sprintf("%s Game **%s** đã được **%s**.", gameEmojiStr(game), gameLabel(game), status)
	followupEphemeral(s, i, msg)

	// Refresh panel
	embed, components := b.buildGameMenuUI(s, game)
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// ── PAYOUT MODAL ──────────────────────────────────────────────────────────────

func (b *Bot) openPayoutModal(s *discordgo.Session, i *discordgo.InteractionCreate, game string) {
	// Encode channelID + messageID để sau khi submit có thể edit lại panel
	channelID := i.ChannelID
	msgID := ""
	if i.Message != nil {
		msgID = i.Message.ID
	}

	cfg := b.casinoConfigSvc.GetConfig(game)

	var modal discordgo.InteractionResponseData
	switch game {
	case "taixiu":
		txc := models.DefaultTaixiuConfig
		if cfg.TaixiuCfg != nil {
			txc = *cfg.TaixiuCfg
		}
		modal = buildTaixiuPayoutModal(txc, channelID, msgID)
	case "baucua":
		bc := models.DefaultBaucuaConfig
		if cfg.BaucuaCfg != nil {
			bc = *cfg.BaucuaCfg
		}
		modal = buildBaucuaPayoutModal(bc, channelID, msgID)
	case "xidach":
		xd := models.DefaultXidachConfig
		if cfg.XidachCfg != nil {
			xd = *cfg.XidachCfg
		}
		modal = buildXidachPayoutModal(xd, channelID, msgID)
	case "baicao":
		bc := models.DefaultBaicaoConfig
		if cfg.BaicaoCfg != nil {
			bc = *cfg.BaicaoCfg
		}
		modal = buildBaicaoPayoutModal(bc, channelID, msgID)
	default:
		respondEphemeral(s, i, ui.Emojis.Error+" Game không hỗ trợ chỉnh payout.")
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &modal,
	})
}

func buildTaixiuPayoutModal(txc models.TaixiuConfig, channelID, msgID string) discordgo.InteractionResponseData {
	return discordgo.InteractionResponseData{
		CustomID: fmt.Sprintf("gcfg:taixiu:modal:payout:%s:%s", channelID, msgID),
		Title:    "Chỉnh Payout — Tài Xỉu",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_normal",
					Label:       "Tỉ lệ Lời Tài/Xỉu (VD: 0.95 hoặc 1.0)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.2f", txc.PayoutNormal),
					Placeholder: "Nhập số > 0...",
					Required:    true,
					MinLength:   1,
					MaxLength:   10,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_triple",
					Label:       "Payout Tam Giống 222-555 (VD: 24)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.0f", txc.PayoutTriple),
					Placeholder: "Nhập số > 0...",
					Required:    true,
					MinLength:   1,
					MaxLength:   10,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_hu",
					Label:       "Payout Hũ 111/666 (VD: 10)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.0f", txc.PayoutHu),
					Placeholder: "Nhập số > 0...",
					Required:    true,
					MinLength:   1,
					MaxLength:   10,
				},
			}},
		},
	}
}

func buildBaucuaPayoutModal(bc models.BaucuaConfig, channelID, msgID string) discordgo.InteractionResponseData {
	return discordgo.InteractionResponseData{
		CustomID: fmt.Sprintf("gcfg:baucua:modal:payout:%s:%s", channelID, msgID),
		Title:    "Chỉnh Payout — Bầu Cua",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_per_match",
					Label:       "Payout mỗi mặt khớp (VD: 1.0 = x1 mỗi mặt)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.2f", bc.PayoutPerMatch),
					Placeholder: "Nhập số > 0...",
					Required:    true,
					MinLength:   1,
					MaxLength:   10,
				},
			}},
		},
	}
}

func buildXidachPayoutModal(xd models.XidachConfig, channelID, msgID string) discordgo.InteractionResponseData {
	return discordgo.InteractionResponseData{
		CustomID: fmt.Sprintf("gcfg:xidach:modal:xd:%s:%s", channelID, msgID),
		Title:    "Chỉnh Payout — Xì Dách",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_blackjack",
					Label:       "Payout Xì Dách (Ace + 10-value)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.1f", xd.PayoutBlackjack),
					Placeholder: "Nhập số > 0...",
					Required:    true,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_xi_ban",
					Label:       "Payout Xì Bàn (2 Át)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.1f", xd.PayoutXiBan),
					Placeholder: "Nhập số > 0...",
					Required:    true,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_ngu_linh",
					Label:       "Payout Ngũ Linh (5 lá ≤21)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.1f", xd.PayoutNguLinh),
					Placeholder: "Nhập số > 0...",
					Required:    true,
				},
			}},
		},
	}
}

func buildBaicaoPayoutModal(bc models.BaicaoConfig, channelID, msgID string) discordgo.InteractionResponseData {
	peekVal := "0"
	if bc.HostPeekEnabled {
		peekVal = "1"
	}
	return discordgo.InteractionResponseData{
		CustomID: fmt.Sprintf("gcfg:baicao:modal:baicao:%s:%s", channelID, msgID),
		Title:    "Chỉnh Payout — Bài Cào",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_normal",
					Label:       "Payout tay thường (VD: 1.0)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.1f", bc.PayoutNormal),
					Placeholder: "Nhập số > 0...",
					Required:    true,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "payout_sap",
					Label:       "Payout Sáp — 3 lá cùng số (VD: 2.0)",
					Style:       discordgo.TextInputShort,
					Value:       fmt.Sprintf("%.1f", bc.PayoutSap),
					Placeholder: "Nhập số > 0...",
					Required:    true,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.TextInput{
					CustomID:    "host_peek",
					Label:       "Đặc quyền Cái xem bài? (1=BẬT, 0=TẮT)",
					Style:       discordgo.TextInputShort,
					Value:       peekVal,
					Placeholder: "1 hoặc 0",
					Required:    true,
					MaxLength:   1,
				},
			}},
		},
	}
}

func (b *Bot) openPrefixModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "gcfg:sys:modal:prefix:0:0",
			Title:    "Cấu Hình Prefix Lệnh",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "prefix_value",
						Label:       "Ký tự prefix mới (VD: !, ?, $)",
						Style:       discordgo.TextInputShort,
						Placeholder: "Nhập 1 ký tự...",
						Required:    true,
						MaxLength:   3,
					},
				}},
			},
		},
	})
}

func (b *Bot) handlePrefixModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, userID string, comps []discordgo.MessageComponent) {
	vals := extractModalValues(comps)
	newPrefix := vals["prefix_value"]
	if newPrefix == "" {
		followupEphemeral(s, i, ui.Emojis.Error+" Prefix không được để trống.")
		return
	}
	cfg, _ := b.configRepo.Get(ctx)
	cfg.Prefix = newPrefix
	_ = b.configRepo.Save(ctx, cfg)
	followupEphemeral(s, i, fmt.Sprintf("%s Đã đổi Prefix lệnh thành: `%s`", ui.Emojis.Success, newPrefix))
}

// ── MODAL SUBMIT HANDLERS ─────────────────────────────────────────────────────

func (b *Bot) handlePayoutModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context,
	game, userID, channelID, messageID string, comps []discordgo.MessageComponent) {

	vals := extractModalValues(comps)
	var err error

	switch game {
	case "taixiu":
		normal, e1 := parseFloat(vals["payout_normal"])
		triple, e2 := parseFloat(vals["payout_triple"])
		hu, e3 := parseFloat(vals["payout_hu"])
		if e1 != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Payout Tài/Xỉu: "+e1.Error())
			return
		}
		if e2 != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Payout Tam Giống: "+e2.Error())
			return
		}
		if e3 != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Payout Hũ: "+e3.Error())
			return
		}
		err = b.casinoConfigSvc.UpdateTaixiuPayout(ctx, normal, triple, hu, userID)

	case "baucua":
		ppm, e := parseFloat(vals["payout_per_match"])
		if e != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Payout Bầu Cua: "+e.Error())
			return
		}
		err = b.casinoConfigSvc.UpdateBaucuaPayout(ctx, ppm, userID)
	}

	if err != nil {
		log.Printf("[CASINO_ADMIN] Lỗi update config payout (Game: %s, User: %s): %v", game, userID, err)
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi lưu config: "+err.Error())
		return
	}

	followupEphemeral(s, i, fmt.Sprintf("%s Đã cập nhật Payout **%s** thành công!", ui.Emojis.Success, gameLabel(game)))
	refreshPanelMessage(s, b, channelID, messageID, game)
}

func (b *Bot) handleXidachModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context,
	userID, channelID, messageID string, comps []discordgo.MessageComponent) {

	vals := extractModalValues(comps)
	bj, e1 := parseFloat(vals["payout_blackjack"])
	xb, e2 := parseFloat(vals["payout_xi_ban"])
	nl, e3 := parseFloat(vals["payout_ngu_linh"])
	if e1 != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Payout Xì Dách: "+e1.Error())
		return
	}
	if e2 != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Payout Xì Bàn: "+e2.Error())
		return
	}
	if e3 != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Payout Ngũ Linh: "+e3.Error())
		return
	}

	if err := b.casinoConfigSvc.UpdateXidachPayout(ctx, bj, xb, nl, userID); err != nil {
		log.Printf("[CASINO_ADMIN] Lỗi update config Xì Dách (User: %s): %v", userID, err)
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi lưu config: "+err.Error())
		return
	}

	followupEphemeral(s, i, ui.Emojis.Success+" Đã cập nhật Payout **Xì Dách** thành công!")
	refreshPanelMessage(s, b, channelID, messageID, "xidach")
}

func (b *Bot) handleBaicaoModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context,
	userID, channelID, messageID string, comps []discordgo.MessageComponent) {

	vals := extractModalValues(comps)
	normal, e1 := parseFloat(vals["payout_normal"])
	sap, e2 := parseFloat(vals["payout_sap"])
	if e1 != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Payout Thường: "+e1.Error())
		return
	}
	if e2 != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Payout Sáp: "+e2.Error())
		return
	}
	peek := vals["host_peek"] == "1"

	if err := b.casinoConfigSvc.UpdateBaicaoPayout(ctx, normal, sap, peek, userID); err != nil {
		log.Printf("[CASINO_ADMIN] Lỗi update config Bài Cào (User: %s): %v", userID, err)
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi lưu config: "+err.Error())
		return
	}

	followupEphemeral(s, i, ui.Emojis.Success+" Đã cập nhật Payout **Bài Cào** thành công!")
	refreshPanelMessage(s, b, channelID, messageID, "baicao")
}

// refreshPanelMessage edit lại message gốc với config mới nhất.
func refreshPanelMessage(s *discordgo.Session, b *Bot, channelID, messageID, game string) {
	if channelID == "" || messageID == "" {
		return
	}
	embed, components := b.buildGameMenuUI(s, game)
	embeds := []*discordgo.MessageEmbed{embed}
	_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    channelID,
		ID:         messageID,
		Embeds:     &embeds,
		Components: &components,
	})
	if err != nil {
		log.Printf("[CASINO_ADMIN] Lỗi refresh bảng điều khiển game %s: %v", game, err)
	}
}

// ── RIG MODE MENU ─────────────────────────────────────────────────────────────

func (b *Bot) renderRigMenu(s *discordgo.Session, i *discordgo.InteractionCreate, game string) {
	cfg := b.casinoConfigSvc.GetConfig(game)
	currentRig := cfg.RigMode

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s RIG MODE — %s", gameEmojiStr(game), strings.ToUpper(gameLabel(game))),
		Description: "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n" +
			"Rig mode kiểm soát mức độ **ưu tiên nhà cái** khi lăn lại kết quả.\n\n" +
			ui.Emojis.Success + " **TẮT** — Kết quả hoàn toàn ngẫu nhiên\n" +
			ui.Emojis.Star + " **NHẸ** — 40% cơ hội reroll có lợi cho host\n" +
			ui.Emojis.Warning + " **CÔNG BẰNG** — 50% (mặc định)\n" +
			ui.Emojis.Error + " **NẶNG** — 80% ưu tiên nhà cái\n" +
			ui.Emojis.Fire + " **ÁC MỘNG** — 95% ưu tiên nhà cái\n" +
			"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		Color: gameColor(game),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Đang Dùng", Value: currentRig.Label(), Inline: true},
		},
	}

	options := []discordgo.SelectMenuOption{
		{Label: "TẮT", Value: string(models.RigOff), Description: "Kết quả 100% ngẫu nhiên, không can thiệp", Emoji: ui.ParseEmoji(s, ui.Emojis.Success, "🟢"), Default: currentRig == models.RigOff},
		{Label: "NHẸ (40%)", Value: string(models.RigLight), Description: "40% cơ hội reroll có lợi cho host", Emoji: ui.ParseEmoji(s, ui.Emojis.Star, "🟡"), Default: currentRig == models.RigLight},
		{Label: "CÔNG BẰNG (50%)", Value: string(models.RigNormal), Description: "Mặc định — cân bằng casino 50/50", Emoji: ui.ParseEmoji(s, ui.Emojis.Warning, "🟠"), Default: currentRig == models.RigNormal},
		{Label: "NẶNG (80%)", Value: string(models.RigHard), Description: "Host thắng mạnh hơn nhiều", Emoji: ui.ParseEmoji(s, ui.Emojis.Error, "🔴"), Default: currentRig == models.RigHard},
		{Label: "ÁC MỘNG (95%)", Value: string(models.RigNightmare), Description: "Nhà cái thắng tối đa", Emoji: ui.ParseEmoji(s, ui.Emojis.Fire, "💀"), Default: currentRig == models.RigNightmare},
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    fmt.Sprintf("gcfg:%s:sel:rig", game),
					Placeholder: "Chọn Rig Mode...",
					Options:     options,
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Quay Lại", fmt.Sprintf("gcfg:%s:menu", game), ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton),
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func (b *Bot) handleSetRig(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string, mode models.RigMode) {
	userID := interactionUserID(i)
	if err := b.casinoConfigSvc.SetRigMode(ctx, game, mode, userID); err != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi cập nhật rig mode: "+err.Error())
		return
	}
	// Update to game menu with confirmation
	embed, components := b.buildGameMenuUI(s, game)
	embed.Fields = append([]*discordgo.MessageEmbedField{
		{Name: ui.Emojis.Success + " Đã Cập Nhật", Value: fmt.Sprintf("Rig Mode: **%s**", mode.Label()), Inline: false},
	}, embed.Fields...)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// ── PRESET MENU ───────────────────────────────────────────────────────────────

func (b *Bot) renderPresetMenu(s *discordgo.Session, i *discordgo.InteractionCreate, game string) {
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("%s PRESET — %s", gameEmojiStr(game), strings.ToUpper(gameLabel(game))),
		Description: "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n" +
			"**Preset** áp dụng bộ cài đặt nhanh cho game.\n\n" +
			ui.Emojis.Shield + " **Công Bằng** — Payout cao, Rig OFF (người chơi thích)\n" +
			ui.Emojis.Bank + " **Casino** — Cân bằng, Rig NHẸ (mặc định)\n" +
			ui.Emojis.Party + " **Sự Kiện** — Payout cực cao, Rig OFF (event đặc biệt)\n" +
			ui.Emojis.Fire + " **Tàn Bạo** — Payout thấp, Rig ÁC MỘNG (hardcore)\n" +
			"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n" +
			ui.Emojis.Warning + " **Preset sẽ ghi đè toàn bộ cài đặt hiện tại!**",
		Color: gameColor(game),
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    fmt.Sprintf("gcfg:%s:sel:preset", game),
					Placeholder: "Chọn Preset...",
					Options: []discordgo.SelectMenuOption{
						{
							Label:       "Công Bằng",
							Value:       string(models.PresetFair),
							Description: "Payout tăng, Rig OFF — người chơi được lợi",
							Emoji:       ui.ParseEmoji(s, ui.Emojis.Shield, "⚖️"),
						},
						{
							Label:       "Casino (Mặc Định)",
							Value:       string(models.PresetCasino),
							Description: "Cân bằng casino thực tế — Rig NHẸ",
							Emoji:       ui.ParseEmoji(s, ui.Emojis.Bank, "🎰"),
						},
						{
							Label:       "Sự Kiện",
							Value:       string(models.PresetEvent),
							Description: "Payout cao bất thường — Rig OFF",
							Emoji:       ui.ParseEmoji(s, ui.Emojis.Party, "🎉"),
						},
						{
							Label:       "Tàn Bạo",
							Value:       string(models.PresetBrutal),
							Description: "Payout thấp + Rig ÁC MỘNG — cực khó",
							Emoji:       ui.ParseEmoji(s, ui.Emojis.Fire, "💀"),
						},
					},
				},
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Quay Lại", fmt.Sprintf("gcfg:%s:menu", game), ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton),
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

func (b *Bot) handleApplyPreset(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string, preset models.GamePreset) {
	userID := interactionUserID(i)
	if err := b.casinoConfigSvc.ApplyPreset(ctx, game, preset, userID); err != nil {
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi áp dụng preset: "+err.Error())
		return
	}
	embed, components := b.buildGameMenuUI(s, game)
	embed.Fields = append([]*discordgo.MessageEmbedField{
		{Name: ui.Emojis.Success + " Preset Đã Áp Dụng", Value: fmt.Sprintf("**%s**", preset.Label()), Inline: false},
	}, embed.Fields...)

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// ── SIMULATION ────────────────────────────────────────────────────────────────

func (b *Bot) handleSimulation(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string, n int) {
	if n <= 0 {
		n = 1000
	}

	// BỎ double defer: Vì tương tác đã được b.handleCasinoAdminInteraction Defer từ trước đó
	// Loại bỏ lỗi rác "Interaction has already been acknowledged" trong console.

	var result service.SimulationResult
	switch game {
	case "taixiu":
		result = b.casinoConfigSvc.SimulateTaixiu(n)
	case "baucua":
		result = b.casinoConfigSvc.SimulateBaucua(n)
	default:
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{{
				Title:       ui.Emojis.Dice + " Test RNG",
				Description: "Simulation cho game này chưa được hỗ trợ.\nSắp ra mắt!",
				Color:       colorSimulation,
			}},
			Components: &[]discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					adminBtn(s, "Quay Lại", fmt.Sprintf("gcfg:%s:menu", game), ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton),
				}},
			},
		})
		return
	}

	embed := buildSimResultEmbed(game, result)
	backComponents := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Chạy Lại", fmt.Sprintf("gcfg:%s:test:%d", game, n), ui.Emojis.Restart, "🔁", discordgo.SecondaryButton),
				adminBtn(s, "Quay Lại Panel", fmt.Sprintf("gcfg:%s:menu", game), ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton),
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &backComponents,
	})
}

func buildSimResultEmbed(game string, r service.SimulationResult) *discordgo.MessageEmbed {
	nFmt := fmtInt(r.N)
	histLines := strings.Join(r.Histogram, "\n")

	fields := []*discordgo.MessageEmbedField{
		{
			Name:   ui.Emojis.History + " Phân Bố Kết Quả",
			Value:  histLines,
			Inline: false,
		},
		{
			Name:   ui.Emojis.Tag + " RTP (Return to Player)",
			Value:  fmt.Sprintf("**%.2f%%** — người chơi nhận lại", r.RTP),
			Inline: true,
		},
		{
			Name:   ui.Emojis.Bank + " House Edge",
			Value:  fmt.Sprintf("**%.2f%%** — nhà cái giữ lại", r.HouseEdge),
			Inline: true,
		},
		{
			Name:   ui.Emojis.Tag + " Ý Nghĩa",
			Value:  simInterpretation(r),
			Inline: false,
		},
	}

	var title string
	switch game {
	case "taixiu":
		title = fmt.Sprintf("%s TEST RNG — TÀI XỈU (%s vòng)", ui.Emojis.Dice, nFmt)
	case "baucua":
		title = fmt.Sprintf("%s TEST RNG — BẦU CUA (%s vòng, cược vào Bầu)", ui.Emojis.Dice, nFmt)
	default:
		title = fmt.Sprintf("%s TEST RNG — %s (%s vòng)", ui.Emojis.Dice, strings.ToUpper(game), nFmt)
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\nKết quả simulation với config hiện tại:",
		Color:       colorSimulation,
		Fields:      fields,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "Simulation dùng PRNG (fast) — không ảnh hưởng game thật"},
	}
}

func simInterpretation(r service.SimulationResult) string {
	if r.HouseEdge < 0 {
		return fmt.Sprintf("%s **Người chơi được lợi** %.2f%% — casino lỗ!", ui.Emojis.Gift, -r.HouseEdge)
	}
	if r.HouseEdge < 3 {
		return fmt.Sprintf("%s **Cân bằng** — House Edge thấp (%.2f%%), người chơi gần thắng hòa", ui.Emojis.Shield, r.HouseEdge)
	}
	if r.HouseEdge < 8 {
		return fmt.Sprintf("%s **Casino chuẩn** — House Edge %.2f%%, nhà cái có lợi thế vừa phải", ui.Emojis.Bank, r.HouseEdge)
	}
	if r.HouseEdge < 15 {
		return fmt.Sprintf("%s **Nhà cái lợi cao** — House Edge %.2f%%, người chơi thua nhiều", ui.Emojis.Error, r.HouseEdge)
	}
	return fmt.Sprintf("%s **Cực kỳ bất lợi** cho người chơi — House Edge %.2f%%", ui.Emojis.Fire, r.HouseEdge)
}

// ── RESET ─────────────────────────────────────────────────────────────────────

func (b *Bot) handleResetConfig(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string) {
	userID := interactionUserID(i)
	customID := ""
	if data, ok := i.Interaction.Data.(discordgo.MessageComponentInteractionData); ok {
		customID = data.CustomID
	}
	log.Printf("[ADMIN_RESET] received custom_id=%s user_id=%s", customID, userID)

	if err := b.casinoConfigSvc.ResetToDefault(ctx, game, userID); err != nil {
		log.Printf("[ADMIN_RESET] failed to reset service: %v", err)
		followupEphemeral(s, i, ui.Emojis.Error+" Lỗi reset: "+err.Error())
		return
	}
	log.Printf("[ADMIN_RESET] reset service completed")
	log.Printf("[ADMIN_RESET] rendering config panel")

	embed, components := b.buildGameMenuUI(s, game)
	embed.Fields = append([]*discordgo.MessageEmbedField{
		{Name: ui.Emojis.Restart + " Đã Reset", Value: "Cấu hình **" + gameLabel(game) + "** đã được khôi phục về mặc định.", Inline: false},
	}, embed.Fields...)

	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
	if err != nil {
		log.Printf("[ADMIN_RESET] failed to edit/send panel: %v", err)
		return
	}
	log.Printf("[ADMIN_RESET] panel render completed")
}

// ── AUDIT LOGS ────────────────────────────────────────────────────────────────

func (b *Bot) renderAuditLogs(s *discordgo.Session, i *discordgo.InteractionCreate, ctx context.Context, game string) {
	logs, err := b.casinoConfigSvc.GetRecentLogs(ctx, game, 8)
	if err != nil {
		// PHẢI DÙNG followup vì interaction đã được Defer ở Router tổng, dùng respondEphemeral sẽ văng lỗi Failed Interaction
		followupEphemeral(s, i, ui.Emojis.Error+" Không thể tải logs: "+err.Error())
		return
	}

	var desc string
	if len(logs) == 0 {
		desc = ui.Emojis.Empty + " Chưa có thao tác nào được ghi lại."
	} else {
		var sb strings.Builder
		for _, log := range logs {
			ts := log.ChangedAt.Local().Format("02/01 15:04")
			sb.WriteString(fmt.Sprintf("**%s** — <@%s>\n`%s`\n\n", ts, log.ChangedBy, log.Action))
		}
		desc = sb.String()
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s AUDIT LOG — %s", ui.Emojis.History, strings.ToUpper(gameLabel(game))),
		Description: "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n" + desc,
		Color:       colorAuditLog,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer:      &discordgo.MessageEmbedFooter{Text: "8 thao tác gần nhất"},
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				adminBtn(s, "Quay Lại", fmt.Sprintf("gcfg:%s:menu", game), ui.Emojis.FinishTX, "◀️", discordgo.SecondaryButton),
			},
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	})
}

// ── HELPERS: UI COMPONENTS ────────────────────────────────────────────────────

func adminBtn(s *discordgo.Session, label, customID, emojiStr, fallback string, style discordgo.ButtonStyle) discordgo.Button {
	btn := discordgo.Button{
		Label:    label,
		Style:    style,
		CustomID: customID,
	}
	if emojiStr != "" || fallback != "" {
		btn.Emoji = ui.ParseEmoji(s, emojiStr, fallback)
	}
	return btn
}

// ── HELPERS: VALIDATION ───────────────────────────────────────────────────────

// extractModalValues lấy value từ modal components theo customID.
func extractModalValues(comps []discordgo.MessageComponent) map[string]string {
	out := make(map[string]string)
	for _, row := range comps {
		ar, ok := row.(*discordgo.ActionsRow) // discordgo sử dụng con trỏ (Pointer) cho Modal Data
		if !ok {
			continue
		}
		for _, comp := range ar.Components {
			ti, ok := comp.(*discordgo.TextInput) // Tương tự, cần dùng con trỏ
			if !ok {
				continue
			}
			out[ti.CustomID] = strings.TrimSpace(ti.Value)
		}
	}
	return out
}

// parseFloat parse float với validation.
func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".") // Hỗ trợ định dạng số Việt Nam

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("Giá trị '%s' không hợp lệ. Vui lòng nhập số (VD: 1.95)", s)
	}
	if v <= 0 {
		return 0, fmt.Errorf("Giá trị payout phải lớn hơn 0")
	}
	return v, nil
}

// fmtInt format số nguyên với dấu chấm ngăn cách (VD: 10000 → "10.000").
func fmtInt(n int) string {
	s := strconv.Itoa(n)
	result := make([]byte, 0, len(s)+len(s)/3)
	for idx, c := range s {
		if idx > 0 && (len(s)-idx)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// Suppress unused import
var _ = ui.Emojis
