package ui

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

type BaicaoPlayerResult struct {
	Rank    games.BaicaoRank
	Net     int64
	Balance int64
	Status  string // "folded", "revealed", ...
}

func BuildBaicaoEmbed(tableID, hostID string, betAmount int64, durationSec int, playerIDs []string, playerStatus map[string]string, hands map[string][]games.Card, status string, deadline int64) *discordgo.MessageEmbed {
	var sb strings.Builder
	var title string
	color := 0xFF3333 // Red

	switch status {
	case "waiting":
		title = fmt.Sprintf("%s **BÀN BÀI CÀO**", Emojis.BaicaoHeart)
		sb.WriteString(fmt.Sprintf("%s Mức Cược: **%s**\n", Emojis.BaicaoBet, FormatNumber(betAmount)))
		sb.WriteString(fmt.Sprintf("%s Yêu cầu số dư đảm bảo phòng đền Sáp cho ván đấu\n\n", Emojis.BaicaoReq))

		sb.WriteString(fmt.Sprintf("%s Chủ bàn: <@%s>\n", Emojis.BaicaoHost, hostID))
		sb.WriteString(fmt.Sprintf("%s Người chơi: **%d/6**\n\n", Emojis.BaicaoPlayers, len(playerIDs)))

		sb.WriteString(fmt.Sprintf("%s 𝑷𝑳𝑨𝒀𝑬𝑹:\n", Emojis.BaicaoPlayerTitle))
		if len(playerIDs) <= 1 {
			sb.WriteString(fmt.Sprintf("%s _Chờ khách tham gia..._\n", Emojis.BaicaoJoin))
		} else {
			for _, uid := range playerIDs {
				sb.WriteString(fmt.Sprintf("%s <@%s>\n", Emojis.BaicaoJoin, uid))
			}
		}
	case "dealing":
		title = fmt.Sprintf("%s VÁN BÀI CÀO ĐANG DIỄN RA", Emojis.BaicaoKing)
		sb.WriteString(fmt.Sprintf("Nhà Cái Đang Chia Bài... %s %s %s\n\n", Emojis.BaicaoBlink, Emojis.BaicaoBlink, Emojis.BaicaoBlink))
		sb.WriteString("Vui lòng đợi giây lát...\n")
		for _, uid := range playerIDs {
			sb.WriteString(fmt.Sprintf("<@%s> ", uid))
		}
		sb.WriteString("\n")
	default:
		title = fmt.Sprintf("%s VÁN BÀI CÀO ĐANG DIỄN RA", Emojis.BaicaoHeart)
		if status == "finished" {
			title = fmt.Sprintf("%s VÁN BÀI CÀO ĐÃ KẾT THÚC", Emojis.BaicaoHeart)
		}
		sb.WriteString(fmt.Sprintf("%s 𝑷𝑳𝑨𝒀𝑬𝑹:\n", Emojis.RoundTX))
		for _, uid := range playerIDs {
			st := playerStatus[uid]
			stStr := ""
			switch st {
			case "folded":
				stStr = fmt.Sprintf(" %s *Đã Bỏ bài*", Emojis.BaicaoFoldBtn)
			case "showed":
				handStr := HandString(hands[uid])
				stStr = fmt.Sprintf(" %s ***Đã Show bài***\n> %s", Emojis.BaicaoResultTitle, handStr)
			default:
				stStr = fmt.Sprintf("%s %s %s *(Đang xem bài)*", Emojis.CardBackside, Emojis.CardBackside, Emojis.CardBackside)
			}
			sb.WriteString(fmt.Sprintf("%s <@%s>: %s\n", Emojis.BaicaoJoin, uid, stStr))
		}
		if deadline > 0 {
			sb.WriteString(fmt.Sprintf("\n%s Tự động kết thúc ván: <t:%d:R>", Emojis.BaicaoEndgameTitle, deadline))
		} else if deadline == -1 {
			sb.WriteString(fmt.Sprintf("\n%s **Ván đấu đã kết thúc**", Emojis.BaicaoEndgameTitle))
		}
	}

	var footer *discordgo.MessageEmbedFooter
	if status == "waiting" {
		tips := []string{
			"Mẹo: Bài Cào J, Q, K tính 10đ. A tính 1đ.",
			"Mẹo: Có Sáp (3 lá giống nhau) ăn gấp đôi tiền cược từ mọi nhà!",
			"Mẹo: Ba Cào (3 lá Tây) lớn hơn 9 điểm.",
			"Info: Bài xấu là do mày, không phải tại code.",
			"Mẹo: Con gái sẽ được ưu ái!",
			"Warn: Chơi quá 180p một ngày sẽ trở thành con nghẹo đấy!",
		}
		footer = &discordgo.MessageEmbedFooter{Text: tips[rand.Intn(len(tips))]}
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: sb.String(),
		Color:       color,
		Footer:      footer,
	}
}

func BuildBaicaoResultEmbed(hostID string, playerIDs []string, hands map[string][]games.Card, results map[string]BaicaoPlayerResult) *discordgo.MessageEmbed {
	var sb strings.Builder

	for _, uid := range playerIDs {
		hand := hands[uid]
		res := results[uid]

		netStr := FormatNumber(res.Net)
		if res.Net > 0 {
			netStr = "+" + netStr
		}

		balStr := FormatNumber(res.Balance)

		if uid == hostID {
			sb.WriteString(fmt.Sprintf("%s **Chủ bàn:** <@%s>\n", Emojis.BaicaoHost, uid))
		} else {
			sb.WriteString(fmt.Sprintf("%s **Người chơi:** <@%s>\n", Emojis.BaicaoPlayer, uid))
		}

		if res.Status == "folded" {
			sb.WriteString(fmt.Sprintf("> %s %s %s *(Úp bài)*\n", Emojis.CardBackside, Emojis.CardBackside, Emojis.CardBackside))
			sb.WriteString(fmt.Sprintf("%s Kết quả: **Bỏ Bài** | Lời/Lỗ: **%s** %s\n%s Số dư: **%s** %s\n", Emojis.RuleChoiceTX, netStr, Emojis.Coin, Emojis.RuleChoiceTX, balStr, Emojis.Coin))
		} else {
			sb.WriteString(fmt.Sprintf("> %s\n", HandString(hand)))
			sb.WriteString(fmt.Sprintf("%s Kết quả: **%s** | Lời/Lỗ: **%s** %s\n%s Số dư: **%s** %s\n", Emojis.RuleChoiceTX, res.Rank.Label, netStr, Emojis.Coin, Emojis.RuleChoiceTX, balStr, Emojis.Coin))
		}
		sb.WriteString("----------------------------------------\n")
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s KẾT QUẢ BÀI CÀO %s", Emojis.BaicaoResultTitle, Emojis.BaicaoResultTitle),
		Description: sb.String(),
		Color:       0xFF3333, // Red
	}
}

func BaicaoWaitingButtons(tableID string, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("bcao:join:%s", tableID), Label: "Tham Gia", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoJoinBtn, Emojis.BaicaoJoinBtnFB)},
				discordgo.Button{CustomID: fmt.Sprintf("bcao:start:%s", tableID), Label: "Chia Bài (Host)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoStartBtn, Emojis.BaicaoStartBtnFB)},
				discordgo.Button{CustomID: fmt.Sprintf("bcao:cancel:%s", tableID), Label: "Hủy Bàn", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoCancelBtn, Emojis.BaicaoCancelBtnFB)},
			},
		},
	}
}

func BaicaoResultButtons(hostID string, bet int64, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("bcao:continue:%s:%d", hostID, bet), Label: "Tiếp tục (Tạo ván mới)", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoContinueBtn, Emojis.BaicaoContinueBtnFB)},
			},
		},
	}
}

func BaicaoPlayingButtons(tableID string, s *discordgo.Session) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{CustomID: fmt.Sprintf("bcao:view:%s", tableID), Label: "Xem Bài", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoViewBtn, Emojis.BaicaoViewBtnFB)},
				discordgo.Button{CustomID: fmt.Sprintf("bcao:reveal:%s", tableID), Label: "Nặn Bài", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoRevealBtn, Emojis.BaicaoRevealBtnFB)},
				discordgo.Button{CustomID: fmt.Sprintf("bcao:show:%s", tableID), Label: "Show Bài", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoShowBtn, Emojis.BaicaoShowBtnFB)},
				discordgo.Button{CustomID: fmt.Sprintf("bcao:fold:%s", tableID), Label: "Bỏ Bài", Style: discordgo.SecondaryButton, Emoji: ParseEmoji(s, Emojis.BaicaoFoldBtn, Emojis.BaicaoFoldBtnFB)},
			},
		},
	}
}
