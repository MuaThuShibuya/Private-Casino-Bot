package ui

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

type BaucuaBet struct {
	UserID string
	Side   string
	Amount int64
	Escrow int64
	Payout float64
}

func GetBaucuaName(side string) string {
	switch side {
	case "bau":
		return "Bầu"
	case "cua":
		return "Cua"
	case "tom":
		return "Tôm"
	case "ca":
		return "Cá"
	case "nai":
		return "Nai"
	case "ga":
		return "Gà"
	}
	return "Không rõ"
}

func GetBaucuaEmoji(side string) string {
	switch side {
	case "bau":
		return Emojis.Bau
	case "cua":
		return Emojis.Cua
	case "tom":
		return Emojis.Tom
	case "ca":
		return Emojis.Ca
	case "nai":
		return Emojis.Nai
	case "ga":
		return Emojis.Ga
	}
	return ""
}

func BuildBaucuaEmbed(hostID string, minBet, maxBet int64, deadlineUnix int64, bets []BaucuaBet) *discordgo.MessageEmbed {
	var sb strings.Builder

	if deadlineUnix <= 0 {
		sb.WriteString(fmt.Sprintf("## %s ĐÃ ĐÓNG CƯỢC BẦU CUA %s\n", Emojis.TitleBC, Emojis.TitleBC))
	} else {
		sb.WriteString(fmt.Sprintf("## %s BÀN CƯỢC BẦU CUA %s\n", Emojis.TitleBC, Emojis.TitleBC))
	}

	sb.WriteString(fmt.Sprintf("%s Host: <@%s>\n", Emojis.HostBC, hostID))
	sb.WriteString(fmt.Sprintf("%s Mức cược: **%s** - **%s**\n", Emojis.BetBC, FormatNumber(minBet), FormatNumber(maxBet)))
	sb.WriteString(fmt.Sprintf("%s Số người đã cược: **%d**\n", Emojis.PlayerBC, len(bets)))
	if deadlineUnix > 0 {
		sb.WriteString(fmt.Sprintf("%s Kết thúc: <t:%d:R>\n", Emojis.TimerBC, deadlineUnix))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("## %s Cách tính thưởng\n", Emojis.RewardBC))
	sb.WriteString(fmt.Sprintf("%s Đúng 1 hình %s ăn x1\n", Emojis.StarBC, Emojis.LineArrowBC))
	sb.WriteString(fmt.Sprintf("%s Đúng 2 hình %s ăn x2\n", Emojis.StarBC, Emojis.LineArrowBC))
	sb.WriteString(fmt.Sprintf("%s Đúng 3 hình %s ăn x3\n", Emojis.StarBC, Emojis.LineArrowBC))
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("## %s Lưu ý\n", Emojis.WarningBC))
	sb.WriteString(fmt.Sprintf("%s Kết quả do nhà cái công bố\n", Emojis.Line1BC))
	sb.WriteString(fmt.Sprintf("%s Quyết định nhà cái là cuối cùng\n", Emojis.Line2BC))
	sb.WriteString(fmt.Sprintf("%s Spam hoặc gây rối → cấm chơi\n", Emojis.Line3BC))
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("## %s Danh sách cược\n", Emojis.ListBC))
	if len(bets) == 0 {
		sb.WriteString(fmt.Sprintf("%s _Chưa có ai tham gia..._\n", Emojis.EmptyBC))
	} else {
		for _, bet := range bets {
			sb.WriteString(fmt.Sprintf("%s <@%s> cược **%s** vào %s **%s**\n", Emojis.LineArrowBC, bet.UserID, FormatNumber(bet.Amount), GetBaucuaEmoji(bet.Side), GetBaucuaName(bet.Side)))
		}
	}

	return &discordgo.MessageEmbed{
		Description: sb.String(),
		Color:       0x00BFFF, // Màu Xanh Biển
	}
}

func BuildBaucuaResultEmbed(hostID string, result games.BaucuaResult, bets []BaucuaBet, hostNet int64, payoutPerMatch float64) *discordgo.MessageEmbed {
	var sb strings.Builder

	face1Name := GetBaucuaName(string(result.Face1))
	face2Name := GetBaucuaName(string(result.Face2))
	face3Name := GetBaucuaName(string(result.Face3))

	sb.WriteString(fmt.Sprintf("## %s KẾT QUẢ BẦU CUA %s\n", Emojis.TitleBC, Emojis.TitleBC))
	sb.WriteString(fmt.Sprintf("%s Host: <@%s>\n", Emojis.HostBC, hostID))

	resDisplay := fmt.Sprintf("%s %s %s %s %s", GetBaucuaEmoji(string(result.Face1)), Emojis.LineArrowBC, GetBaucuaEmoji(string(result.Face2)), Emojis.LineArrowBC, GetBaucuaEmoji(string(result.Face3)))
	sb.WriteString(fmt.Sprintf("%s Kết quả: %s\n", Emojis.RewardBC, resDisplay))
	sb.WriteString(fmt.Sprintf("%s Chi tiết: %s - %s - %s\n", Emojis.ListBC, face1Name, face2Name, face3Name))
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("## %s Chi tiết cược\n", Emojis.ListBC))
	if len(bets) == 0 {
		sb.WriteString(fmt.Sprintf("%s _Không có người tham gia._\n", Emojis.EmptyBC))
	} else {
		for _, bet := range bets {
			matches := result.MatchCount(bet.Side)
			profit := -bet.Amount
			statusStr := "Thua"
			if matches > 0 {
				profit = int64(float64(bet.Amount) * float64(matches) * payoutPerMatch)
				statusStr = fmt.Sprintf("Thắng x%d", matches)
			}

			profStr := FormatNumber(profit)
			if profit > 0 {
				profStr = "+" + profStr
			}
			sb.WriteString(fmt.Sprintf("%s <@%s> cược **%s** vào %s | %s: **%s**\n", Emojis.LineArrowBC, bet.UserID, FormatNumber(bet.Amount), GetBaucuaEmoji(bet.Side), statusStr, profStr))
		}

		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("## %s Tổng kết\n", Emojis.StarBC))
		type UserSummary struct {
			Profit int64
			Bets   int
		}
		userSummaries := make(map[string]*UserSummary)
		var order []string
		for _, bet := range bets {
			matches := result.MatchCount(bet.Side)
			profit := -bet.Amount
			if matches > 0 {
				profit = int64(float64(bet.Amount) * float64(matches) * payoutPerMatch)
			}

			if userSummaries[bet.UserID] == nil {
				userSummaries[bet.UserID] = &UserSummary{}
				order = append(order, bet.UserID)
			}
			userSummaries[bet.UserID].Profit += profit
			userSummaries[bet.UserID].Bets++
		}

		for _, userID := range order {
			summary := userSummaries[userID]
			profit := summary.Profit
			profStr := FormatNumber(profit)
			if profit > 0 {
				profStr = "+" + profStr
			}
			sb.WriteString(fmt.Sprintf("%s <@%s>: **%s** | Tổng vé: %d\n", Emojis.StarBC, userID, profStr, summary.Bets))
		}
	}

	return &discordgo.MessageEmbed{
		Description: sb.String(),
		Color:       0x00BFFF, // Màu Xanh Biển
	}
}

func BaucuaButtons(s *discordgo.Session, disabled bool) []discordgo.MessageComponent {
	row1 := []discordgo.MessageComponent{
		discordgo.Button{CustomID: "bc:bau", Label: "Bầu", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaBau, Emojis.BaucuaBauFB)},
		discordgo.Button{CustomID: "bc:cua", Label: "Cua", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaCua, Emojis.BaucuaCuaFB)},
		discordgo.Button{CustomID: "bc:tom", Label: "Tôm", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaTom, Emojis.BaucuaTomFB)},
	}
	row2 := []discordgo.MessageComponent{
		discordgo.Button{CustomID: "bc:ca", Label: "Cá", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaCa, Emojis.BaucuaCaFB)},
		discordgo.Button{CustomID: "bc:nai", Label: "Nai", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaNai, Emojis.BaucuaNaiFB)},
		discordgo.Button{CustomID: "bc:ga", Label: "Gà", Style: discordgo.SecondaryButton, Disabled: disabled, Emoji: ParseEmoji(s, Emojis.BaucuaGa, Emojis.BaucuaGaFB)},
	}

	if disabled {
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: row1},
			discordgo.ActionsRow{Components: row2},
		}
	}

	row3 := []discordgo.MessageComponent{
		discordgo.Button{CustomID: "bc:cancel", Label: "Hủy Bàn", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaucuaCancel, Emojis.BaucuaCancelFB)},
	}
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: row1},
		discordgo.ActionsRow{Components: row2},
		discordgo.ActionsRow{Components: row3},
	}
}

func BaucuaResultButtons(hostID string, minBet, maxBet int64, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("bc:continue:%s:%d:%d", hostID, minBet, maxBet), Label: "Tiếp tục (Tạo ván mới)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaucuaContinue, Emojis.BaucuaContinueFB)},
			},
		},
	}
}
