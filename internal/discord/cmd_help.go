package discord

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ui"
)

// buildHelpEmbed tạo giao diện Help. Bóc tách logic để dễ Unit Test.
func buildHelpEmbed(isAdmin bool, prefix string) *discordgo.MessageEmbed {
	if prefix == "" {
		prefix = "!"
	}
	embed := &discordgo.MessageEmbed{
		Title: "WHISKEY: THE ABYSS DEALER",

		Description: "" +
			"```ansi\n" +
			"[ ABYSS CASINO SYSTEM ONLINE ]\n" +
			"```\n\n" +

			ui.Emojis.Crown + " **Abyss Dealer đã được kích hoạt.** " + ui.Emojis.Crown + "\n" +
			ui.Emojis.StartWhiteL + " Những quân bài nơi vực sao chưa từng tồn tại khái niệm công bằng.\n" +
			"Mỗi lựa chọn đều mang theo một cái giá.\n\n" +

			ui.Emojis.BaicaoBet + " **Quantum Fate Engine:** `ACTIVE`\n" +
			ui.Emojis.BaicaoBet + " **Astral Table Connection:** `STABLE`\n" +
			ui.Emojis.BaicaoBet + " **Soul Gamble Authorization:** `VERIFIED`\n\n",

		Fields: []*discordgo.MessageEmbedField{
			{
				Name: ui.Emojis.BlackWingLeft + " NGHI THỨC MỞ BÀN " + ui.Emojis.BlackWingRight,
				Value: "" +
					"`" + prefix + "taixiu` hoặc `" + prefix + "tx [thời gian] [min] [max]`\n" +
					ui.Emojis.ArrowBlack + " Mở bàn **Tài Xỉu**, nơi xúc xắc quyết định vận mệnh.\n\n" +

					"`" + prefix + "stop`\n" +
					ui.Emojis.ArrowBlack + " Dừng ngay lập tức các lệnh Auto Game và Dọn dẹp kênh.\n\n" +

					"`" + prefix + "baucua` hoặc `" + prefix + "bcua [thời gian] [min] [max]`\n" +
					ui.Emojis.ArrowBlack + " Triệu hồi bàn **Bầu Cua**, trò chơi của linh cảm và may rủi.\n\n" +

					"`" + prefix + "xidach` hoặc `" + prefix + "xd [thời gian] [số tiền]`\n" +
					ui.Emojis.ArrowBlack + " Đối đầu nhà cái trong ván **Xì Dách**.\n\n" +

					"`" + prefix + "baicao` hoặc `" + prefix + "bcao [thời gian] [số tiền]`\n" +
					ui.Emojis.ArrowBlack + " Thách thức vận may trong bàn **Bài Cào**.\n\n",
				Inline: false,
			},
			{
				Name: ui.Emojis.BaicaoBet + " KHẾ ƯỚC TÀI SẢN" + ui.Emojis.BaicaoBet,
				Value: "" +
					"`" + prefix + "pay @user [số tiền]`\n" +
					ui.Emojis.ArrowBlack + " Chuyển tiền cho người chơi khác.\n\n" +

					"`/balance`\n" +
					ui.Emojis.ArrowBlack + " Xem số dư cá nhân hoặc bảng xếp hạng phú hộ.\n\n" +

					"`/history`\n" +
					ui.Emojis.ArrowBlack + " Quan sát lịch sử giao dịch gần nhất.\n\n" +

					"`/gamehistory`\n" +
					ui.Emojis.ArrowBlack + " Xem thống kê lời/lỗ theo từng trò chơi.",
				Inline: false,
			},
			{
				Name: ui.Emojis.BidTX + " LUẬT CỦA VỰC SÂU" + ui.Emojis.BidTX,
				Value: "" +
					"**The Abyss Watches Every Bet.**\n" +
					"**Đừng đổ lỗi do ta sắp đặt, mọi kết quả là vì định mệnh của ngươi!**\n" +
					"**Ngồi xuống bàn cược đồng nghĩa với việc để số phận nhìn lại ngươi.**",
				Inline: false,
			},
		},

		Footer: &discordgo.MessageEmbedFooter{
			Text: "Whiskey: The Abyss Dealer • " + GetRandomTip(prefix),
		},

		Timestamp: time.Now().Format(time.RFC3339),
	}

	if isAdmin {
		adminField := &discordgo.MessageEmbedField{
			Name: ui.Emojis.Settings + " QUYỀN NĂNG QUẢN TRỊ",
			Value: "" +
				"`" + prefix + "panel` hoặc `" + prefix + "dev`\n" +
				ui.Emojis.ArrowBlack + " Mở bảng điều khiển dành cho Admin.\n\n" +
				"`" + prefix + "delete [số lượng]`\n" +
				ui.Emojis.ArrowBlack + " Xóa hàng loạt tin nhắn trong kênh.\n\n" +
				"`/admin give/take`\n" +
				ui.Emojis.ArrowBlack + " Cộng hoặc trừ tiền người chơi.\n\n" +
				"`/admin whitelist`\n" +
				ui.Emojis.ArrowBlack + " Bật hoặc tắt quyền sử dụng bot trong kênh hiện tại.\n\n" +
				"`" + prefix + "limit on/off`\n" +
				ui.Emojis.ArrowBlack + " Bật hoặc tắt giới hạn cược.\n\n" +
				"`/reset`\n" +
				ui.Emojis.ArrowBlack + " Xóa dữ liệu theo phạm vi: `all`, `game`, hoặc `user`.",
			Inline: false,
		}
		embed.Fields = append(embed.Fields[:len(embed.Fields)-1], adminField, embed.Fields[len(embed.Fields)-1])
	}

	return embed
}

func (b *Bot) cmdHelp(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	embed := buildHelpEmbed(role >= RoleAdmin, b.GetPrefix(ctx))
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
