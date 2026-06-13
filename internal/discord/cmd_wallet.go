package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

// !pay @user [amount]                    → transfer by mention
// !pay discord.com/users/ID [amount]     → transfer by profile URL
// !pay 123456789012345678 [amount]       → transfer by raw ID
func (b *Bot) cmdPay(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if len(args) < 2 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Dùng: `"+b.GetPrefix(ctx)+"pay @user [số tiền]`", 5*time.Second)
		return
	}

	// Resolve target: try mention first, then regex on the string
	toID := ""
	if len(m.Mentions) > 0 {
		toID = m.Mentions[0].ID
	} else {
		var ok bool
		toID, ok = ExtractUserID(args[0])
		if !ok {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không nhận ra người dùng. Dùng @mention, link profile, hoặc ID.", 5*time.Second)
			return
		}
	}

	amount, err := strconv.ParseInt(args[len(args)-1], 10, 64)
	if err != nil || amount <= 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Số tiền không hợp lệ.", 5*time.Second)
		return
	}

	if toID == m.Author.ID {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không thể chuyển tiền cho chính mình.", 5*time.Second)
		return
	}

	// Check target is not a bot
	if targetUser, err := s.User(toID); err == nil && targetUser.Bot {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không thể chuyển tiền cho bot.", 5*time.Second)
		return
	}

	// Dùng Message ID làm Idempotency Key để chống spam lệnh
	if err := b.wallet.Transfer(ctx, m.Author.ID, toID, amount, m.ID); err != nil {
		var errMsg string
		switch err {
		case repository.ErrInsufficientBalance:
			errMsg = "Số dư không đủ."
		case repository.ErrInvalidAmount:
			errMsg = "Số tiền không hợp lệ."
		default:
			errMsg = "Lỗi hệ thống, vui lòng thử lại."
		}
		msg, _ := s.ChannelMessageSend(m.ChannelID, ui.Emojis.Error+" "+errMsg)
		if msg != nil {
			deleteAfter(s, m.ChannelID, msg.ID, 5*time.Second)
		}
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n\n", ui.Emojis.Header))
	sb.WriteString(fmt.Sprintf("%s **Kẻ trao:** <@%s>\n", ui.Emojis.ArrowBlack, m.Author.ID))
	sb.WriteString(fmt.Sprintf("%s **Kẻ nhận:** <@%s>\n", ui.Emojis.ArrowBlack, toID))
	sb.WriteString(fmt.Sprintf("> %s **Tài sản dịch chuyển:** %s\n\n", ui.Emojis.Coin, coinDisplay(amount)))
	sb.WriteString("_Vực sâu đã chứng giám sự trao đổi này._")

	embed := view.NewEmbed(ui.Emojis.Bank+" KHẾ ƯỚC CHUYỂN GIAO", sb.String())
	embed.Color = 0x00FFFF // Xanh cyan huyền ảo
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Sổ cái vực sâu đã lưu vết. | " + GetRandomTip(b.GetPrefix(ctx))}
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

// !cash → Xem tổng quan ví và lịch sử cá nhân (Thiết kế mới UI/UX)
func (b *Bot) cmdCash(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if len(args) == 0 {
		bal, err := b.wallet.GetBalance(ctx, m.Author.ID)
		amount := int64(0)
		if err == nil && bal != nil {
			amount = bal.Balance
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s <@%s>, tài sản hiện tại của bạn là: %s", ui.Emojis.Bank, m.Author.ID, coinDisplay(amount)))
		return
	}

	targetID := m.Author.ID
	if len(args) > 0 {
		if id, ok := ExtractUserID(args[0]); ok {
			targetID = id
		} else if len(m.Mentions) > 0 {
			targetID = m.Mentions[0].ID
		}
	}

	// Chống tra cứu trái phép
	if targetID != m.Author.ID {
		role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
		if role < RoleAdmin {
			sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Admin hoặc Owner mới có thể tra cứu ví của người khác.", 5*time.Second)
			return
		}
	}

	b.renderCashEmbed(s, m.ChannelID, targetID, 1, false, "")
}

// handleWalletPagination xử lý khi người dùng nhấn nút chuyển trang lịch sử
func (b *Bot) handleWalletPagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 4 {
		return
	}

	targetUserID := parts[3]
	callerID := interactionUserID(i)

	if callerID != targetUserID {
		role := b.GetRole(ctx, s, i.GuildID, callerID, i.Member)
		if role < RoleAdmin {
			replyEphemeral(s, i, ui.Emojis.Block+" Bạn không thể lật trang ví của người khác.")
			return
		}
	}

	page, _ := strconv.Atoi(parts[2])

	// Defer để tránh lỗi Interaction Failed
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})

	b.renderCashEmbed(s, i.ChannelID, targetUserID, page, true, i.Message.ID)
}

// renderCashEmbed: Hàm trung tâm render UI ví tiền
func (b *Bot) renderCashEmbed(s *discordgo.Session, channelID, userID string, page int, isEdit bool, messageID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. LUÔN FETCH TỪ DATABASE THEO THỜI GIAN THỰC
	bal, err := b.wallet.GetBalance(ctx, userID)
	amount := int64(0)
	if err == nil && bal != nil {
		amount = bal.Balance
	}

	debtLimit := b.GetEffectiveDebtLimit(ctx, s, "", userID)

	// 2. FETCH LỊCH SỬ GIAO DỊCH (Phân trang, 10 record/trang)
	pageSize := 10
	logs, totalLogs, err := b.wallet.HistoryPaginated(ctx, userID, page, pageSize)
	if err != nil {
		logs = []models.TransactionLog{}
		totalLogs = 0
	}
	totalPages := int((totalLogs + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<@%s>\n", userID))
	sb.WriteString(fmt.Sprintf("%s **Tài sản thực:** %s\n", ui.Emojis.Coin, coinDisplay(amount)))

	if debtLimit > 0 {
		sb.WriteString(fmt.Sprintf("%s **Hạn mức Tín dụng:** %s\n", ui.Emojis.Settings, formatCoins(debtLimit)))
	}

	sb.WriteString(fmt.Sprintf("\n%s **Lịch sử giao dịch**\n", ui.Emojis.History))

	if len(logs) == 0 {
		sb.WriteString(ui.Emojis.Empty + " *Chưa có giao dịch nào.*")
	} else {
		vnZone := time.FixedZone("ICT", 7*3600)
		for _, tx := range logs {
			absAmount := tx.Amount
			if absAmount < 0 {
				absAmount = -absAmount
			}

			timeStr := tx.CreatedAt.In(vnZone).Format("15:04 02/01")
			desc := tx.Description
			emoji := ui.Emojis.Give
			if tx.Amount < 0 {
				emoji = ui.Emojis.Take
			}

			// Tự động phân tách cú pháp User Mention mới siêu gọn gàng
			if strings.HasPrefix(desc, "Transfer to ") {
				desc = fmt.Sprintf("<@%s> chuyển **%s** cho <@%s>", userID, formatCoins(absAmount), strings.TrimPrefix(desc, "Transfer to "))
			} else if strings.HasPrefix(desc, "Transfer from ") {
				desc = fmt.Sprintf("<@%s> nhận **%s** từ <@%s>", userID, formatCoins(absAmount), strings.TrimPrefix(desc, "Transfer from "))
			} else if strings.HasPrefix(desc, "Admin deposit by ") {
				desc = fmt.Sprintf("Admin <@%s> cộng **%s** cho <@%s>", strings.TrimPrefix(desc, "Admin deposit by "), formatCoins(absAmount), userID)
			} else if strings.HasPrefix(desc, "Admin withdrawal by ") {
				desc = fmt.Sprintf("Admin <@%s> trừ **%s** của <@%s>", strings.TrimPrefix(desc, "Admin withdrawal by "), formatCoins(absAmount), userID)
			} else {
				desc = fmt.Sprintf("**%s** • %s", formatCoins(absAmount), strings.ReplaceAll(desc, "\n", " "))
			}

			if !strings.Contains(desc, "<@") && len(desc) > 60 {
				desc = desc[:57] + "..."
			}

			sb.WriteString(fmt.Sprintf("`[%s]` %s %s\n", timeStr, emoji, desc))
		}
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name: "HỒ SƠ GIAO DỊCH CÁ NHÂN",
		},
		Description: sb.String(),
		Color:       0x00FFFF, // Xanh cyan huyền ảo
	}

	// 4. TẠO NÚT BẤM CHUYỂN TRANG
	var components []discordgo.MessageComponent
	if totalPages > 1 {
		btnPrev := discordgo.Button{
			CustomID: fmt.Sprintf("wallet:hist:%d:%s", page-1, userID),
			Label:    "◀ Trang Trước",
			Style:    discordgo.SecondaryButton,
			Disabled: page <= 1,
			Emoji:    ui.ParseEmoji(s, ui.Emojis.FinishTX, "◀️"),
		}
		btnNext := discordgo.Button{
			CustomID: fmt.Sprintf("wallet:hist:%d:%s", page+1, userID),
			Label:    "Trang Sau ▶",
			Style:    discordgo.SecondaryButton,
			Disabled: page >= totalPages,
			Emoji:    ui.ParseEmoji(s, ui.Emojis.Play, "▶️"),
		}
		btnPage := discordgo.Button{
			CustomID: "wallet:hist:indicator",
			Label:    fmt.Sprintf("Trang %d/%d", page, totalPages),
			Style:    discordgo.SecondaryButton,
			Disabled: true,
		}
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{btnPrev, btnPage, btnNext},
		})
	}

	// 5. GỬI HOẶC CHỈNH SỬA TIN NHẮN
	if isEdit {
		_, _ = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    channelID,
			ID:         messageID,
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})
	} else {
		_, _ = s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		})
	}
}
