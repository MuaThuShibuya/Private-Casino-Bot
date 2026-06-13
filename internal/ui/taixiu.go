package ui

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

type TaixiuBet struct {
	UserID string
	Side   string
	Amount int64
	Escrow int64
}

type TaixiuPlayerResult struct {
	IsWin   bool
	Net     int64
	Balance int64
	Side    string
	Amount  int64
}

func BuildTaixiuEmbed(hostID string, minBet, maxBet int64, deadlineUnix int64, bets []TaixiuBet) *discordgo.MessageEmbed {
	var sb strings.Builder
	var title string
	color := 0x000000 // Black

	if deadlineUnix <= 0 {
		title = fmt.Sprintf("%s ĐÃ ĐÓNG CƯỢC TÀI XỈU %s", Emojis.TXClosed, Emojis.TXClosed)
	} else {
		title = fmt.Sprintf("%s **BÀN CƯỢC TÀI XỈU** %s", Emojis.TXClosed, Emojis.TXClosed)
	}

	sb.WriteString(fmt.Sprintf("%s *Host*: <@%s>\n", Emojis.KingTX, hostID))
	sb.WriteString(fmt.Sprintf("%s *Mức cược:* %s – %s\n", Emojis.BidTX, FormatNumber(minBet), FormatNumber(maxBet)))
	sb.WriteString(fmt.Sprintf("%s *Số người đã cược:* %d\n", Emojis.PeopleTX, len(bets)))
	if deadlineUnix > 0 {
		sb.WriteString(fmt.Sprintf("%s Kết thúc: <t:%d:R>\n", Emojis.StartWhiteL, deadlineUnix))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("%s **Cửa cược:**\n", Emojis.RoundTX))
	sb.WriteString(fmt.Sprintf("%s *XỈU:* 4 – 10\n", Emojis.RuleChoiceTX))
	sb.WriteString(fmt.Sprintf("%s *TÀI:* 11 – 17\n", Emojis.RuleChoiceTX))
	sb.WriteString(fmt.Sprintf("%s **Luật đặc biệt:**\n", Emojis.RoundTX))
	sb.WriteString(fmt.Sprintf("%s *Bộ ba (111,666):*\n", Emojis.RuleChoiceTX))
	sb.WriteString(fmt.Sprintf("%s Nhà cái ăn hết hoặc ai đặt Hũ thì **x10**\n\n", Emojis.ArrowBlack))
	sb.WriteString(fmt.Sprintf("%s **Lưu ý:**\n", Emojis.Warn))
	sb.WriteString(fmt.Sprintf("%s Kết quả do nhà cái công bố.\n%s Quyết định của nhà cái là cuối cùng.\n%s Spam hoặc gây rối → cấm chơi.\n\n", Emojis.ArrowBlack, Emojis.ArrowBlack, Emojis.ArrowBlack))

	sb.WriteString(fmt.Sprintf("%s **Danh sách cược**:\n", Emojis.RoundTX))
	if len(bets) == 0 {
		sb.WriteString(fmt.Sprintf("%s _Chưa có ai tham gia..._\n", Emojis.RuleChoiceTX))
	} else {
		for i, bet := range bets {
			sideName := ""
			switch bet.Side {
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
			sb.WriteString(fmt.Sprintf("%s %d. <@%s> | %s | %s\n", Emojis.RuleChoiceTX, i+1, bet.UserID, sideName, FormatNumber(bet.Amount)))
		}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: sb.String(),
		Color:       color,
	}
}

func BuildTaixiuResultEmbed(hostID string, result games.TaixiuResult, results map[string]TaixiuPlayerResult, hostNet int64, isHuRule bool) *discordgo.MessageEmbed {
	var sb strings.Builder

	leChan := "Lẻ"
	if result.Sum%2 == 0 {
		leChan = "Chẵn"
	}
	sb.WriteString(fmt.Sprintf("%s **Chủ bàn:** <@%s>\n", Emojis.KingTX, hostID))

	sb.WriteString(fmt.Sprintf("%s **%d** %s **%s** %s **%s** %s\n", Emojis.OWOBlack, result.Sum, Emojis.Dot, result.Label(), Emojis.Dot, leChan, Emojis.OWOBlack))

	huStr := "Không"
	if result.IsTriple {
		huStr = "Có"
	}
	sb.WriteString(fmt.Sprintf("%s *Hũ*: **%s**\n", Emojis.RoundTX, huStr))

	cuaThang := ""
	if result.IsTriple {
		cuaThang = "Hũ"
	} else {
		cuaThang = fmt.Sprintf("%s, %s", result.Label(), leChan)
	}
	sb.WriteString(fmt.Sprintf("%s *Cửa thắng*: **%s**\n\n", Emojis.FinishTX, cuaThang))
	sb.WriteString(fmt.Sprintf("%s ***DANH SÁCH CƯỢC***\n", Emojis.Crown))

	if len(results) == 0 {
		sb.WriteString(fmt.Sprintf("%s _Không có người tham gia._\n", Emojis.RuleChoiceTX))
	} else {
		for uid, res := range results {
			statusStr := "Thua"
			if res.IsWin {
				statusStr = "Thắng"
			}
			profStr := FormatNumber(res.Net)
			if res.Net > 0 {
				profStr = "+" + profStr
			}
			balStr := FormatNumber(res.Balance)

			sideName := ""
			switch res.Side {
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

			sb.WriteString(fmt.Sprintf("<@%s> | Cửa: **%s** | Cược: **%s**\n", uid, sideName, FormatNumber(res.Amount)))
			sb.WriteString(fmt.Sprintf("Kết quả: **%s** | Lời/Lỗ: **%s** %s\n%s Số dư: **%s** %s\n", statusStr, profStr, Emojis.Coin, Emojis.RuleChoiceTX, balStr, Emojis.Coin))
			sb.WriteString("----------------------------------------\n")
		}
	}

	return &discordgo.MessageEmbed{
		Title:       "KẾT QUẢ TÀI XỈU",
		Description: sb.String(),
		Color:       0x000000, // Black
	}
}

func TaixiuButtons(s *discordgo.Session, disabled bool) []discordgo.MessageComponent {
	components1 := []discordgo.MessageComponent{
		discordgo.Button{CustomID: "taixiu:tai", Label: "Tài", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.TaixiuTai, Emojis.TaixiuTaiFB)},
		discordgo.Button{CustomID: "taixiu:xiu", Label: "Xỉu", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.TaixiuXiu, Emojis.TaixiuXiuFB)},
		discordgo.Button{CustomID: "taixiu:chan", Label: "Chẵn", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.TaixiuChan, Emojis.TaixiuChanFB)},
		discordgo.Button{CustomID: "taixiu:le", Label: "Lẻ", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.TaixiuLe, Emojis.TaixiuLeFB)},
		discordgo.Button{CustomID: "taixiu:hu", Label: "Hũ", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.TaixiuHu, Emojis.TaixiuHuFB)},
	}

	components2 := []discordgo.MessageComponent{}
	if !disabled {
		components2 = append(components2, discordgo.Button{CustomID: "taixiu:cancel", Label: "Hủy Bàn", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.TaixiuCancel, Emojis.TaixiuCancelFB)})
	}

	if disabled {
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: components1},
		}
	}
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: components1},
		discordgo.ActionsRow{Components: components2},
	}
}

func TaixiuResultButtons(hostID string, minBet, maxBet int64, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("taixiu:continue:%s:%d:%d", hostID, minBet, maxBet), Label: "Tiếp tục (Tạo ván mới)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.TaixiuContinue, Emojis.TaixiuContinueFB)},
			},
		},
	}
}
