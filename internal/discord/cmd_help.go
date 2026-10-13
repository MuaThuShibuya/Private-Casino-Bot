package discord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

// HelpCommand defines a single command for the help menu.
type HelpCommand struct {
	Name        string // e.g., "/balance" or "!pay"
	Description string
	MinRole     RoleLevel
}

// HelpCategory defines a category of commands.
type HelpCategory struct {
	Name        string // e.g., "wallet"
	Label       string // e.g., "💰 Ví & Giao Dịch"
	Description string
	Emoji       string // using ui.Emojis string
	Fallback    string // fallback if emoji fails
	Commands    []HelpCommand
}

// getHelpData returns the dynamic help structure containing all registered commands
func getHelpData() []HelpCategory {
	return []HelpCategory{
		{
			Name:        "wallet",
			Label:       "Tài Sản & Giao Dịch",
			Description: "Quản lý ví tiền, chuyển khoản và xem lịch sử",
			Emoji:       ui.Emojis.Bank,
			Fallback:    "💰",
			Commands: []HelpCommand{
				{Name: "/balance [@user]", Description: "Xem số dư ví và bảng xếp hạng phú hộ.", MinRole: RoleUser},
				{Name: "!balance / !cash / !tien", Description: "Xem ví cá nhân; có thể dùng riêng trong DM với bot.", MinRole: RoleUser},
				{Name: "!history [1-20]", Description: "Xem lịch sử giao dịch cá nhân; dùng riêng trong DM với bot.", MinRole: RoleUser},
				{Name: "!pay @user <số tiền>", Description: "Chuyển tiền nhanh (Hỗ trợ mention, ID, link).", MinRole: RoleUser},
				{Name: "/transfer", Description: "Lệnh chuyển tiền an toàn (Slash command).", MinRole: RoleUser},
				{Name: "/history [số lượng]", Description: "Trích xuất lịch sử dòng tiền giao dịch cá nhân.", MinRole: RoleUser},
				{Name: "/gamehistory <game>", Description: "Xem thống kê tổng quan Lời/Lỗ theo từng game.", MinRole: RoleUser},
				{Name: "!swap / !sync", Description: "Đồng bộ tỷ giá ví tiền mặt Cash và OWO.", MinRole: RoleUser},
			},
		},
		{
			Name:        "games",
			Label:       "Trò Chơi Casino",
			Description: "Các lệnh điều khiển bàn cược và tác vụ game",
			Emoji:       ui.Emojis.Dice,
			Fallback:    "🎲",
			Commands: []HelpCommand{
				{Name: "!taixiu / !tx [thời gian] [min] [max] [auto]", Description: "Khởi tạo bàn cược Tài Xỉu tập thể.", MinRole: RoleAdmin},
				{Name: "!baucua / !bc [thời gian] [min] [max] [auto]", Description: "Khởi tạo bàn cược Bầu Cua Tôm Cá.", MinRole: RoleAdmin},
				{Name: "!xidach / !bj [thời gian] [số tiền] [auto]", Description: "Khởi tạo bàn cược Xì Dách đối đầu nhà cái.", MinRole: RoleAdmin},
				{Name: "!baicao / !bcao [số tiền] [auto]", Description: "Khởi tạo bàn cược Bài Cào đền Sáp.", MinRole: RoleAdmin},
				{Name: "!host / /host", Description: "Kéo lại bảng kết quả bàn game đang làm Cái xuống cuối kênh.", MinRole: RoleUser},
				{Name: "!restore", Description: "Cứu hộ khẩn cấp bàn game kẹt lỗi hoặc khôi phục giao diện.", MinRole: RoleUser},
				{Name: "!stop", Description: "Dừng chuỗi Auto Game tạo bàn liên tục đang chạy.", MinRole: RoleUser},
			},
		},
		{
			Name:        "moderation",
			Label:       "Quản Trị Kênh",
			Description: "Công cụ kiểm duyệt tin nhắn và dọn dẹp kênh",
			Emoji:       ui.Emojis.Shield,
			Fallback:    "🛡️",
			Commands: []HelpCommand{
				{Name: "!delete / !clear [số lượng] [--old]", Description: "Xóa hàng loạt tin nhắn dọn dẹp kênh siêu tốc.", MinRole: RoleAdmin},
				{Name: "!delete stop", Description: "Gửi tín hiệu dừng tác vụ dọn dẹp kênh đang chạy.", MinRole: RoleAdmin},
				{Name: "!whitelist add/remove #kênh", Description: "Cho phép hoặc Cấm bot hoạt động tại kênh chỉ định.", MinRole: RoleOwner},
				{Name: "!whitelist list/clear", Description: "Xem danh sách kênh Whitelist hoặc xóa toàn bộ list.", MinRole: RoleOwner},
				{Name: "!removelist [game] [@user]", Description: "Xóa dữ liệu thống kê bảng xếp hạng trò chơi.", MinRole: RoleAdmin},
			},
		},
		{
			Name:        "system",
			Label:       "Hệ Thống & Phân Quyền",
			Description: "Thiết lập quy tắc, cấu hình tỷ lệ và tài chính",
			Emoji:       ui.Emojis.Settings,
			Fallback:    "⚙️",
			Commands: []HelpCommand{
				{Name: "!panel / !dev", Description: "Mở Bảng Điều Khiển Admin (Quản trị Config & System).", MinRole: RoleAdmin},
				{Name: "!limit on/off", Description: "Bật/Tắt giới hạn cược tổng của toàn bộ máy chủ.", MinRole: RoleOwner},
				{Name: "!agency / !daily", Description: "Mở Bảng Quản Lý Đại Lý / Đối tác Sòng bạc.", MinRole: RoleOwner},
				{Name: "/admin give/take", Description: "Bơm hoặc Thu hồi tiền người chơi trực tiếp.", MinRole: RoleOwner},
				{Name: "/reset [all/game/user]", Description: "Admin có thể xóa lịch sử game; reset toàn hệ thống hoặc số dư chỉ dành cho Owner.", MinRole: RoleAdmin},
				{Name: "!admin [@user] / !admin remove @user", Description: "Owner cấp hoặc thu hồi quyền Admin cho User.", MinRole: RoleOwner},
				{Name: "!admin role add/remove @role", Description: "Gắn quyền truy cập Admin hệ thống cho Role Discord.", MinRole: RoleOwner},
			},
		},
		{
			Name:        "owner",
			Label:       "Owner Tools (Dev)",
			Description: "Lệnh vận hành, bảo trì và kiểm soát máy chủ VPS",
			Emoji:       ui.Emojis.Crown,
			Fallback:    "👑",
			Commands: []HelpCommand{
				{Name: "!uptime / !server", Description: "Xem thông tin tiêu thụ tài nguyên VPS và Uptime.", MinRole: RoleOwner},
				{Name: "!render", Description: "Xem trạng thái Render, Gateway, MongoDB, latency và deploy gần nhất.", MinRole: RoleAdmin},
				{Name: "!upd", Description: "Owner yêu cầu Render build/deploy commit mới nhất từ GitHub.", MinRole: RoleOwner},
				{Name: "!reload / !restart", Description: "Khởi động lại tiến trình Bot an toàn.", MinRole: RoleOwner},
				{Name: "!debug_channel_config", Description: "Xem Data thực tế cấu hình kênh và tự động chuẩn hóa.", MinRole: RoleOwner},
				{Name: "!resetall XACNHAN", Description: "WIPE DATABASE: Khôi phục cài đặt gốc TOÀN BỘ hệ thống.", MinRole: RoleOwner},
			},
		},
	}
}

// handleHelp is the entry point for the /help slash command.
func (b *Bot) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)

	embed := buildHelpDashboardEmbed()
	components := b.buildHelpComponents(s, role, "")

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

func (b *Bot) cmdHelp(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)

	embed := buildHelpDashboardEmbed()
	components := b.buildHelpComponents(s, role, "")

	_, _ = s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: components,
	})
}

// handleHelpComponent handles interactions from the help menu (select menu, buttons).
func (b *Bot) handleHelpComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Anti-spam & interaction lock
	lockKey := "help_menu:" + i.Message.ID + ":" + interactionUserID(i)
	if !b.TryLockAction(lockKey, 1*time.Second) {
		respondEphemeral(s, i, ui.Emojis.Loading+" Bảng trợ giúp đang được cập nhật. Vui lòng thử lại.")
		return
	}

	data := i.MessageComponentData()
	if data.CustomID == "help:set_prefix" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)
		if role < RoleOwner {
			respondEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới được đổi prefix.")
			return
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
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
		}); err != nil {
			log.Printf("[Help] Không thể mở modal đổi prefix interaction_id=%s: %v", i.ID, err)
		}
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Printf("[Help] Không thể xác nhận tương tác interaction_id=%s: %v", i.ID, err)
		return
	}

	// Check if the menu is expired
	if time.Since(i.Message.Timestamp) > 10*time.Minute {
		content := ui.Emojis.Warning + " Bảng trợ giúp này đã hết hạn. Vui lòng gõ lệnh lại."
		embeds := []*discordgo.MessageEmbed{}
		components := []discordgo.MessageComponent{}
		if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content:    &content,
			Embeds:     &embeds,
			Components: &components,
		}); err != nil {
			log.Printf("[Help] Không thể cập nhật bảng trợ giúp hết hạn interaction_id=%s: %v", i.ID, err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	role := b.GetRole(ctx, s, i.GuildID, interactionUserID(i), i.Member)

	var embed *discordgo.MessageEmbed
	var selectedCategory string

	switch data.CustomID {
	case "help:select_category":
		if len(data.Values) == 0 {
			embed = view.ErrorEmbed("Vui lòng chọn một danh mục hợp lệ.")
			break
		}
		selectedCategory = data.Values[0]
		embed = buildHelpCategoryEmbed(selectedCategory, role, b.GetPrefix(ctx))
	case "help:back_to_main":
		embed = buildHelpDashboardEmbed()
	default:
		embed = view.ErrorEmbed("Tương tác trợ giúp không hợp lệ.")
	}

	components := b.buildHelpComponents(s, role, selectedCategory)
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}); err != nil {
		log.Printf("[Help] Không thể cập nhật bảng trợ giúp interaction_id=%s: %v", i.ID, err)
	}
}

// buildHelpDashboardEmbed creates the main embed for the help menu.
func buildHelpDashboardEmbed() *discordgo.MessageEmbed {
	embed := view.NewEmbed(
		ui.Emojis.Crown+" TOÀN THƯ TƯƠNG TÁC HỆ THỐNG",
		"Chào mừng đến với **Abyss Casino**!\n"+
			"Vực sâu vẫy gọi những linh hồn dũng cảm. Dưới đây là toàn thư khế ước dành cho ngài.\n\n"+
			ui.Emojis.ArrowBlack+" **Cách tra cứu lệnh:**\n"+
			"Hãy sử dụng Menu lựa chọn bên dưới để lọc và tìm kiếm các lệnh phù hợp với quyền hạn của bạn.",
	)
	embed.Color = 0x1E0B2D // Abyss color
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Mọi lựa chọn đều có cái giá của nó | Chọn một danh mục để bắt đầu"}
	return embed
}

// buildHelpCategoryEmbed creates an embed listing commands for a specific category.
func buildHelpCategoryEmbed(categoryKey string, userRole RoleLevel, prefix string) *discordgo.MessageEmbed {
	var targetCat *HelpCategory
	for _, cat := range getHelpData() {
		if cat.Name == categoryKey {
			targetCat = &cat
			break
		}
	}

	if targetCat == nil {
		return view.ErrorEmbed("Danh mục không hợp lệ.")
	}

	var sb strings.Builder
	count := 0
	for _, cmd := range targetCat.Commands {
		// Filter by permission
		if userRole < cmd.MinRole {
			continue
		}
		count++
		// Replace prefix placeholder safely
		cmdName := cmd.Name
		cmdName = strings.ReplaceAll(cmdName, "!", prefix)
		sb.WriteString(fmt.Sprintf("%s **`%s`**\n> %s\n\n", ui.Emojis.ListDot, cmdName, cmd.Description))
	}

	if count == 0 {
		sb.WriteString(ui.Emojis.Block + " Bạn không có quyền truy cập lệnh nào trong danh mục này.")
	}

	embed := view.NewEmbed(targetCat.Emoji+" "+targetCat.Label, sb.String())
	embed.Color = 0x00FFFF
	embed.Footer = &discordgo.MessageEmbedFooter{Text: targetCat.Description}
	return embed
}

func GetBotInviteURL(s *discordgo.Session) string {
	if s == nil || s.State == nil || s.State.User == nil || s.State.User.ID == "" {
		return ""
	}
	return fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&scope=bot%%20applications.commands",
		s.State.User.ID,
	)
}

// buildHelpComponents creates the select menu and buttons for the help panel.
func (b *Bot) buildHelpComponents(s *discordgo.Session, userRole RoleLevel, selectedCategory string) []discordgo.MessageComponent {
	options := []discordgo.SelectMenuOption{}

	for _, cat := range getHelpData() {
		// Check if user has access to at least one command in this category
		hasAccess := false
		for _, cmd := range cat.Commands {
			if userRole >= cmd.MinRole {
				hasAccess = true
				break
			}
		}
		if !hasAccess {
			continue
		}

		options = append(options, discordgo.SelectMenuOption{
			Label:       cat.Label,
			Value:       cat.Name,
			Description: cat.Description,
			Emoji:       ui.ParseEmoji(s, cat.Emoji, cat.Fallback),
			Default:     selectedCategory == cat.Name,
		})
	}

	if len(options) == 0 {
		return []discordgo.MessageComponent{}
	}

	selectMenu := discordgo.SelectMenu{
		CustomID:    "help:select_category",
		Placeholder: "Chọn một danh mục để xem lệnh...",
		Options:     options,
	}

	// Build buttons
	backButton := discordgo.Button{
		CustomID: "help:back_to_main",
		Label:    "Quay Lại",
		Style:    discordgo.SecondaryButton,
		Emoji:    ui.ParseEmoji(s, ui.Emojis.FinishTX, "◀️"),
		Disabled: selectedCategory == "",
	}
	if selectedCategory == "" {
		backButton.CustomID = "help:set_prefix"
		backButton.Label = "Đổi Prefix"
		backButton.Emoji = ui.ParseEmoji(s, ui.Emojis.Settings, "⚙️")
		backButton.Disabled = userRole < RoleOwner
	}

	var buttonRowComponents []discordgo.MessageComponent
	buttonRowComponents = append(buttonRowComponents, backButton)

	inviteURL := GetBotInviteURL(s)
	if inviteURL != "" {
		inviteButton := discordgo.Button{
			Label: "Mời Abyss Bot",
			Style: discordgo.LinkButton,
			Emoji: ui.ParseEmoji(s, ui.Emojis.Success, "➕"),
			URL:   inviteURL,
		}
		buttonRowComponents = append(buttonRowComponents, inviteButton)
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{selectMenu},
		},
		discordgo.ActionsRow{
			Components: buttonRowComponents,
		},
	}
}
