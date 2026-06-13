package discord

// ═══════════════════════════════════════════════════════════════════════════════
// util.go — THƯ VIỆN UI CƠ BẢN
//
// Đây là nơi định nghĩa các HÀM DÙNG LẠI cho toàn bộ giao diện bot.
// Mỗi game handler (handler_taixiu.go, handler_blackjack.go...) gọi các hàm
// này để gửi tin nhắn, tạo embed, xóa tin nhắn tự động, v.v.
//
// SƠ ĐỒ UI BOT:
//
//   util.go  ←─── Hàm gửi/xóa/cập nhật tin nhắn cơ bản
//      ↑
//   handler_taixiu.go    → buildTaixiuEmbed()   ← Thiết kế giao diện Tài Xỉu
//   handler_blackjack.go → ui.RenderBlackjackScene() ← Thiết kế giao diện Blackjack
//   bot.go               → handleBalance...     ← Thiết kế giao diện Slash Commands
//
// MÀU SẮC CHUẨN (thay ở đây để đồng bộ toàn bot):
//   ColorError   = 0xFF4444   → Đỏ    (thua, lỗi)
//   ColorSuccess = 0x00CC66   → Xanh  (thắng, thành công)
//   ColorPending = 0xFFD700   → Vàng  (đang chờ cược - Tài Xỉu)
//   ColorGame    = 0x5865F2   → Tím   (đang chơi - Blackjack, RTB)
//   ColorNeutral = 0xAAAAAA   → Xám   (hoà, thông tin)
// ═══════════════════════════════════════════════════════════════════════════════

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

// ── Màu sắc embed — đổi ở đây để thay đổi toàn bộ bot ───────────────────────
const (
	ColorError   = 0xFF4444 // Đỏ: lỗi, thua, từ chối
	ColorSuccess = 0x00CC66 // Xanh: thành công, thắng
	ColorPending = 0xFFD700 // Vàng: đang chờ (Tài Xỉu countdown)
	ColorGame    = 0x5865F2 // Tím Discord: đang trong ván
	ColorNeutral = 0xAAAAAA // Xám: hoà, thông tin trung lập
	ColorAdmin   = 0xFFA500 // Cam: hành động admin
)

const (
	GameMessageRefreshInterval = 10 * time.Second
	ActionDebounceDelay        = 800 * time.Millisecond
	DrawStatsDebounceDelay     = 800 * time.Millisecond
	InteractionLockTTL         = 3 * time.Second
)

// ── Gửi tin nhắn text tạm thời — tự xóa sau delay ───────────────────────────
// Dùng cho: thông báo lỗi, rate limit, cảnh báo nhỏ
func sendTemp(s *discordgo.Session, channelID, content string, delay time.Duration) {
	msg, err := s.ChannelMessageSend(channelID, content)
	if err != nil {
		log.Printf("[UI LỖI] Không thể gửi tin nhắn text vào kênh %s (Thiếu quyền Send Messages?): %v", channelID, err)
		return
	}
	time.AfterFunc(delay, func() {
		_ = s.ChannelMessageDelete(channelID, msg.ID)
	})
}

// ── Gửi embed tạm thời — tự xóa sau delay ───────────────────────────────────
// Dùng cho: thông báo có màu sắc nhưng không cần tồn tại lâu
func sendTempEmbed(s *discordgo.Session, channelID string, embed *discordgo.MessageEmbed, delay time.Duration) {
	msg, err := s.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		log.Printf("[UI LỖI] Không thể gửi embed vào kênh %s (Thiếu quyền Embed Links?): %v", channelID, err)
		return
	}
	time.AfterFunc(delay, func() {
		_ = s.ChannelMessageDelete(channelID, msg.ID)
	})
}

// ── Trả lời ephemeral (chỉ người nhấn nút / gọi lệnh mới thấy) ──────────────
// Dùng cho: "Đây không phải ván của bạn", "Số dư không đủ", lỗi cá nhân
func replyEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// ── Trả lời bằng embed công khai (mọi người trong kênh đều thấy) ─────────────
// Dùng cho: gửi kết quả game, thông báo hành động admin
func replyEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

// ── Trả lời bằng ephemeral sau khi đã Defer (InteractionResponseDeferredMessageUpdate) ──
func followupEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		log.Printf("[UI] Lỗi gửi followupEphemeral: %v", err)
	}
}

// ── Cập nhật embed đang hiển thị (khi người chơi nhấn nút) ───────────────────
// Dùng cho: Blackjack khi hit/stand, mở bát...
// Quan trọng: dùng InteractionResponseUpdateMessage chứ không gửi message mới
func updateEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})
}

// ── Xóa tin nhắn sau delay (goroutine, không block) ──────────────────────────
func deleteAfter(s *discordgo.Session, channelID, messageID string, delay time.Duration) {
	time.AfterFunc(delay, func() {
		if err := s.ChannelMessageDelete(channelID, messageID); err != nil {
			log.Printf("[UI] Không xóa được tin nhắn %s: %v", messageID, err)
		}
	})
}

// ── Tắt tất cả nút bấm trên một message ──────────────────────────────────────
// Dùng sau khi game kết thúc để tránh người chơi click lại
func disableAllButtons(components []discordgo.MessageComponent) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, len(components))
	for i, row := range components {
		ar, ok := row.(discordgo.ActionsRow)
		if !ok {
			out[i] = row
			continue
		}
		newButtons := make([]discordgo.MessageComponent, len(ar.Components))
		for j, comp := range ar.Components {
			if btn, ok := comp.(discordgo.Button); ok {
				btn.Disabled = true
				newButtons[j] = btn
			} else {
				newButtons[j] = comp
			}
		}
		out[i] = discordgo.ActionsRow{Components: newButtons}
	}
	return out
}

// ── Định dạng số tiền có dấu phẩy phân cách hàng nghìn ───────────────────────
// Ví dụ: 1234567 → "1,234,567"
func formatCoins(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return sign + s
	}
	var sb strings.Builder
	sb.WriteString(sign)
	mod := len(s) % 3
	for i, ch := range s {
		if i > 0 && (i-mod)%3 == 0 && mod != 0 || (i > 0 && mod == 0 && i%3 == 0) {
			sb.WriteRune('.')
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

// ── Tạo embed lỗi màu đỏ ─────────────────────────────────────────────────────
// Dùng cho: mọi thông báo lỗi hệ thống, lỗi quyền truy cập
func errorEmbed(msg string) *discordgo.MessageEmbed {
	return view.ErrorEmbed(msg)
}

// ── Tạo embed thành công màu xanh ────────────────────────────────────────────
// Dùng cho: thao tác admin thành công, chuyển tiền thành công
func successEmbed(title, msg string) *discordgo.MessageEmbed {
	return view.SuccessEmbed(title, msg)
}

// ── Hiển thị số tiền chuẩn với icon xu ───────────────────────────────────────
// Thay assets.IconCoin trong assets/emojis.go để đổi icon toàn bot.
func coinDisplay(amount int64) string {
	return fmt.Sprintf("**%s** %s", formatCoins(amount), ui.Emojis.Coin)
}

// ── Mẹo vặt Random ───────────────────────────────────────────────────────────
func GetRandomTip(prefix string) string {
	if prefix == "" {
		prefix = "!"
	}
	tips := []string{
		fmt.Sprintf("Mẹo: Dùng lệnh %stx, %sbc, %sxd để gọi game nhanh hơn.", prefix, prefix, prefix),
		"Mẹo: Gõ /balance để xem Top 10 phú hộ trong server.",
		fmt.Sprintf("Mẹo: Bạn có thể chuyển tiền nhanh bằng lệnh %spay @user.", prefix),
		"Mẹo: Dùng /gamehistory để kiểm tra lịch sử Lời/Lỗ của bạn.",
		"Mẹo: Hãy chơi có chừng mực, đừng all-in nếu không chắc chắn!",
		"Mẹo: Dùng /history để tra cứu giao dịch gần nhất của bạn.",
		"Mẹo: Chơi xì dách bị đền sòng sẽ mất nhiều tiền hơn bình thường!",
		"Mẹo: Nếu gặp lỗi, hãy báo cho admin hoặc owner của bot nhé!",
		"Mẹo: Bot có thể bị lag nếu server Discord quá tải, hãy kiên nhẫn!",
		"Mẹo: Nếu bạn xinh gái, liên hệ Admin để được ưu đãi nhé!",
		"Mẹo: Bạn có thể chơi nhiều game cùng lúc, nhưng hãy cẩn thận với số dư của mình!",
		"Mẹo: Đừng quên kiểm tra lịch sử giao dịch của bạn để theo dõi số dư và các khoản cược đã đặt!",
		"Info: Bot được phát triển bởi Whiskey. Nếu bạn có bất kỳ câu hỏi nào, đừng ngần ngại liên hệ với admin!",
		"Info: Admin: ig: whiskyesc04, liên hệ nếu BOT có lỗi hoặc bạn muốn góp ý tính năng mới!",
		"Info: Đây là bot build cho vui nên đừng mắng Admin lười nhé!",
		"Warning: Bot đang trong quá trình phát triển, có thể xuất hiện lỗi hoặc bất tiện!",
		"Warning: Hãy chơi có trách nhiệm, đừng để thua quá nhiều tiền ảo nhé!",
		"Warning: Dev tính nóng như kem có thể Ban nick nếu bạn spam hoặc phá game đấy!",
	}
	return tips[rand.Intn(len(tips))]
}

// Lấy tỉ lệ bịp (Rig Probability) từ Config
func getRigProb(mode models.RigMode) int {
	switch mode {
	case models.RigLight:
		return 40
	case models.RigNormal:
		return 50
	case models.RigHard:
		return 80
	case models.RigNightmare:
		return 95
	default:
		return 0 // RigOff
	}
}

// ── HỆ THỐNG TỰ ĐỘNG TẠO BÀN (AUTO-CREATE SERIES) ─────────────────────────────
type GameCreateParams struct {
	MinBet      int64
	MaxBet      int64
	BetAmount   int64
	DurationSec int
}

type AutoGameSeries struct {
	SeriesID     string
	GameType     string
	ChannelID    string
	GuildID      string
	OwnerID      string
	CurrentIndex int
	TotalCount   int
	Params       GameCreateParams
	Active       bool
}

var activeAutoSeries sync.Map // Key: ChannelID_GameType -> *AutoGameSeries

func GetAutoSeries(channelID, gameType string) *AutoGameSeries {
	if val, ok := activeAutoSeries.Load(channelID + "_" + gameType); ok {
		return val.(*AutoGameSeries)
	}
	return nil
}

func StartAutoSeries(series *AutoGameSeries) bool {
	key := series.ChannelID + "_" + series.GameType
	if _, exists := activeAutoSeries.Load(key); exists {
		return false // Đã có chuỗi chạy trong kênh này
	}
	series.Active = true
	activeAutoSeries.Store(key, series)
	return true
}

func StopAutoSeries(channelID, gameType string) {
	activeAutoSeries.Delete(channelID + "_" + gameType)
}

// ── HỆ THỐNG CACHE UI (CHỐNG RATELIMIT & MẤT GIAO DIỆN) ─────────────────────────

type CachedGameResult struct {
	GameName   string
	Content    string
	Embed      *discordgo.MessageEmbed
	Components []discordgo.MessageComponent
	Timestamp  time.Time
}

var lastChannelResults sync.Map

// CacheGameResult lưu trữ lại giao diện kết quả của ván game mới nhất trong kênh.
func CacheGameResult(channelID, gameName, content string, embed *discordgo.MessageEmbed, comps []discordgo.MessageComponent) {
	var compsCopy []discordgo.MessageComponent
	if comps != nil {
		compsCopy = make([]discordgo.MessageComponent, len(comps))
		copy(compsCopy, comps)
	}
	lastChannelResults.Store(channelID, CachedGameResult{
		GameName:   gameName,
		Content:    content,
		Embed:      embed,
		Components: compsCopy,
		Timestamp:  time.Now(),
	})
}
