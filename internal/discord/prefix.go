package discord

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ui"
)

// Ký tự bắt đầu lệnh — đổi ở đây nếu muốn dùng prefix khác (ví dụ: "$", "?")
const commandPrefix = "!"

// discordIDPattern nhận diện Discord user ID từ 3 dạng:
//   - Mention:       <@123456789> hoặc <@!123456789>
//   - Link profile:  discord.com/users/123456789012345678
//   - ID thuần:      123456789012345678  (17–19 chữ số)
var discordIDPattern = regexp.MustCompile(
	`(?:<@!?(\d{17,19})>|discord\.com/users/(\d{17,19})|(?:^|\s)(\d{17,19})(?:\s|$))`,
)

// ExtractUserID tách Discord user ID từ chuỗi bất kỳ.
// Dùng cho lệnh /pay hỗ trợ mention, link profile, và raw ID.
func ExtractUserID(s string) (string, bool) {
	m := discordIDPattern.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	for _, g := range m[1:] {
		if g != "" {
			return g, true
		}
	}
	return "", false
}

// parseCommand phân tích nội dung tin nhắn thành (lệnh, tham số).
// Trả về ("", nil) nếu không bắt đầu bằng prefix hoặc không có lệnh.
func parseCommand(content, prefix string) (string, []string) {
	if len(content) < len(prefix) {
		return "", nil
	}

	// Kiểm tra prefix không phân biệt chữ hoa/thường (ví dụ: w và W đều hợp lệ)
	if !strings.EqualFold(content[:len(prefix)], prefix) {
		return "", nil
	}

	parts := strings.Fields(content[len(prefix):])
	if len(parts) == 0 {
		return "", nil
	}
	return strings.ToLower(parts[0]), parts[1:]
}

func (b *Bot) GetPrefix(ctx context.Context) string {
	cfg, err := b.configRepo.Get(ctx)
	if err == nil && cfg != nil && cfg.Prefix != "" {
		return cfg.Prefix
	}
	return commandPrefix
}

// onMessage xử lý sự kiện MessageCreate — nhận và điều phối các lệnh /
func (b *Bot) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Tránh panic nếu discordgo không thể đọc hoàn toàn tin nhắn (do Discord cập nhật tính năng mới)
	if m == nil || m.Message == nil || m.Author == nil {
		return
	}

	if m.Author.Bot {
		return
	}

	isDM := m.GuildID == ""
	if !isDM {
		b.ProcessAIMessage(s, m)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	prefix := b.GetPrefix(ctx)

	cmd, args := parseCommand(m.Content, prefix)
	if cmd == "" {
		return
	}
	if isDM && !isPrivateCommandAllowed(cmd) {
		_, _ = s.ChannelMessageSend(m.ChannelID, ui.Emojis.Block+" Lệnh này chỉ dùng trong máy chủ. Trong DM, bạn có thể dùng `!help`, `!balance`, `!history`, `!info` hoặc `!upd`.")
		return
	}

	// Giới hạn tốc độ: 1 lệnh / giây / user
	if !b.rateLimiter.Allow(m.Author.ID) {
		msg, _ := s.ChannelMessageSend(m.ChannelID, ui.Emojis.Loading+" Bạn đang gửi lệnh quá nhanh. Hãy chờ 1 giây.")
		if msg != nil {
			deleteAfter(s, m.ChannelID, msg.ID, 3*time.Second)
		}
		return
	}

	commandStarted := time.Now()
	defer func() {
		b.getMonitor().RecordCommand(time.Since(commandStarted))
	}()

	switch cmd {
	// ── Quản trị ────────────────────────────────────────────────────
	case "admin":
		b.cmdAdmin(s, m, args)
	case "adminmenu", "panel", "dev":
		b.cmdAdminMenu(s, m, args)
	case "limit":
		b.cmdLimit(s, m, args)
	case "whitelist":
		b.cmdWhitelist(s, m, args)
	case "removelist", "clearlist":
		b.cmdRemoveList(s, m, args)
	case "info":
		b.cmdRender(s, m)
	case "upd":
		b.cmdUpdateRender(s, m)
	case "balance":
		b.cmdCash(s, m, args)
	case "history":
		b.cmdHistory(s, m, args)

	// ── Moderation ──────────────────────────────────────────────────
	case "delete", "clear", "purge":
		b.cmdDelete(s, m, cmd, args)

	// ── Debug & Fix Data ────────────────────────────────────────────
	case "debug_channel_config":
		b.cmdDebugChannel(s, m, args)

	// ── Kill Switch (Dừng mọi tác vụ auto/dọn dẹp) ──────────────────
	case "stop":
		b.cmdStop(s, m, args)

	// ── Ví tiền ─────────────────────────────────────────────────────
	case "pay", "chuyentien":
		b.cmdPay(s, m, args)
	case "cash", "tien":
		b.cmdCash(s, m, args)

	// ── Game ────────────────────────────────────────────────────────
	case "taixiu", "tx":
		b.cmdTaixiu(s, m, args)
	case "baucua", "bc":
		b.cmdBaucua(s, m, args)
	case "xidach", "xd", "blackjack", "bj":
		b.cmdBlackjack(s, m, args)
	case "baicao", "bcao":
		b.cmdBaicao(s, m, args)
	case "restore":
		b.cmdRestore(s, m, args)
	case "host":
		b.cmdHost(s, m, args)
	case "helpcasino", "help", "casino":
		b.cmdHelp(s, m, args)
	}
}

func isPrivateCommandAllowed(command string) bool {
	switch command {
	case "help", "helpcasino", "casino", "info",
		"upd", "balance", "cash", "tien", "history":
		return true
	default:
		return false
	}
}
