package ui

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

type BJPlayerResult struct {
	Status  string
	Net     int64
	Balance int64
}

func randomBJTip() string {
	tips := []string{
		"Mẹo: Quắc (>21) là thua, nhưng trên 28 sẽ bị Đền Sòng nhé!",
		"Mẹo: Nhà cái dưới 16 (Non) sẽ tự động bị xử thua.",
		"Mẹo: Bài dưới 16 điểm không thể dằn bài.",
		"Mẹo: Bài đạt 21 điểm ngay 2 lá đầu tiên gọi là Xì Bàn!",
		"Mẹo: Host cũng có rủi ro bị Đền Sòng nếu vượt quá 28 điểm.",
	}
	return tips[rand.Intn(len(tips))]
}

func BuildBlackjackEmbed(t *games.BlackjackTable, status string) *discordgo.MessageEmbed {
	var sb strings.Builder
	var title string
	color := 0x6B295C // Cute Pink

	if t.Status == "waiting" {
		sb.WriteString(Emojis.Header)
		sb.WriteString("\n\n")
		title = fmt.Sprintf("%s **BÀN XÌ DÁCH ĐÃ MỞ**", Emojis.Dice)
		sb.WriteString(fmt.Sprintf("%s Cược: **%s**\n", Emojis.HeartHost, FormatNumber(t.BetAmount)))
		sb.WriteString(fmt.Sprintf("%s Thời gian rút/dừng: **%ds**\n\n", Emojis.WhiteStarCircle, t.DurationSec))

		sb.WriteString(fmt.Sprintf("%s Host: <@%s>\n", Emojis.HeartHost, t.HostID))
		sb.WriteString(fmt.Sprintf("%s Số người chơi hiện tại: **%d/12**\n\n", Emojis.Star, len(t.Players)))

		sb.WriteString(fmt.Sprintf("%s 𝑷𝑳𝑨𝒀𝑬𝑹 %s:\n", Emojis.WingLeft, Emojis.WingRight))
		if len(t.PlayerIDs) <= 1 {
			sb.WriteString(fmt.Sprintf("%s _Chưa có ai tham gia..._\n", Emojis.StarPurple))
		} else {
			for _, uid := range t.PlayerIDs {
				if uid == t.HostID {
					continue
				}
				sb.WriteString(fmt.Sprintf("%s <@%s>\n", Emojis.WhiteStar, uid))
			}
		}
	} else { // Playing or finished
		title = fmt.Sprintf("%s **VÁN XÌ DÁCH**", Emojis.Dice)
		if strings.Contains(status, "Đang chia bài...") {
			title = status
		}

		sb.WriteString(fmt.Sprintf("%s Host: <@%s>\n", Emojis.KingPurple, t.HostID))
		hostP := t.Players[t.HostID]
		if len(hostP.Hand) > 1 {
			sb.WriteString(fmt.Sprintf("└ Bài: %s %s\n\n", CardString(hostP.Hand[0]), Emojis.CardBack))
		} else {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("%s 𝑷𝑳𝑨𝒀𝑬𝑹 %s:\n", Emojis.WingLeft, Emojis.WingRight))
		playerCount := 0
		for _, uid := range t.PlayerIDs {
			if uid == t.HostID {
				continue
			}
			playerCount++

			pState := t.Players[uid]

			statusStr := ""
			cards := len(pState.Hand)
			if cards > 2 {
				statusStr = fmt.Sprintf(" *(**%d** lá)*", cards)
			}
			sb.WriteString(fmt.Sprintf("%s %d. <@%s>%s\n", Emojis.WhiteStar, playerCount, uid, statusStr))
		}
		if playerCount == 0 {
			sb.WriteString(fmt.Sprintf("%s _Không có người chơi._\n", Emojis.WhiteEnd))
		}
		sb.WriteString(fmt.Sprintf("\n%s %s", Emojis.WhiteStar, status))
		if !t.Deadline.IsZero() && t.Status == "playing" {
			sb.WriteString(fmt.Sprintf("\n%s Kết thúc: <t:%d:R>", Emojis.ArrowXoan, t.Deadline.Unix()))
		}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: sb.String(),
		Color:       color,
		Footer:      &discordgo.MessageEmbedFooter{Text: randomBJTip()},
	}
}

func BuildBlackjackResultEmbed(t *games.BlackjackTable, results map[string]BJPlayerResult, hostNet int64, hostNewBalance int64) *discordgo.MessageEmbed {
	var sb strings.Builder

	sb.WriteString(Emojis.Header)
	sb.WriteString("\n\n")

	getPointsStr := func(hand []games.Card, isHost bool) string {
		val := games.HandValue(hand)
		if games.IsXiBan(hand) {
			return "Xì Bàn!"
		}
		if games.IsXiDach(hand) {
			return "Xì Dách!"
		}
		if games.IsNguLinh(hand) {
			return "Ngũ Linh!"
		}
		if isHost {
			if val >= 28 {
				return fmt.Sprintf("Già (%d)", val)
			}
			if val > 21 {
				return fmt.Sprintf("Quắc (%d)", val)
			}
			if val < 15 {
				return fmt.Sprintf("Non (%d)", val)
			}
			return fmt.Sprintf("%d điểm", val)
		} else {
			if val >= 28 {
				return fmt.Sprintf("Già (%d)", val)
			}
			if val > 21 {
				return fmt.Sprintf("Quắc (%d)", val)
			}
			if val <= 15 {
				return fmt.Sprintf("Non (%d)", val)
			}
			return fmt.Sprintf("%d điểm", val)
		}
	}

	// Host
	hostState := t.Players[t.HostID]
	hostPoints := getPointsStr(hostState.Hand, true)
	hostNetStr := FormatNumber(hostNet)
	if hostNet > 0 {
		hostNetStr = "+" + hostNetStr
	}
	sb.WriteString(fmt.Sprintf("%s **Chủ bàn: <@%s>**\n", Emojis.KingPurple, t.HostID))
	sb.WriteString(fmt.Sprintf("%s %s\n", Emojis.View, HandString(hostState.Hand)))
	sb.WriteString(fmt.Sprintf("%s %s | Lời/Lỗ: **%s** %s \n%s Số dư: **%s** %s\n", Emojis.WhiteStar, hostPoints, hostNetStr, Emojis.Coin, Emojis.WhiteStar, FormatNumber(hostNewBalance), Emojis.Coin))
	sb.WriteString("----------------------------------------\n")

	if len(results) == 0 {
		sb.WriteString("\n_Ván bài kết thúc do không có khách tham gia!_\n")
	} else {
		for uid, res := range results {
			p := t.Players[uid]
			pPoints := getPointsStr(p.Hand, false)
			pNetStr := FormatNumber(res.Net)
			if res.Net > 0 {
				pNetStr = "+" + pNetStr
			}

			sb.WriteString(fmt.Sprintf("%s **Người chơi: <@%s>**\n", Emojis.ButterflyPurple, uid))
			sb.WriteString(fmt.Sprintf("%s %s\n", Emojis.View, HandString(p.Hand)))
			finalStatus := res.Status
			if !strings.Contains(finalStatus, "ĐỀN BÀN") {
				finalStatus = fmt.Sprintf("%s - %s", pPoints, res.Status)
			}
			sb.WriteString(fmt.Sprintf("**%s** **%s** | Lời/Lỗ: **%s** %s \n%s Số dư: **%s** %s\n", Emojis.WhiteStar, finalStatus, pNetStr, Emojis.Coin, Emojis.WhiteStar, FormatNumber(res.Balance), Emojis.Coin))
			sb.WriteString("----------------------------------------\n")
		}
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s **KẾT QUẢ VÁN XÌ DÁCH** %s", Emojis.WhiteEnd, Emojis.WhiteEnd),
		Description: sb.String(),
		Color:       0x6B295C, // Cute Pink
	}
}

func BlackjackWaitingButtons(tableID string, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("xd:join:%s", tableID), Label: "Tham Gia", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackJoin, Emojis.BlackjackJoinFB)},
				discordgo.Button{CustomID: fmt.Sprintf("xd:start:%s", tableID), Label: "Bắt Đầu (Host)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackStart, Emojis.BlackjackStartFB)},
				discordgo.Button{CustomID: fmt.Sprintf("xd:cancel:%s", tableID), Label: "Hủy Bàn", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackCancel, Emojis.BlackjackCancelFB)},
			},
		},
	}
}

func BlackjackPlayingButtons(tableID string, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("xd:view:%s", tableID), Label: "Xem Bài", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackView, Emojis.BlackjackViewFB)},
				discordgo.Button{CustomID: fmt.Sprintf("xd:hit:%s", tableID), Label: "Rút Thêm", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackHit, Emojis.BlackjackHitFB)},
				discordgo.Button{CustomID: fmt.Sprintf("xd:stand:%s", tableID), Label: "Dằn!", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackStand, Emojis.BlackjackStandFB)},
			},
		},
	}
}

func BlackjackResultButtons(hostID string, bet int64, durationSec int, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("xd:continue:%s:%d:%d", hostID, bet, durationSec), Label: "Tiếp tục (Tạo ván mới)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BlackjackContinue, Emojis.BlackjackContinueFB)},
			},
		},
	}
}
