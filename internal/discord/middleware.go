package discord

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/ui"
)

// ── Khoá session game (sync.Map, lưu trong RAM) ───────────────────────────────
// Key: userID (string) → Value: true
// Reset khi bot khởi động lại — chấp nhận được vì ván đang chơi cũng mất khi restart.
var userLocks sync.Map

// LockUser đặt khoá game cho user. Trả về true nếu khoá thành công.
// Trả về false nếu user đang bị khoá (đang chơi ván khác).
// Dùng LoadOrStore để đảm bảo atomic — không có race condition.
func LockUser(userID string) bool {
	_, loaded := userLocks.LoadOrStore(userID, true)
	return !loaded
}

// UnlockUser giải phóng khoá game. Trả về true nếu trước đó đang khoá (chống click đúp).
func UnlockUser(userID string) bool {
	_, loaded := userLocks.LoadAndDelete(userID)
	return loaded
}

// IsUserLocked kiểm tra user có đang trong ván game không.
func IsUserLocked(userID string) bool {
	_, ok := userLocks.Load(userID)
	return ok
}

// ── Hierarchy Phân Quyền ───────────────────────────────────────────────────────
type RoleLevel int

const (
	RoleUser RoleLevel = iota
	RoleAdmin
	RoleOwner
)

// IsEnvOwner kiểm tra ID có khớp với cấu hình .env (quyền lực tối cao, bất biến)
func IsEnvOwner(userID string) bool {
	ownerIDs := strings.Split(os.Getenv("OWNER_ID"), ",")
	for _, id := range ownerIDs {
		if strings.TrimSpace(id) == userID {
			return true
		}
	}
	return false
}

// GetEffectiveRole là Single Source of Truth xác định cấp bậc quyền hạn thực tế.
// Thứ tự: .env Owner > DB Owner > DB Admin > Discord Role Admin > User
func GetEffectiveRole(ctx context.Context, userRepo *repository.UserRepository, configRepo *repository.ConfigRepository, s *discordgo.Session, guildID, userID string, member *discordgo.Member) RoleLevel {
	if IsEnvOwner(userID) {
		return RoleOwner
	}
	isDBOwner, _ := userRepo.IsOwner(ctx, userID)
	if isDBOwner {
		return RoleOwner
	}
	isAdmin, _ := userRepo.IsAdmin(ctx, userID)
	if isAdmin {
		return RoleAdmin
	}
	if guildID != "" && s != nil {
		if member == nil {
			member, _ = s.State.Member(guildID, userID)
			if member == nil {
				member, _ = s.GuildMember(guildID, userID)
			}
		}
		if member != nil {
			adminRoles, err := configRepo.GetAdminRoles(ctx)
			if err == nil {
				for _, userRole := range member.Roles {
					for _, dbRole := range adminRoles {
						if userRole == dbRole {
							return RoleAdmin
						}
					}
				}
			}
		}
	}
	return RoleUser
}

// ── Kiểu hàm handler cho slash commands ──────────────────────────────────────
type HandlerFunc func(s *discordgo.Session, i *discordgo.InteractionCreate)

// ── Middleware kiểm tra quyền truy cập ───────────────────────────────────────
type Middleware struct {
	userRepo   *repository.UserRepository
	configRepo *repository.ConfigRepository
}

func NewMiddleware(userRepo *repository.UserRepository, configRepo *repository.ConfigRepository) *Middleware {
	return &Middleware{userRepo: userRepo, configRepo: configRepo}
}

// ctxWithTimeout tạo context với timeout 5 giây — dùng cho mọi DB query trong middleware.
func ctxWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// RequireOwner — chỉ user có role "owner" mới được qua.
func (m *Middleware) RequireOwner(next HandlerFunc) HandlerFunc {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		ctx, cancel := ctxWithTimeout()
		defer cancel()

		userID := interactionUserID(i)
		role := GetEffectiveRole(ctx, m.userRepo, m.configRepo, s, i.GuildID, userID, i.Member)
		if role < RoleOwner {
			respondEphemeral(s, i, ui.Emojis.Block+" Lệnh này chỉ dành cho Owner.")
			return
		}
		next(s, i)
	}
}

// RequireAdmin — user có role "admin" hoặc "owner" mới được qua.
func (m *Middleware) RequireAdmin(next HandlerFunc) HandlerFunc {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		ctx, cancel := ctxWithTimeout()
		defer cancel()

		userID := interactionUserID(i)
		role := GetEffectiveRole(ctx, m.userRepo, m.configRepo, s, i.GuildID, userID, i.Member)
		if role < RoleAdmin {
			respondEphemeral(s, i, ui.Emojis.Block+" Lệnh này chỉ dành cho Admin.")
			return
		}
		next(s, i)
	}
}

// RequireWhitelistedChannel — chỉ cho phép kênh nằm trong danh sách whitelist.
// Nếu whitelist rỗng (chưa config) → cho phép tất cả kênh.
func (m *Middleware) RequireWhitelistedChannel(next HandlerFunc) HandlerFunc {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		ctx, cancel := ctxWithTimeout()
		defer cancel()

		channelID := i.ChannelID
		ok, err := m.configRepo.IsChannelWhitelisted(ctx, channelID)
		log.Printf("[CHANNEL_CHECK] channelId=%s | isAllowed=%v | err=%v", channelID, ok, err)
		if err != nil || !ok {
			respondEphemeral(s, i, ui.Emojis.Block+" Lệnh casino không được phép ở kênh này.")
			return
		}
		next(s, i)
	}
}

// RequireUnlocked — từ chối nếu user đang trong ván game khác.
// Kết hợp với LockUser/UnlockUser trong game handler.
func RequireUnlocked(next HandlerFunc) HandlerFunc {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		userID := interactionUserID(i)
		if IsUserLocked(userID) {
			respondEphemeral(s, i, ui.Emojis.Loading+" Bạn đang trong một ván đấu. Hãy kết thúc ván hiện tại trước.")
			return
		}
		next(s, i)
	}
}

// Chain ghép nhiều middleware lại — middleware đầu tiên là lớp ngoài cùng.
// Ví dụ: Chain(handler, RequireWhitelistedChannel, RequireUnlocked)
// → Kiểm tra whitelist trước, rồi kiểm tra lock, rồi mới chạy handler.
func Chain(h HandlerFunc, middlewares ...func(HandlerFunc) HandlerFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// ── Hàm tiện ích nội bộ ───────────────────────────────────────────────────────

// interactionUserID lấy Discord user ID từ interaction (hỗ trợ cả server và DM).
func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

// respondEphemeral gửi tin nhắn ẩn qua slash command interaction.
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
