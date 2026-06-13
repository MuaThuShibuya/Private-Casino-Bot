package discord

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

// Helper kiểm tra quyền truy cập Admin (Owner, Admin DB hoặc Role Discord)
func HasAdminAccess(b *Bot, ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	role := GetEffectiveRole(ctx, b.userRepo, b.configRepo, s, i.GuildID, interactionUserID(i), i.Member)
	return role >= RoleAdmin
}

// writeAdminAuditLog ghi log lại mọi thao tác quan trọng vào Terminal/File (Có thể nâng cấp ghi vào DB sau)
func writeAdminAuditLog(executorID, action, targetType, targetID string, success bool, reason string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	status := "FAIL"
	if success {
		status = "SUCCESS"
	}
	log.Printf("[ADMIN AUDIT] [%s] Executor: %s | Action: %s | TargetType: %s | TargetID: %s | Status: %s | Reason: %s", timestamp, executorID, action, targetType, targetID, status, reason)
}

func adminControlEmbed(b *Bot, ctx context.Context, roleName, userID string) *discordgo.MessageEmbed {
	bal, _ := b.wallet.GetBalance(ctx, userID)
	cfg, _ := b.configRepo.Get(ctx)
	limitStatus := "TẮT"
	if cfg.IsLimitEnabled {
		limitStatus = "BẬT"
	}

	var sb strings.Builder
	sb.WriteString("```ansi\n[ ABYSS OVERSEER TERMINAL ]\n```\n")
	sb.WriteString(fmt.Sprintf("%s **Người quản hành:** <@%s>\n", ui.Emojis.AdminPanel, userID))
	sb.WriteString(fmt.Sprintf("%s **Cấp độ quyền hạn:** `%s`\n", ui.Emojis.Tag, roleName))
	sb.WriteString(fmt.Sprintf("%s **Tài sản Overseer:** %s\n\n", ui.Emojis.Bank, coinDisplay(bal.Balance)))
	sb.WriteString(fmt.Sprintf("%s Giới hạn cược Server: **%s**\n", ui.Emojis.Crown, limitStatus))

	embed := view.NewEmbed(fmt.Sprintf("%s THE ABYSS CONTROL PANEL", ui.Emojis.Crown), sb.String())
	embed.Color = 0x1E0B2D // Màu Abyss đậm chuẩn Production
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Whiskey: The Abyss Dealer | Mọi thao tác đều được lưu vết"}
	return embed
}

// !admin @user   → promote user to admin  (Owner only)
// !admin remove @user → demote back to user
// !admin role add @role → grant admin to a Discord role
// !admin role remove @role → remove admin from a Discord role
func (b *Bot) cmdAdmin(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Only owner may assign admin
	actorRole := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if actorRole < RoleOwner {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Owner mới có thể cấp quyền Admin.", 3*time.Second)
		return
	}

	if len(args) == 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"admin @user` hoặc `"+b.GetPrefix(ctx)+"admin role add @role`", 5*time.Second)
		return
	}

	// Xử lý cấp quyền cho Role (!admin role add @role)
	if strings.ToLower(args[0]) == "role" {
		if len(args) < 3 {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"admin role add @role` hoặc `"+b.GetPrefix(ctx)+"admin role remove @role`", 5*time.Second)
			return
		}

		action := strings.ToLower(args[1])
		roleArg := args[2]

		// Lọc lấy ID chữ số tự động
		var roleID string
		for _, char := range roleArg {
			if char >= '0' && char <= '9' {
				roleID += string(char)
			}
		}

		if roleID == "" {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không tìm thấy ID Role hợp lệ.", 5*time.Second)
			return
		}

		if action == "add" {
			_ = b.configRepo.AddAdminRole(ctx, roleID)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s Đã cấp quyền Admin cho Role `<@&%s>`.", ui.Emojis.Success, roleID))
		} else if action == "remove" {
			_ = b.configRepo.RemoveAdminRole(ctx, roleID)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s Đã thu hồi quyền Admin của Role `<@&%s>`.", ui.Emojis.Success, roleID))
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"admin role add` hoặc `remove`", 5*time.Second)
		}
		return
	}

	removing := strings.ToLower(args[0]) == "remove"
	target := args[0]
	if removing {
		if len(args) < 2 {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"admin remove @user`", 5*time.Second)
			return
		}
		target = args[1]
	}

	targetID, ok := ExtractUserID(target)
	if !ok {
		// Try mention from message
		if len(m.Mentions) > 0 {
			targetID = m.Mentions[0].ID
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không tìm thấy người dùng. Dùng @mention hoặc ID.", 5*time.Second)
			return
		}
	}

	role := models.RoleAdmin
	verb := "cấp quyền Admin"
	if removing {
		role = models.RoleUser
		verb = "thu hồi quyền Admin"
	}

	// Bảo vệ Owner tuyệt đối
	targetRole := b.GetRole(ctx, s, m.GuildID, targetID, nil)
	if targetRole == RoleOwner {
		log.Printf("[PERMISSION_DENIED] reason=cannot_modify_env_owner actorId=%s targetId=%s", m.Author.ID, targetID)
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Người này là Owner hệ thống, không thể thay đổi quyền.", 5*time.Second)
		return
	}

	if err := b.userRepo.SetRole(ctx, targetID, role); err != nil {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi khi cập nhật quyền.", 5*time.Second)
		return
	}

	log.Printf("[PERMISSION_CHANGE] actorId=%s targetId=%s requestedRole=%s action=cmd_admin result=success", m.Author.ID, targetID, role)
	s.ChannelMessageSend(m.ChannelID,
		ui.Emojis.Success+" Đã "+verb+" cho <@"+targetID+">.")
}

// !limit on  → enable betting limits
// !limit off → disable betting limits
func (b *Bot) cmdLimit(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleOwner {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Owner mới có thể dùng lệnh này.", 3*time.Second)
		return
	}

	if len(args) == 0 || (args[0] != "on" && args[0] != "off") {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"limit on` hoặc `"+b.GetPrefix(ctx)+"limit off`", 5*time.Second)
		return
	}

	cfg, err := b.configRepo.Get(ctx)
	if err != nil {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi lấy config.", 5*time.Second)
		return
	}

	cfg.IsLimitEnabled = args[0] == "on"
	if err := b.configRepo.Save(ctx, cfg); err != nil {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi lưu config.", 5*time.Second)
		return
	}

	state := "BẬT"
	if !cfg.IsLimitEnabled {
		state = "TẮT"
	}
	s.ChannelMessageSend(m.ChannelID, ui.Emojis.Success+" Giới hạn cược đã được **"+state+"**.")
}

// !whitelist add #channel    → whitelist a channel
// !whitelist remove #channel → remove from whitelist
func (b *Bot) cmdWhitelist(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleOwner {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Owner mới có thể dùng lệnh này.", 3*time.Second)
		return
	}

	if len(args) == 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"whitelist add #kênh`, `"+b.GetPrefix(ctx)+"whitelist remove #kênh`, `"+b.GetPrefix(ctx)+"whitelist list` hoặc `"+b.GetPrefix(ctx)+"whitelist clear`", 5*time.Second)
		return
	}

	action := strings.ToLower(args[0])
	if action != "add" && action != "remove" && action != "list" && action != "clear" {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Action phải là `add`, `remove`, `list` hoặc `clear`.", 5*time.Second)
		return
	}

	if action == "list" {
		cfg, err := b.configRepo.Get(ctx)
		if err != nil {
			log.Printf("[Whitelist] Lỗi tải cấu hình: %v", err)
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi tải danh sách.", 5*time.Second)
			return
		}
		if len(cfg.WhitelistedChannels) == 0 {
			s.ChannelMessageSend(m.ChannelID, ui.Emojis.Empty+" Hiện chưa có kênh nào trong whitelist. Bot sẽ hoạt động ở TẤT CẢ các kênh.")
			return
		}
		var wl []string
		for _, ch := range cfg.WhitelistedChannels {
			wl = append(wl, fmt.Sprintf("<#%s> (`%s`)", ch, ch))
		}
		s.ChannelMessageSend(m.ChannelID, ui.Emojis.Success+" **Các kênh đang được phép hoạt động:**\n"+strings.Join(wl, "\n"))
		return
	}

	if action == "clear" {
		cfg, err := b.configRepo.Get(ctx)
		if err == nil {
			cfg.WhitelistedChannels = []string{}
			_ = b.configRepo.Save(ctx, cfg)
			s.ChannelMessageSend(m.ChannelID, ui.Emojis.Success+" Đã xóa sạch danh sách whitelist bị lỗi. Bot hiện sẽ hoạt động ở tất cả các kênh.")
		}
		return
	}

	var channelID string
	if len(args) < 2 {
		// Foolproof: Nếu không nhập ID, tự động lấy ID của kênh hiện tại
		channelID = m.ChannelID
	} else {
		// Dùng Regex bóc tách ID cực chuẩn (17-20 số)
		matches := regexp.MustCompile(`\d{17,20}`).FindAllString(args[1], -1)
		if len(matches) > 0 {
			channelID = matches[len(matches)-1]
		}
	}

	if channelID == "" {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không tìm thấy ID Kênh hợp lệ.", 5*time.Second)
		return
	}

	var err error
	if action == "add" {
		err = b.configRepo.AddWhitelistedChannel(ctx, channelID)
	} else {
		err = b.configRepo.RemoveWhitelistedChannel(ctx, channelID)
	}

	if err != nil {
		log.Printf("[Whitelist] Lỗi thao tác %s cho kênh %s: %v", action, channelID, err)
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi cập nhật whitelist.", 5*time.Second)
		return
	}

	verb := "thêm vào"
	if action == "remove" {
		verb = "xóa khỏi"
	}
	log.Printf("[Whitelist] Thành công: %s %s", verb, channelID)
	s.ChannelMessageSend(m.ChannelID, ui.Emojis.Success+" Đã "+verb+" whitelist: <#"+channelID+">.")
}

// ─── BẢNG ĐIỀU KHIỂN ADMIN (DEV MENU) ─────────────────────────────────────────

func (b *Bot) cmdAdminMenu(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleAdmin {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Bạn không có quyền mở Bảng Điều Khiển.", 3*time.Second)
		return
	}

	roleName := "ADMIN"
	isOwner := (role == RoleOwner)
	if isOwner {
		roleName = "OWNER"
	}

	embed := adminControlEmbed(b, ctx, roleName, m.Author.ID)
	components := adminMainComponents(isOwner, s)

	_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: components,
	})
	if err != nil {
		fmt.Println("[LỖI Bảng Điều Khiển]:", err)
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không thể bung Bảng điều khiển.\nNguyên nhân: Bot không có quyền dùng 1 trong các Emoji được cấu hình, hãy kiểm tra lại file `emoji.go`.", 10*time.Second)
	}
}

func adminMainComponents(isOwner bool, s *discordgo.Session) []discordgo.MessageComponent {
	var options []discordgo.SelectMenuOption

	if isOwner {
		options = append(options, discordgo.SelectMenuOption{
			Label:       "Tài Sản",
			Value:       "wealth",
			Description: "Quản lý số dư, cộng trừ tiền",
			Emoji:       ui.ParseEmoji(s, ui.Emojis.Bank, ui.Emojis.BankFB),
		})
	}

	options = append(options, discordgo.SelectMenuOption{
		Label:       "Dữ Liệu",
		Value:       "data",
		Description: "Xóa và quản lý lịch sử game",
		Emoji:       ui.ParseEmoji(s, ui.Emojis.KingTX, ui.Emojis.HistoryFB),
	})

	if isOwner {
		options = append(options, discordgo.SelectMenuOption{
			Label:       "Quyền Hạn",
			Value:       "auth",
			Description: "Cấp hoặc thu hồi quyền Admin",
			Emoji:       ui.ParseEmoji(s, ui.Emojis.Crown, ui.Emojis.CrownFB),
		})
		options = append(options, discordgo.SelectMenuOption{
			Label:       "Cấu Hình",
			Value:       "system",
			Description: "Quản lý Limit cược và Whitelist",
			Emoji:       ui.ParseEmoji(s, ui.Emojis.Settings, ui.Emojis.SettingsFB),
		})
		options = append(options, discordgo.SelectMenuOption{
			Label:       "Nguy Hiểm",
			Value:       "danger",
			Description: "Khôi phục cài đặt gốc, Restart",
			Emoji:       ui.ParseEmoji(s, ui.Emojis.Fire, ui.Emojis.FireFB),
		})
		options = append(options, discordgo.SelectMenuOption{
			Label:       "Tín Dụng",
			Value:       "credit",
			Description: "Quản lý hạn mức âm tiền (Debt Limit)",
			Emoji:       ui.ParseEmoji(s, ui.Emojis.Bank, ui.Emojis.BankFB),
		})
	}

	menuRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    "admin:main_menu",
				Placeholder: "Vui lòng chọn danh mục quản trị...",
				Options:     options,
			},
		},
	}

	btnRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: "admin:close",
				Label:    "Đóng Bảng Điều Khiển",
				Style:    discordgo.SecondaryButton,
				Emoji:    ui.ParseEmoji(s, ui.Emojis.Shield, ui.Emojis.ErrorFB),
			},
		},
	}

	return []discordgo.MessageComponent{menuRow, btnRow}
}

func adminCategoryMenu(category string, isOwner bool, s *discordgo.Session) []discordgo.MessageComponent {
	var options []discordgo.SelectMenuOption

	switch category {
	case "wealth":
		options = append(options, discordgo.SelectMenuOption{Label: "Cộng tiền (Give)", Value: "give", Description: "Cộng tiền cho người chơi", Emoji: ui.ParseEmoji(s, ui.Emojis.Give, ui.Emojis.GiveFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Trừ tiền (Take)", Value: "take", Description: "Trừ tiền người chơi", Emoji: ui.ParseEmoji(s, ui.Emojis.Take, ui.Emojis.TakeFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Reset Số Dư User", Value: "wipe_user", Description: "Reset số dư người chơi về 0", Emoji: ui.ParseEmoji(s, ui.Emojis.Trash, ui.Emojis.TrashFB)})
	case "data":
		options = append(options, discordgo.SelectMenuOption{Label: "Xem Thống Kê Tổng Quan", Value: "stats", Description: "Xem tổng tiền và dữ liệu máy chủ", Emoji: ui.ParseEmoji(s, ui.Emojis.History, ui.Emojis.HistoryFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Xóa Lịch Sử Game", Value: "wipe_game", Description: "Xóa dữ liệu thống kê của game", Emoji: ui.ParseEmoji(s, ui.Emojis.Broom, ui.Emojis.BroomFB)})
	case "auth":
		options = append(options, discordgo.SelectMenuOption{Label: "Cấp Quyền Admin (Role)", Value: "role_admin_add", Description: "Thêm Role vào danh sách Admin", Emoji: ui.ParseEmoji(s, ui.Emojis.Crown, ui.Emojis.CrownFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Xóa Quyền Admin (Role)", Value: "role_admin_remove", Description: "Xóa Role khỏi danh sách Admin", Emoji: ui.ParseEmoji(s, ui.Emojis.StartWhiteL, ui.Emojis.BlockFB)})
	case "system":
		options = append(options, discordgo.SelectMenuOption{Label: "Bật Giới Hạn Cược", Value: "limit_on", Description: "Bật Limit cược", Emoji: ui.ParseEmoji(s, ui.Emojis.Success, ui.Emojis.SuccessFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Tắt Giới Hạn Cược", Value: "limit_off", Description: "Tắt Limit cược", Emoji: ui.ParseEmoji(s, ui.Emojis.Error, ui.Emojis.ErrorFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Whitelist Kênh Này", Value: "wl_add", Description: "Cho phép bot chạy ở kênh này", Emoji: ui.ParseEmoji(s, ui.Emojis.Shield, ui.Emojis.ShieldFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Xóa Whitelist Kênh Này", Value: "wl_remove", Description: "Cấm bot chạy ở kênh này", Emoji: ui.ParseEmoji(s, ui.Emojis.Fire, ui.Emojis.FireFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Chỉnh Prefix Lệnh", Value: "set_prefix", Description: "Đổi dấu lệnh (VD: !, ?, /, $)", Emoji: ui.ParseEmoji(s, ui.Emojis.Settings, ui.Emojis.SettingsFB)})
	case "danger":
		options = append(options, discordgo.SelectMenuOption{Label: "Khôi Phục Cài Đặt Gốc", Value: "wipe_all", Description: "Xóa TOÀN BỘ dữ liệu (Tiền & Lịch sử)", Emoji: ui.ParseEmoji(s, ui.Emojis.Fire, ui.Emojis.FireFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Khởi Động Lại Bot", Value: "restart", Description: "Restart tiến trình máy chủ", Emoji: ui.ParseEmoji(s, ui.Emojis.Restart, ui.Emojis.RestartFB)})
	case "credit":
		options = append(options, discordgo.SelectMenuOption{Label: "Cấp Hạn Mức Âm Tiền", Value: "set_debt", Description: "Cấp cho 1 User quyền đánh nợ", Emoji: ui.ParseEmoji(s, ui.Emojis.Give, ui.Emojis.GiveFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Xóa Hạn Mức", Value: "remove_debt", Description: "Tắt quyền đánh nợ của User", Emoji: ui.ParseEmoji(s, ui.Emojis.Take, ui.Emojis.TakeFB)})
		options = append(options, discordgo.SelectMenuOption{Label: "Cấu hình Role Tín Dụng", Value: "set_debt_role", Description: "Gắn hạn mức tự động cho Role", Emoji: ui.ParseEmoji(s, ui.Emojis.Settings, ui.Emojis.SettingsFB)})
	}

	menuRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{CustomID: "admin:menu", Placeholder: "Chọn thao tác cần thực hiện...", Options: options},
		},
	}
	btnRow := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: "admin:back",
				Label:    "Quay Lại",
				Style:    discordgo.SecondaryButton,
				Emoji:    ui.ParseEmoji(s, ui.Emojis.FinishTX, ui.Emojis.FinishTXFB),
			},
		},
	}
	return []discordgo.MessageComponent{menuRow, btnRow}
}

func (b *Bot) handleAdminComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !HasAdminAccess(b, ctx, s, i) {
		replyEphemeral(s, i, ui.Emojis.Block+" Bạn không có quyền.")
		return
	}

	userID := interactionUserID(i)
	actorRole := b.GetRole(ctx, s, i.GuildID, userID, i.Member)
	isOwner := (actorRole == RoleOwner)

	data := i.MessageComponentData()

	// Nút Đóng Panel
	if data.CustomID == "admin:close" {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
		if err != nil {
			log.Printf("[ADMIN_PANEL] failed to defer close action: %v", err)
			return
		}
		s.ChannelMessageDelete(i.ChannelID, i.Message.ID)
		return
	}

	// Nút Quay Lại Main Panel hoặc Open Dev
	if data.CustomID == "admin:back" || data.CustomID == "admin:open_dev" {
		roleName := "ADMIN"
		if isOwner {
			roleName = "OWNER"
		}
		embed := adminControlEmbed(b, ctx, roleName, userID)
		comps := adminMainComponents(isOwner, s)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: comps,
			},
		})
		if err != nil {
			log.Printf("[ADMIN_PANEL] failed to back menu: %v", err)
		}
		return
	}

	// Bấm vào danh mục Menu Chính
	if data.CustomID == "admin:main_menu" && len(data.Values) > 0 {
		category := data.Values[0]

		if !isOwner && category != "data" {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được truy cập danh mục này.")
			return
		}

		comps := adminCategoryMenu(category, isOwner, s)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Components: comps,
			},
		})
		return
	}

	if data.CustomID == "admin:menu" && len(data.Values) > 0 {
		val := data.Values[0]

		switch val {
		case "stats":
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
			})

			entries, _ := b.userRepo.GetBalanceLeaderboard(ctx, 5000)
			var totalCoins int64
			for _, e := range entries {
				totalCoins += e.Balance
			}

			msg := fmt.Sprintf("%s **THỐNG KÊ DỮ LIỆU MÁY CHỦ (Top 5000)**\n\n", ui.Emojis.History)
			msg += fmt.Sprintf("👥 Tổng số tài khoản: **%d**\n", len(entries))
			msg += fmt.Sprintf("💰 Tổng tiền lưu thông: **%s** %s\n\n", formatCoins(totalCoins), ui.Emojis.Coin)
			msg += fmt.Sprintf("%s *Audit Log (Lịch sử thao tác quản trị) hiện đang được lưu trực tiếp vào cửa sổ Console/Terminal của Server để bảo mật tuyệt đối.*", ui.Emojis.Shield)

			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: msg,
				Flags:   discordgo.MessageFlagsEphemeral,
			})

		case "give", "take":
			if !isOwner {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
				return
			}
			title := "Cộng Tiền (Give)"
			if val == "take" {
				title = "Trừ Tiền (Take)"
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:" + val,
					Title:    title,
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "target",
								Label:       "ID Người Nhận (Raw ID)",
								Style:       discordgo.TextInputShort,
								Placeholder: "Ví dụ: 1053382061018320987",
								Required:    true,
							},
						}},
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "amount",
								Label:       "Số Tiền",
								Style:       discordgo.TextInputShort,
								Placeholder: "Ví dụ: 1000000",
								Required:    true,
							},
						}},
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "reason",
								Label:       "Lý Do (Audit Log)",
								Style:       discordgo.TextInputShort,
								Placeholder: "VD: Trả thưởng event, Đền bù bug...",
								Required:    true,
							},
						}},
					},
				},
			})

		case "wipe_user":
			if !isOwner {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:wipe_user",
					Title:    "Xóa Dữ Liệu Số Dư",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "target",
								Label:       "ID Người Dùng Cần Reset (Raw ID)",
								Style:       discordgo.TextInputShort,
								Placeholder: "Ví dụ: 1053382061018320987",
								Required:    true,
							},
						}},
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "reason",
								Label:       "Lý Do (Audit Log)",
								Style:       discordgo.TextInputShort,
								Placeholder: "VD: Phạt cheat game...",
								Required:    true,
							},
						}},
					},
				},
			})

		case "wipe_game":
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:wipe_game",
					Title:    "Xóa Lịch Sử Game",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "game_name",
								Label:       "Tên Game (all / taixiu / xidach...)",
								Style:       discordgo.TextInputShort,
								Placeholder: "Ví dụ: all, taixiu, baucua, baicao",
								Required:    true,
							},
						}},
					},
				},
			})

		case "wipe_all":
			if !isOwner {
				replyEphemeral(s, i, "Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:wipe_all",
					Title:    "XÓA TOÀN BỘ HỆ THỐNG",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "confirm",
								Label:       "Nhập 'XACNHAN' để xóa sạch dữ liệu",
								Style:       discordgo.TextInputShort,
								Placeholder: "Gõ XACNHAN vào đây (Không thể hoàn tác)",
								Required:    true,
							},
						}},
					},
				},
			})

		case "role_admin_add", "role_admin_remove":
			if !isOwner {
				replyEphemeral(s, i, "Chỉ Owner mới được dùng chức năng này.")
				return
			}
			title := "Cấp Quyền Admin Cho Role"
			if val == "role_admin_remove" {
				title = "Thu Hồi Quyền Admin Của Role"
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:" + val,
					Title:    title,
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "target",
								Label:       "Nhập Role ID (Raw ID)",
								Style:       discordgo.TextInputShort,
								Placeholder: "Ví dụ: 123456789012345678",
								Required:    true,
							},
						}},
					},
				},
			})

		case "limit_on", "limit_off":
			if !isOwner {
				replyEphemeral(s, i, "Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredMessageUpdate,
			})
			cfg, _ := b.configRepo.Get(ctx)
			cfg.IsLimitEnabled = (val == "limit_on")
			_ = b.configRepo.Save(ctx, cfg)
			state := "BẬT"
			if !cfg.IsLimitEnabled {
				state = "TẮT"
			}
			writeAdminAuditLog(userID, val, "system", "system", true, "Changed bet limit state")
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "Giới hạn cược đã được **" + state + "**.", Flags: discordgo.MessageFlagsEphemeral})

		case "wl_add":
			if !isOwner {
				replyEphemeral(s, i, " Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			_ = b.configRepo.AddWhitelistedChannel(ctx, i.ChannelID)
			writeAdminAuditLog(userID, val, "channel", i.ChannelID, true, "Whitelist channel")
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "Đã thêm whitelist kênh <#" + i.ChannelID + ">.", Flags: discordgo.MessageFlagsEphemeral})

		case "wl_remove":
			if !isOwner {
				replyEphemeral(s, i, " Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			_ = b.configRepo.RemoveWhitelistedChannel(ctx, i.ChannelID)
			writeAdminAuditLog(userID, val, "channel", i.ChannelID, true, "Remove whitelist channel")
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "Đã xóa whitelist kênh <#" + i.ChannelID + ">.", Flags: discordgo.MessageFlagsEphemeral})

		case "set_prefix":
			if !isOwner {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:set_prefix",
					Title:    "Cấu Hình Prefix Lệnh",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "prefix",
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

		case "restart":
			if !isOwner {
				replyEphemeral(s, i, " Chỉ Owner mới được khởi động lại Bot.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			writeAdminAuditLog(userID, val, "system", "bot", true, "Manual restart triggered")
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "Đang tắt tiến trình... Bot sẽ sống lại ngay!", Flags: discordgo.MessageFlagsEphemeral})
			go func() {
				time.Sleep(2 * time.Second)
				os.Exit(0) // Render/Docker sẽ tự động auto-restart khi process exit
			}()

		case "set_debt", "remove_debt":
			if !isOwner {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
				return
			}
			title := "Cấp Hạn Mức Tín Dụng"
			if val == "remove_debt" {
				title = "Xóa Hạn Mức Tín Dụng"
			}

			comps := []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{CustomID: "target", Label: "ID Người Dùng", Style: discordgo.TextInputShort, Required: true},
				}},
			}
			if val == "set_debt" {
				comps = append(comps, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{CustomID: "amount", Label: "Số Tiền Hạn Mức (Tối Đa Được Âm)", Style: discordgo.TextInputShort, Required: true},
				}})
				comps = append(comps, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{CustomID: "role", Label: "ID Role Kèm Theo (Tùy chọn, để trống bỏ qua)", Style: discordgo.TextInputShort, Required: false},
				}})
			}

			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID:   "admin:modal:" + val,
					Title:      title,
					Components: comps,
				},
			})

		case "set_debt_role":
			if !isOwner {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
				return
			}
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseModal,
				Data: &discordgo.InteractionResponseData{
					CustomID: "admin:modal:set_debt_role",
					Title:    "Cấu Hình Tín Dụng Cho Role",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{CustomID: "role_id", Label: "ID Role", Style: discordgo.TextInputShort, Required: true},
						}},
						discordgo.ActionsRow{Components: []discordgo.MessageComponent{
							discordgo.TextInput{CustomID: "amount", Label: "Hạn mức (Nhập 0 để xóa)", Style: discordgo.TextInputShort, Required: true},
						}},
					},
				},
			})
		}
	}
}

func (b *Bot) handleAdminModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// DEFER NGAY LẬP TỨC ĐỂ TRÁNH LỖI "INTERACTION FAILED" KHI TÁC VỤ XỬ LÝ LÂU (>3s)
	errDefer := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if errDefer != nil {
		log.Printf("[ADMIN_MODAL] failed to defer interaction: %v", errDefer)
		return
	}

	if !HasAdminAccess(b, ctx, s, i) {
		followupEphemeral(s, i, " Bạn không có quyền.")
		return
	}
	adminID := interactionUserID(i)
	actorRole := b.GetRole(ctx, s, i.GuildID, adminID, i.Member)
	isOwner := (actorRole == RoleOwner)

	data := i.ModalSubmitData()
	action := strings.TrimPrefix(data.CustomID, "admin:modal:")

	var targetID, amountStr, reason, roleStr string
	for _, c := range data.Components {
		row := c.(*discordgo.ActionsRow)
		input := row.Components[0].(*discordgo.TextInput)
		if input.CustomID == "target" || input.CustomID == "role_id" {
			targetID = strings.TrimSpace(input.Value)
		} else if input.CustomID == "amount" {
			amountStr = strings.TrimSpace(input.Value)
		} else if input.CustomID == "reason" {
			reason = strings.TrimSpace(input.Value)
		} else if input.CustomID == "role" {
			roleStr = strings.TrimSpace(input.Value)
		} else if input.CustomID == "prefix" {
			amountStr = strings.TrimSpace(input.Value) // Tái sử dụng để lưu prefix
		}
	}

	if action == "role_admin_add" {
		var roleID string
		for _, char := range targetID {
			if char >= '0' && char <= '9' {
				roleID += string(char)
			}
		}
		if roleID == "" {
			followupEphemeral(s, i, ui.Emojis.Error+" ID Role không hợp lệ.")
			return
		}
		_ = b.configRepo.AddAdminRole(ctx, roleID)
		writeAdminAuditLog(adminID, "role_admin_add", "role", roleID, true, "Added to Admin Roles")
		followupEphemeral(s, i, fmt.Sprintf("%s Đã cấp quyền truy cập bảng Admin cho thành viên có Role `<@&%s>`.", ui.Emojis.Success, roleID))
		return
	} else if action == "role_admin_remove" {
		var roleID string
		for _, char := range targetID {
			if char >= '0' && char <= '9' {
				roleID += string(char)
			}
		}
		if roleID == "" {
			followupEphemeral(s, i, ui.Emojis.Error+" ID Role không hợp lệ.")
			return
		}
		_ = b.configRepo.RemoveAdminRole(ctx, roleID)
		writeAdminAuditLog(adminID, "role_admin_remove", "role", roleID, true, "Removed from Admin Roles")
		followupEphemeral(s, i, fmt.Sprintf("%s Đã thu hồi quyền Admin của Role `<@&%s>`.", ui.Emojis.Success, roleID))
		return
	}

	if action == "set_prefix" {
		if !isOwner {
			followupEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}
		newPrefix := amountStr
		cfg, _ := b.configRepo.Get(ctx)
		cfg.Prefix = newPrefix
		_ = b.configRepo.Save(ctx, cfg)
		writeAdminAuditLog(adminID, "set_prefix", "system", newPrefix, true, "Changed command prefix")
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã đổi Prefix lệnh thành: `%s`", ui.Emojis.Success, newPrefix))
		return
	}

	if action == "wipe_all" {
		var confirmStr string
		for _, c := range data.Components {
			row := c.(*discordgo.ActionsRow)
			input := row.Components[0].(*discordgo.TextInput)
			if input.CustomID == "confirm" {
				confirmStr = strings.TrimSpace(input.Value)
			}
		}
		if confirmStr != "XACNHAN" {
			followupEphemeral(s, i, ui.Emojis.Error+" Xác nhận không hợp lệ. Đã hủy thao tác.")
			return
		}

		log.Printf("[ADMIN_RESET] received custom_id=%s user_id=%s", data.CustomID, adminID)

		// Tách riêng context 5 phút để quá trình vòng lặp Wipe qua hàng ngàn user không bị Timeout
		wipeCtx, wipeCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer wipeCancel()

		log.Printf("[ADMIN_RESET] discovered balance source db=%s collection=users", b.userRepo.GetDBName())

		gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
		for _, g := range gamesList {
			_ = b.casinoConfigSvc.ResetToDefault(wipeCtx, g, adminID)
		}
		log.Printf("[ADMIN_RESET] wiping collection=configs reset=4")

		delHist, err := b.userRepo.ClearAllGameHistories(wipeCtx)
		log.Printf("[ADMIN_RESET] wiping collection=game_history deleted=%d err=%v", delHist, err)

		delTx, err := b.userRepo.ClearAllTransactions(wipeCtx)
		log.Printf("[ADMIN_RESET] wiping collection=transaction_logs deleted=%d err=%v", delTx, err)

		modCount, err := b.userRepo.ResetAllBalances(wipeCtx)
		log.Printf("[ADMIN_RESET] wiping collection=users (field: balance) modified=%d err=%v", modCount, err)
		log.Printf("[ADMIN_RESET] no cache layer to clear for balances")

		log.Printf("[ADMIN_RESET] reset service completed")

		verifyBal, err := b.wallet.GetBalance(wipeCtx, adminID)
		balVal := int64(0)
		if verifyBal != nil {
			balVal = verifyBal.Balance
		}
		log.Printf("[ADMIN_RESET_VERIFY] user_id=%s balance_after_reset=%v err=%v", adminID, balVal, err)

		if balVal == 0 {
			writeAdminAuditLog(adminID, "wipe_all", "system", "all", true, "Full factory reset verified")
		} else {
			writeAdminAuditLog(adminID, "wipe_all", "system", "all", false, "Balance verification failed")
		}
		followupEphemeral(s, i, "Thành công!")
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã khôi phục cài đặt gốc! Toàn bộ lịch sử game và số dư hệ thống đã được xóa sạch.", ui.Emojis.Success))
		return
	}

	if action == "wipe_game" {
		var gameName string
		for _, c := range data.Components {
			row := c.(*discordgo.ActionsRow)
			input := row.Components[0].(*discordgo.TextInput)
			if input.CustomID == "game_name" {
				gameName = strings.TrimSpace(strings.ToLower(input.Value))
			}
		}
		if gameName == "xd" {
			gameName = "xidach"
		} else if gameName == "bc" {
			gameName = "baucua"
		} else if gameName == "bcao" {
			gameName = "baicao"
		}

		if gameName == "all" {
			gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
			for _, g := range gamesList {
				_ = b.userRepo.ClearGameHistory(ctx, g)
			}
			writeAdminAuditLog(adminID, "wipe_game", "game", "all", true, "Cleared all game histories")
			replyEphemeral(s, i, fmt.Sprintf("%s Đã xóa toàn bộ lịch sử dữ liệu của TẤT CẢ các game!", ui.Emojis.Success))
			return
		}

		if gameName != "taixiu" && gameName != "xidach" && gameName != "baucua" && gameName != "baicao" {
			replyEphemeral(s, i, ui.Emojis.Error+" Tên game không hợp lệ (all, taixiu, xidach, baucua, baicao).")
			return
		}

		_ = b.userRepo.ClearGameHistory(ctx, gameName)
		writeAdminAuditLog(adminID, "wipe_game", "game", gameName, true, "Cleared specific game history")
		replyEphemeral(s, i, fmt.Sprintf("%s Đã xóa toàn bộ lịch sử dữ liệu của game **%s**!", ui.Emojis.Success, gameName))
		return
	}

	if action == "wipe_user" {
		if !isOwner {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}
		bal, err := b.wallet.GetBalance(ctx, targetID)
		if err != nil {
			replyEphemeral(s, i, ui.Emojis.Error+" Không tìm thấy thông tin tài khoản của user này.")
			return
		}

		// Cấm Reset tài khoản của Owner
		if b.GetRole(ctx, s, i.GuildID, targetID, nil) == RoleOwner {
			replyEphemeral(s, i, ui.Emojis.Error+" Không thể tác động lên số dư của Owner hệ thống qua Panel.")
			return
		}

		if bal.Balance > 0 {
			if err := b.wallet.AdminTake(ctx, targetID, adminID, bal.Balance); err != nil {
				replyEphemeral(s, i, ui.Emojis.Error+" Lỗi: "+friendlyErr(err))
				return
			}
		} else if bal.Balance < 0 {
			if err := b.wallet.AdminGive(ctx, targetID, adminID, -bal.Balance); err != nil {
				replyEphemeral(s, i, ui.Emojis.Error+" Lỗi: "+friendlyErr(err))
				return
			}
		}
		writeAdminAuditLog(adminID, "wipe_user", "user", targetID, true, reason)
		replyEphemeral(s, i, "Thành công!")
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã reset toàn bộ số dư của <@%s> về 0.", ui.Emojis.Success, targetID))
		return
	}

	if action == "set_debt" || action == "remove_debt" {
		if !isOwner {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}

		amount := int64(0)
		if action == "set_debt" {
			amount, _ = strconv.ParseInt(amountStr, 10, 64)
			if amount <= 0 {
				replyEphemeral(s, i, ui.Emojis.Error+" Hạn mức phải lớn hơn 0.")
				return
			}
		}

		// Cập nhật DB (Giả định hàm b.userRepo.SetDebtLimit tồn tại)
		enabled := action == "set_debt"
		_ = b.userRepo.SetDebtLimit(ctx, targetID, enabled, amount, roleStr, adminID)

		if enabled && roleStr != "" && i.GuildID != "" {
			err := s.GuildMemberRoleAdd(i.GuildID, targetID, roleStr)
			if err != nil {
				writeAdminAuditLog(adminID, action, "user", targetID, false, fmt.Sprintf("Set Debt %d but Role Assignment Failed", amount))
				s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã lưu hạn mức âm %s cho <@%s>, **nhưng bot thiếu quyền cấp Role!**", ui.Emojis.Warning, formatCoins(amount), targetID))
				replyEphemeral(s, i, "Thành công 1 phần.")
				return
			}
		}

		writeAdminAuditLog(adminID, action, "user", targetID, true, fmt.Sprintf("Set Debt %d Enabled: %v", amount, enabled))
		replyEphemeral(s, i, "Thành công!")
		status := "Đã BẬT"
		if !enabled {
			status = "Đã TẮT"
		}
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s %s hạn mức tín dụng cho <@%s>. Mức âm tối đa: %s.", ui.Emojis.Success, status, targetID, coinDisplay(amount)))
		return
	}

	if action == "set_debt_role" {
		if !isOwner {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}

		amount, _ := strconv.ParseInt(amountStr, 10, 64)
		// Giả định configRepo.SetDebtRoleLimit tồn tại
		_ = b.configRepo.SetDebtRoleLimit(ctx, targetID, amount)
		writeAdminAuditLog(adminID, "set_debt_role", "role", targetID, true, fmt.Sprintf("Limit: %d", amount))
		replyEphemeral(s, i, "Thành công!")
		if amount > 0 {
			s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã gắn hạn mức tín dụng %s cho Role `<@&%s>`.", ui.Emojis.Success, coinDisplay(amount), targetID))
		} else {
			s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã XÓA cấu hình tín dụng của Role `<@&%s>`.", ui.Emojis.Success, targetID))
		}
		return
	}

	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amount <= 0 {
		replyEphemeral(s, i, ui.Emojis.Error+" Số tiền không hợp lệ.")
		return
	}

	if action == "give" {
		if !isOwner {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}
		if err := b.wallet.AdminGive(ctx, targetID, adminID, amount); err != nil {
			replyEphemeral(s, i, ui.Emojis.Error+" Lỗi: "+friendlyErr(err))
			return
		}
		writeAdminAuditLog(adminID, "give", "user", targetID, true, fmt.Sprintf("Amount: %d, Reason: %s", amount, reason))
		replyEphemeral(s, i, "Thành công!")
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã cộng %s cho <@%s>", ui.Emojis.Success, coinDisplay(amount), targetID))
	} else if action == "take" {
		if !isOwner {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được dùng chức năng này.")
			return
		}
		if err := b.wallet.AdminTake(ctx, targetID, adminID, amount); err != nil {
			replyEphemeral(s, i, ui.Emojis.Error+" Lỗi: "+friendlyErr(err))
			return
		}
		writeAdminAuditLog(adminID, "take", "user", targetID, true, fmt.Sprintf("Amount: %d, Reason: %s", amount, reason))
		replyEphemeral(s, i, "Thành công!")
		s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("%s Đã trừ %s của <@%s>", ui.Emojis.Success, coinDisplay(amount), targetID))
	}
}

// !removelist [game] → Xóa danh sách leaderboard
func (b *Bot) cmdRemoveList(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Kiểm tra quyền Admin/Owner
	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleAdmin {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Admin mới có thể xóa danh sách.", 3*time.Second)
		return
	}

	if len(args) == 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"removelist [game]` hoặc `"+b.GetPrefix(ctx)+"removelist [game] [@user]`", 5*time.Second)
		return
	}

	gameName := strings.ToLower(args[0])
	if gameName != "taixiu" && gameName != "xd" && gameName != "xidach" && gameName != "bc" && gameName != "baucua" && gameName != "bcao" && gameName != "baicao" {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Tên game không hợp lệ.", 5*time.Second)
		return
	}
	if gameName == "xd" {
		gameName = "xidach"
	} else if gameName == "bc" {
		gameName = "baucua"
	} else if gameName == "bcao" {
		gameName = "baicao"
	}

	// Nếu có tham số thứ 2, xóa riêng user đó
	if len(args) > 1 {
		targetID, ok := ExtractUserID(args[1])
		if !ok && len(m.Mentions) > 0 {
			targetID = m.Mentions[0].ID
		}
		if targetID != "" {
			_ = b.userRepo.ClearUserGameHistory(ctx, gameName, targetID)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s Đã xóa lịch sử game **%s** của <@%s>!", ui.Emojis.Success, gameName, targetID))
			return
		}
	}

	_ = b.userRepo.ClearGameHistory(ctx, gameName)

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s Đã xóa toàn bộ danh sách dữ liệu của game **%s**!", ui.Emojis.Success, gameName))
}

// !debug_channel_config — Xem Data thực tế & Tự động chuẩn hóa Whitelist
func (b *Bot) cmdDebugChannel(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleOwner {
		return
	}

	cfg, err := b.configRepo.Get(ctx)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Lỗi lấy DB: "+err.Error())
		return
	}

	var cleanList []string
	for _, id := range cfg.WhitelistedChannels {
		cleanID, _ := ExtractUserID(id) // Dùng lại regex có sẵn
		if cleanID == "" {              // Fallback regex backup
			cleanID = strings.TrimSpace(id)
		}
		cleanList = append(cleanList, cleanID)
	}

	msg := fmt.Sprintf("```ini\n[DEBUG CHANNEL INFO]\nGuild_ID: %s\nChannel_ID: %s\n\n[DB RAW WHITELIST]\n%#v\n\n[AUTO-MIGRATION]\nĐã quét và lọc rác thành công. Vui lòng gọi lại lệnh '%swhitelist list' để kiểm tra.```", m.GuildID, m.ChannelID, cfg.WhitelistedChannels, b.GetPrefix(ctx))
	cfg.WhitelistedChannels = cleanList
	_ = b.configRepo.Save(ctx, cfg) // Lưu lại bản sạch

	s.ChannelMessageSend(m.ChannelID, msg)
}

// !resetall → Xóa toàn bộ dữ liệu (Tiền, Lịch sử game) - Chỉ Owner
func (b *Bot) cmdResetAll(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleOwner {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Owner mới có thể dùng lệnh này.", 3*time.Second)
		return
	}

	if len(args) == 0 || args[0] != "XACNHAN" {
		sendTemp(s, m.ChannelID, ui.Emojis.Warning+" CẢNH BÁO NGUY HIỂM: Lệnh này sẽ xóa SẠCH toàn bộ số dư của tất cả user và toàn bộ lịch sử game.\nĐể tiếp tục, hãy gõ: `"+b.GetPrefix(ctx)+"resetall XACNHAN`", 10*time.Second)
		return
	}

	log.Printf("[ADMIN_RESET] received command=!resetall user_id=%s", m.Author.ID)

	wipeCtx, wipeCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer wipeCancel()

	log.Printf("[ADMIN_RESET] discovered balance source db=%s collection=users", b.userRepo.GetDBName())

	gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
	for _, g := range gamesList {
		_ = b.casinoConfigSvc.ResetToDefault(wipeCtx, g, m.Author.ID)
	}
	log.Printf("[ADMIN_RESET] wiping collection=configs reset=4")

	delHist, err := b.userRepo.ClearAllGameHistories(wipeCtx)
	log.Printf("[ADMIN_RESET] wiping collection=game_history deleted=%d err=%v", delHist, err)

	delTx, err := b.userRepo.ClearAllTransactions(wipeCtx)
	log.Printf("[ADMIN_RESET] wiping collection=transaction_logs deleted=%d err=%v", delTx, err)

	modCount, err := b.userRepo.ResetAllBalances(wipeCtx)
	log.Printf("[ADMIN_RESET] wiping collection=users (field: balance) modified=%d err=%v", modCount, err)
	log.Printf("[ADMIN_RESET] no cache layer to clear for balances")

	log.Printf("[ADMIN_RESET] reset service completed")

	verifyBal, err := b.wallet.GetBalance(wipeCtx, m.Author.ID)
	balVal := int64(0)
	if verifyBal != nil {
		balVal = verifyBal.Balance
	}
	log.Printf("[ADMIN_RESET_VERIFY] user_id=%s balance_after_reset=%v err=%v", m.Author.ID, balVal, err)

	if balVal == 0 {
		writeAdminAuditLog(m.Author.ID, "wipe_all", "system", "all", true, "Full factory reset verified")
	} else {
		writeAdminAuditLog(m.Author.ID, "wipe_all", "system", "all", false, "Balance verification failed")
	}

	s.ChannelMessageSend(m.ChannelID, ui.Emojis.Success+" Đã khôi phục cài đặt gốc thành công! Toàn bộ số dư và lịch sử game hệ thống đã bị xóa.")
}

// Xử lý lệnh slash /reset với giao diện hiện đại và các options
func (b *Bot) handleReset(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminID := interactionUserID(i)
	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return
	}
	sub := opts[0]

	switch sub.Name {
	case "all":
		actorRole := b.GetRole(ctx, s, i.GuildID, adminID, i.Member)
		if actorRole < RoleOwner {
			respondEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể dùng chức năng này.")
			return
		}

		var confirm string
		for _, opt := range sub.Options {
			if opt.Name == "confirm" {
				confirm = opt.StringValue()
			}
		}

		if confirm != "XACNHAN" {
			respondEphemeral(s, i, ui.Emojis.Error+" Xác nhận không hợp lệ. Đã hủy thao tác.")
			return
		}

		gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
		for _, g := range gamesList {
			_ = b.userRepo.ClearGameHistory(ctx, g)
		}
		entries, err := b.userRepo.GetBalanceLeaderboard(ctx, 10000)
		if err == nil {
			for _, e := range entries {
				if e.Balance > 0 {
					_ = b.wallet.AdminTake(ctx, e.UserID, adminID, e.Balance)
				}
			}
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: ui.Emojis.Success + " Đã khôi phục cài đặt gốc! Toàn bộ lịch sử game và số dư hệ thống đã được xóa sạch.",
			},
		})

	case "game":
		var gameName string
		var targetUserID string
		for _, opt := range sub.Options {
			if opt.Name == "name" {
				gameName = opt.StringValue()
			} else if opt.Name == "user" && opt.UserValue(s) != nil {
				targetUserID = opt.UserValue(s).ID
			}
		}

		if targetUserID != "" {
			if gameName == "all" {
				gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
				for _, g := range gamesList {
					_ = b.userRepo.ClearUserGameHistory(ctx, g, targetUserID)
				}
				respondEphemeral(s, i, fmt.Sprintf("%s Đã xóa lịch sử TẤT CẢ game của <@%s>!", ui.Emojis.Success, targetUserID))
			} else {
				_ = b.userRepo.ClearUserGameHistory(ctx, gameName, targetUserID)
				respondEphemeral(s, i, fmt.Sprintf("%s Đã xóa lịch sử game **%s** của <@%s>!", ui.Emojis.Success, gameName, targetUserID))
			}
			return
		}

		if gameName == "all" {
			gamesList := []string{"taixiu", "xidach", "baucua", "baicao"}
			for _, g := range gamesList {
				_ = b.userRepo.ClearGameHistory(ctx, g)
			}
			respondEphemeral(s, i, fmt.Sprintf("%s Đã xóa toàn bộ lịch sử dữ liệu của TẤT CẢ các game!", ui.Emojis.Success))
			return
		}

		_ = b.userRepo.ClearGameHistory(ctx, gameName)
		respondEphemeral(s, i, fmt.Sprintf("%s Đã xóa toàn bộ lịch sử dữ liệu của game **%s**!", ui.Emojis.Success, gameName))

	case "user":
		actorRole := b.GetRole(ctx, s, i.GuildID, adminID, i.Member)
		if actorRole < RoleOwner {
			respondEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể dùng chức năng này.")
			return
		}
		var targetID string
		for _, opt := range sub.Options {
			if opt.Name == "target" && opt.UserValue(s) != nil {
				targetID = opt.UserValue(s).ID
			}
		}

		if targetID == "" {
			return
		}

		bal, err := b.wallet.GetBalance(ctx, targetID)
		if err != nil {
			respondEphemeral(s, i, ui.Emojis.Error+" Không tìm thấy thông tin tài khoản của user này.")
			return
		}
		if bal.Balance > 0 {
			if err := b.wallet.AdminTake(ctx, targetID, adminID, bal.Balance); err != nil {
				respondEphemeral(s, i, ui.Emojis.Error+" Lỗi: "+friendlyErr(err))
				return
			}
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("%s Đã reset toàn bộ số dư của <@%s> về 0.", ui.Emojis.Success, targetID),
			},
		})
	}
}

// !restore → Lệnh khôi phục giao diện kết quả cuối cùng bị kẹt do Rate Limit / Cứu hộ bàn lỗi
func (b *Bot) cmdRestore(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	restoredUI := false
	restoredStuck := false

	// 1. Chuyên sâu: Xử lý bàn Xì Dách kẹt trạng thái settlement_failed (Cứu hộ tài sản bị khóa do DB lỗi)
	if b.rescueStuckBlackjackTable(ctx, s, m.ChannelID) {
		restoredStuck = true
	}

	// 2. Kiểm tra Cache UI (Giải quyết nhanh lỗi Rate Limit nuốt thông báo)
	if val, ok := lastChannelResults.Load(m.ChannelID); ok {
		res := val.(CachedGameResult)
		if time.Since(res.Timestamp) < 2*time.Hour { // Hạn sử dụng cache 2 giờ
			msgContent := fmt.Sprintf("%s **[HỆ THỐNG KHÔI PHỤC]** Bảng kết quả **%s** gần nhất:\n%s", ui.Emojis.Success, res.GameName, res.Content)
			_, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
				Content:    msgContent,
				Embed:      res.Embed,
				Components: res.Components,
			})
			if err == nil {
				restoredUI = true
				lastChannelResults.Delete(m.ChannelID) // Xóa cache sau khi khôi phục thành công để chống spam
			}
		}
	}

	if !restoredUI && !restoredStuck {
		msg, err := s.ChannelMessageSend(m.ChannelID, ui.Emojis.Warning+" Không tìm thấy kết quả hoặc bàn chơi lỗi nào cần khôi phục trong kênh này.")
		if err == nil {
			time.AfterFunc(5*time.Second, func() {
				_ = s.ChannelMessageDelete(m.ChannelID, msg.ID)
			})
		}
	}
}
