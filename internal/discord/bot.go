package discord

// ═══════════════════════════════════════════════════════════════════════════════
// bot.go — KHỞI TẠO VÀ ĐIỀU PHỐI BOT
//
// File này làm 3 việc:
//   1. Khởi tạo bot (kết nối Discord, đăng ký handler)
//   2. Đăng ký Slash Commands lên Discord
//   3. Điều phối mọi interaction đến đúng handler
//
// VỀ GUILD_ID (giải thích chi tiết):
//
//   Discord có 2 loại Slash Command:
//
//   ┌─────────────────┬─────────────────────────────────────────────────────┐
//   │  Guild Command  │ Chỉ hiện trong 1 server cụ thể. Active NGAY LẬP TỨC│
//   │  (có Guild ID)  │ → Dùng khi dev/test để tránh chờ                   │
//   ├─────────────────┼─────────────────────────────────────────────────────┤
//   │ Global Command  │ Hiện trong MỌI server bot đã vào. Mất ~1 giờ sync  │
//   │  (Guild ID = "")│ → Dùng cho production                               │
//   └─────────────────┴─────────────────────────────────────────────────────┘
//
//   Trong code bên dưới:
//     b.session.ApplicationCommandCreate(..., b.guildID, cmd)
//     Khi guildID = "" → Discord tự hiểu là Global Command
//
//   QUAN TRỌNG: Lệnh game ! (taixiu, bj, rtb, doanxu) KHÔNG bị ảnh hưởng.
//   GUILD_ID chỉ ảnh hưởng /balance /transfer /history /admin.
// ═══════════════════════════════════════════════════════════════════════════════

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/service"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

type Bot struct {
	session         *discordgo.Session
	wallet          *service.WalletService
	userRepo        *repository.UserRepository
	configRepo      *repository.ConfigRepository
	casinoConfigSvc *service.CasinoConfigService
	mw              *Middleware
	rateLimiter     *RateLimiter
	guildID         string // "" = global slash commands (~1h), có ID = guild command (ngay lập tức)
	actionLocks     sync.Map
}

func NewBot(
	token string,
	wallet *service.WalletService,
	userRepo *repository.UserRepository,
	configRepo *repository.ConfigRepository,
	casinoConfigSvc *service.CasinoConfigService,
	guildID string,
) (*Bot, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("tạo session thất bại: %w", err)
	}

	// IntentsMessageContent BẮT BUỘC để nhận nội dung tin nhắn (lệnh !)
	// Phải bật trong Discord Developer Portal → Bot → Privileged Gateway Intents
	dg.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	b := &Bot{
		session:         dg,
		wallet:          wallet,
		userRepo:        userRepo,
		configRepo:      configRepo,
		casinoConfigSvc: casinoConfigSvc,
		mw:              NewMiddleware(userRepo, configRepo),
		rateLimiter:     NewRateLimiter(time.Second), // 1 lệnh/giây/user
		guildID:         guildID,
	}
	return b, nil
}

// Lấy Role Thực tế (Bọc qua Middleware)
func (b *Bot) GetRole(ctx context.Context, s *discordgo.Session, guildID, userID string, member *discordgo.Member) RoleLevel {
	return GetEffectiveRole(ctx, b.userRepo, b.configRepo, s, guildID, userID, member)
}

func (b *Bot) TryLockAction(key string, ttl time.Duration) bool {
	_, loaded := b.actionLocks.LoadOrStore(key, time.Now().Add(ttl))
	if loaded {
		return false // Đang bị khoá
	}
	time.AfterFunc(ttl, func() {
		b.actionLocks.Delete(key)
	})
	return true
}

func (b *Bot) Start() error {
	// LOG ĐỂ KIỂM TRA CÓ BỊ RENDER CHẠY NHIỀU INSTANCE CÙNG LÚC KHÔNG
	log.Printf("[BOT_START] pid=%d instance=%s", os.Getpid(), os.Getenv("RENDER_INSTANCE_ID"))

	b.session.AddHandler(b.onInteraction) // xử lý slash commands + nút bấm game
	b.session.AddHandler(b.onMessage)     // xử lý lệnh ! prefix
	b.session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("[Bot] Đã đăng nhập: %s#%s", r.User.Username, r.User.Discriminator)
	})

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("mở kết nối WebSocket thất bại: %w", err)
	}

	if err := b.registerCommands(); err != nil {
		return fmt.Errorf("đăng ký slash commands thất bại: %w", err)
	}

	log.Println("[Bot] Đang chạy. Nhấn CTRL-C để dừng.")
	return nil
}

func (b *Bot) Stop() {
	_ = b.session.Close()
}

// ── Định nghĩa Slash Commands (/balance, /transfer, /history, /admin) ─────────
var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "balance",
		Description: "Xem số dư của bạn hoặc người khác",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "Người dùng cần xem (để trống = bạn)",
				Required:    false,
			},
		},
	},
	{
		Name:        "transfer",
		Description: "Chuyển tiền cho người khác",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "Người nhận",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "amount",
				Description: "Số tiền cần chuyển",
				Required:    true,
				MinValue:    ptrFloat(1),
			},
		},
	},
	{
		Name:        "history",
		Description: "Xem lịch sử giao dịch của bạn",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "limit",
				Description: "Số giao dịch gần nhất (mặc định 10, tối đa 20)",
				Required:    false,
			},
		},
	},
	{
		Name:        "admin",
		Description: "Lệnh quản trị",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "give",
				Description: "Cộng tiền cho người dùng",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Người nhận", Required: true},
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "amount", Description: "Số tiền", Required: true, MinValue: ptrFloat(1)},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "take",
				Description: "Trừ tiền của người dùng",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Người dùng", Required: true},
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "amount", Description: "Số tiền", Required: true, MinValue: ptrFloat(1)},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "setrole",
				Description: "Đặt vai trò cho người dùng (owner only)",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Người dùng", Required: true},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "role",
						Description: "Vai trò",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "admin", Value: models.RoleAdmin},
							{Name: "user", Value: models.RoleUser},
						},
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "whitelist",
				Description: "Thêm/xóa kênh được phép dùng casino",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "action",
						Description: "Thêm hoặc xóa kênh khỏi whitelist",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "add", Value: "add"},
							{Name: "remove", Value: "remove"},
						},
					},
					{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Kênh", Required: true},
				},
			},
		},
	},
	{
		Name:        "gamehistory",
		Description: "Xem lịch sử các ván game gần nhất",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "game",
				Description: "Chọn danh mục game",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Tài Xỉu", Value: "taixiu"},
					{Name: "Xì Dách", Value: "xidach"},
					{Name: "Bầu Cua", Value: "baucua"},
					{Name: "Bài Cào", Value: "baicao"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "user",
				Description: "Người dùng cần xem lịch sử game",
				Required:    false,
			},
		},
	},
	{
		Name:        "host",
		Description: "Kéo lại bàn game bạn đang làm cái về cuối kênh chat.",
	},
	{
		Name:        "reset",
		Description: "Xóa dữ liệu hệ thống (Admin/Owner)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "all",
				Description: "Khôi phục cài đặt gốc toàn hệ thống (Chỉ Owner)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "confirm",
						Description: "Gõ XACNHAN để đồng ý xóa sạch",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "game",
				Description: "Xóa dữ liệu bảng xếp hạng game",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Tên game cần xóa",
						Required:    true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "Tất cả game", Value: "all"},
							{Name: "Tài Xỉu", Value: "taixiu"},
							{Name: "Xì Dách", Value: "xidach"},
							{Name: "Bầu Cua", Value: "baucua"},
							{Name: "Bài Cào", Value: "baicao"},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "user",
						Description: "Chỉ xóa của người chơi này (để trống = xóa tất cả)",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "user",
				Description: "Reset số dư của người chơi về 0",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        "target",
						Description: "Người chơi cần reset số dư",
						Required:    true,
					},
				},
			},
		},
	},
	{
		Name:        "cc",
		Description: "Whiskey -Vận Mệnh Chói Lọi!",
	},
}

func (b *Bot) registerCommands() error {
	// Dùng BulkOverwrite để xóa sạch các lệnh cũ bị rác/nhân bản và cập nhật lệnh mới
	_, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, b.guildID, commands)
	if err != nil {
		return fmt.Errorf("bulk overwrite slash commands thất bại: %w", err)
	}
	log.Printf("[Bot] Đã đồng bộ %d slash commands thành công (Chống duplicate).", len(commands))
	return nil
}

// ── Router chính: phân loại interaction về đúng handler ──────────────────────

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// KHÓA TRÙNG LẶP INTERACTION (Chống lỗi 10062 Unknown Interaction và Double Audit)
	if !b.TryLockAction("int_"+i.ID, 15*time.Second) {
		log.Printf("[INTERACTION] Dropped duplicate interaction_id=%s type=%v", i.ID, i.Type)
		return
	}

	// Rate limit: Chống spam click nút bấm & lệnh / (1 giây / 1 thao tác)
	userID := interactionUserID(i)
	if userID != "" && !b.rateLimiter.Allow(userID) {
		if i.Type == discordgo.InteractionMessageComponent || i.Type == discordgo.InteractionApplicationCommand {
			respondEphemeral(s, i, ui.Emojis.Loading+" Bạn đang thao tác quá nhanh. Hãy chờ 1 giây.")
		}
		return
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.routeSlashCommand(s, i) // xử lý /balance, /transfer, v.v.
	case discordgo.InteractionMessageComponent:
		b.routeButtonClick(s, i) // xử lý nút bấm của game
	case discordgo.InteractionModalSubmit:
		b.routeModalSubmit(s, i) // xử lý form nhập liệu từ admin menu
	}
}

func (b *Bot) routeSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	handlers := map[string]HandlerFunc{
		"balance":     b.handleBalance,
		"transfer":    Chain(b.handleTransfer, b.mw.RequireWhitelistedChannel, RequireUnlocked),
		"history":     b.handleHistory,
		"gamehistory": b.handleGameHistory,
		"admin":       Chain(b.handleAdmin, b.mw.RequireAdmin),
		"reset":       Chain(b.handleReset, b.mw.RequireAdmin),
		"host":        b.handleHostCommand,
		"cc":          b.handleSlashCC,
	}
	name := i.ApplicationCommandData().Name
	if h, ok := handlers[name]; ok {
		h(s, i)
	}
}

// routeButtonClick điều phối nút bấm dựa trên prefix của customID.
// Định dạng customID: "taixiu:tai", "bj:hit:123456", "rtb:red:123456", "dx:heads:123456:1000"
func (b *Bot) routeButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	id := i.MessageComponentData().CustomID
	switch {
	case strings.HasPrefix(id, "taixiu:"):
		b.handleTaixiuButton(s, i)
	case strings.HasPrefix(id, "bc:"):
		b.handleBaucuaButton(s, i)
	case strings.HasPrefix(id, "xd:"):
		b.handleBlackjackButton(s, i)
	case strings.HasPrefix(id, "bcao:"):
		b.handleBaicaoButton(s, i)
	case strings.HasPrefix(id, "admin:"):
		b.handleAdminComponent(s, i)
	case strings.HasPrefix(id, "bal:page:"):
		b.handleBalancePagination(s, i)
	case strings.HasPrefix(id, "gcfg:"):
		// Trỏ luồng xử lý tương tác của Bảng điều khiển Casino (Config Panel)
		b.handleCasinoAdminInteraction(s, i)
	case strings.HasPrefix(id, "wallet:hist:"):
		b.handleWalletPagination(s, i)
	}
}

func (b *Bot) routeModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	id := i.ModalSubmitData().CustomID
	if strings.HasPrefix(id, "admin:modal:") {
		b.handleAdminModal(s, i)
	} else if strings.HasPrefix(id, "taixiu:modal:") {
		b.handleTaixiuModalSubmit(s, i)
	} else if strings.HasPrefix(id, "bc:modal:") {
		b.handleBaucuaModalSubmit(s, i)
	} else if strings.HasPrefix(id, "gcfg:") {
		// Trỏ luồng xử lý form nhập liệu của Bảng điều khiển Casino
		b.handleCasinoAdminModal(s, i)
	}
}

// ── HỆ THỐNG TÍN DỤNG (DEBT LIMIT) ───────────────────────────────────────────

// GetEffectiveDebtLimit tính toán hạn mức âm tiền lớn nhất giữa cá nhân và Role.
func (b *Bot) GetEffectiveDebtLimit(ctx context.Context, s *discordgo.Session, guildID, userID string) int64 {
	var roleLimit int64
	// Lấy Config Role (Giả định configRepo.Get() trả về struct có map DebtRoleLimits)
	cfg, err := b.configRepo.Get(ctx)
	if err == nil && cfg.DebtRoleLimits != nil && guildID != "" {
		// CỰC KỲ QUAN TRỌNG: Đọc từ RAM (Cache) trước để chống Rate-Limit Discord
		member, err := s.State.Member(guildID, userID)
		if err != nil {
			member, err = s.GuildMember(guildID, userID) // Fallback nếu Cache bị miss
		}
		if err == nil && member != nil {
			for _, r := range member.Roles {
				if limit, ok := cfg.DebtRoleLimits[r]; ok && limit > roleLimit {
					roleLimit = limit
				}
			}
		}
	}

	var personalLimit int64
	// Lấy Personal Limit
	user, err := b.userRepo.GetByID(ctx, userID)
	if err == nil && user != nil && user.DebtEnabled {
		personalLimit = user.DebtLimit
	}

	if personalLimit > roleLimit {
		return personalLimit
	}
	return roleLimit
}

// SafeBetDeduct là hàm DUY NHẤT tất cả các game phải dùng để trừ tiền cược.
// Hàm này tự động quét Role và Hạn mức cá nhân để cho phép đánh âm.
func (b *Bot) SafeBetDeduct(ctx context.Context, s *discordgo.Session, guildID, userID string, amount int64) error {
	if amount <= 0 {
		return repository.ErrInvalidAmount
	}

	// 1. Tính toán hạn mức âm tiền tối đa
	debtLimit := b.GetEffectiveDebtLimit(ctx, s, guildID, userID)

	// 2. Đẩy xuống tầng Wallet thực thi Atomic Check
	// YÊU CẦU: Bạn cần cập nhật hàm BetDeduct trong wallet.go để nhận thêm tham số debtLimit
	// logic: if balance + debtLimit < amount { return ErrInsufficientCredit }
	return b.wallet.BetDeductWithDebt(ctx, userID, amount, debtLimit)
}

// ── Slash command handlers (/balance, /transfer, /history, /admin) ────────────

func (b *Bot) handleBalance(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := interactionUserID(i)
	// 1. ACK NGAY LẬP TỨC để tránh treo Ứng dụng không phản hồi
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Printf("[/balance] defer failed user_id=%s interaction_id=%s err=%v", userID, i.ID, err)
		return
	}

	// 2. Set timeout chặt chẽ 3 giây cho mọi query DB liên quan đến lệnh này
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var targetID string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "user" && opt.UserValue(s) != nil {
			targetID = opt.UserValue(s).ID
		}
	}

	// CHẾ ĐỘ 1: XEM CÁ NHÂN
	if targetID != "" {
		result, err := b.wallet.GetBalance(ctx, targetID)
		if err != nil {
			log.Printf("[/balance] no wallet found, treating as default wallet user_id=%s default_balance=0", targetID)
			errMsg := ui.Emojis.Error + " Linh hồn này chưa từng ký kết khế ước tại Vực Sâu."
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &errMsg})
			return
		}

		log.Printf("[/balance] user_id=%s guild_id=%s source=DB db=%s collection=users filter={\"_id\":\"%s\"} balance=%v", targetID, i.GuildID, b.userRepo.GetDBName(), targetID, result.Balance)

		debtLimit := b.GetEffectiveDebtLimit(ctx, s, i.GuildID, targetID)
		available := result.Balance + debtLimit

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("\n%s **Linh hồn:** <@%s>\n", ui.Emojis.ArrowBlack, targetID))
		sb.WriteString(fmt.Sprintf("> %s **Tài sản thực:** %s\n", ui.Emojis.Bank, coinDisplay(result.Balance)))
		if debtLimit > 0 {
			sb.WriteString(fmt.Sprintf("> %s **Hạn mức Tín dụng (Âm tối đa):** %s\n", ui.Emojis.Settings, formatCoins(debtLimit)))
			sb.WriteString(fmt.Sprintf("> %s **Khả dụng cược:** %s\n", ui.Emojis.Success, formatCoins(available)))
		} else {
			sb.WriteString(fmt.Sprintf("> %s **Hạn mức Tín dụng:** Không có\n", ui.Emojis.Settings))
		}
		embed := view.NewEmbed(ui.Emojis.Bank+" KHẾ ƯỚC TÀI SẢN", sb.String())
		embed.Color = 0x00FFFF // Xanh cyan huyền ảo
		embed.Footer = &discordgo.MessageEmbedFooter{Text: "Whiskey: The Abyss Dealer | Dùng /balance để xem Top 10"}

		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &[]*discordgo.MessageEmbed{embed}})
		return
	}

	// CHẾ ĐỘ 2: TOP BẢNG XẾP HẠNG (CÓ PHÂN TRANG)
	b.renderBalanceLeaderboard(s, i, 1, false)
}

func (b *Bot) renderBalanceLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate, page int, isEdit bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pageSize := 10
	entries, totalLogs, err := b.userRepo.GetBalanceLeaderboardPaginated(ctx, page, pageSize)
	if err != nil {
		errMsg := ui.Emojis.Error + " Bảng xếp hạng đang phản hồi chậm. Hãy thử lại sau."
		if isEdit {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: errMsg, Flags: discordgo.MessageFlagsEphemeral})
		} else {
			_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &errMsg})
		}
		return
	}

	totalPages := int((totalLogs + int64(pageSize) - 1) / int64(pageSize))
	if totalPages == 0 {
		totalPages = 1
	}

	var sb strings.Builder
	if len(entries) == 0 {
		sb.WriteString("\n_Vực sâu tĩnh lặng... Chưa có linh hồn nào cất giữ tài sản._")
	} else {
		sb.WriteString(ui.Emojis.BlackWingLeft + ui.Emojis.Header + ui.Emojis.BlackWingRight + "\n\n")
		rankEmojis := []string{ui.Emojis.Rank1, ui.Emojis.Rank2, ui.Emojis.Rank3}

		startIndex := (page - 1) * pageSize
		for idx, entry := range entries {
			globalIdx := startIndex + idx
			var rankStr string
			if globalIdx < 3 {
				rankStr = rankEmojis[globalIdx]
			} else {
				rankStr = ui.Emojis.Dot
			}

			sb.WriteString(fmt.Sprintf("%s **#%d** <@%s>\n %s Nắm giữ: **%s** %s\n", rankStr, globalIdx+1, entry.UserID, ui.Emojis.BlueStar, formatCoins(entry.Balance), ui.Emojis.Coin))
		}
	}

	embed := view.NewEmbed(fmt.Sprintf("%s **BẢNG XẾP HẠNG PHÚ HỘ**", ui.Emojis.Bank), sb.String())
	embed.Color = 0x00FFFF
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Tổng: %d linh hồn • Dùng /balance user:@user để xem cá nhân", totalLogs)}

	var components []discordgo.MessageComponent
	if totalPages > 1 {
		btnPrev := discordgo.Button{
			CustomID: fmt.Sprintf("bal:page:%d", page-1),
			Label:    "◀ Trước",
			Style:    discordgo.SecondaryButton,
			Disabled: page <= 1,
		}
		btnNext := discordgo.Button{
			CustomID: fmt.Sprintf("bal:page:%d", page+1),
			Label:    "Sau ▶",
			Style:    discordgo.SecondaryButton,
			Disabled: page >= totalPages,
		}
		btnPage := discordgo.Button{
			CustomID: "bal:page_indicator",
			Label:    fmt.Sprintf("Trang %d/%d", page, totalPages),
			Style:    discordgo.SecondaryButton,
			Disabled: true,
		}
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{btnPrev, btnPage, btnNext},
		})
	}

	if isEdit {
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})
	} else {
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &components,
		})
	}
}

func (b *Bot) handleBalancePagination(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 3 {
		return
	}
	page, _ := strconv.Atoi(parts[2])

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})

	b.renderBalanceLeaderboard(s, i, page, true)
}

func (b *Bot) handleTransfer(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// 1. CHỐNG TIMEOUT: Defer ngay lập tức
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fromID := interactionUserID(i)
	var toUser *discordgo.User
	var amount int64

	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "user" {
			toUser = opt.UserValue(s)
		} else if opt.Name == "amount" {
			amount = opt.IntValue()
		}
	}

	if toUser == nil || amount <= 0 {
		followupEphemeral(s, i, ui.Emojis.Error+" Vui lòng cung cấp đủ người nhận và số tiền hợp lệ.")
		return
	}

	if toUser.Bot {
		followupEphemeral(s, i, ui.Emojis.Error+" Không thể chuyển tiền cho bot.")
		return
	}
	if toUser.ID == fromID {
		followupEphemeral(s, i, ui.Emojis.Error+" Không thể chuyển tiền cho chính mình.")
		return
	}

	// Dùng interaction ID làm Idempotency Key để chống double-click nhân đôi tiền
	if err := b.wallet.Transfer(ctx, fromID, toUser.ID, amount, i.ID); err != nil {
		followupEphemeral(s, i, friendlyErr(err)+" "+ui.Emojis.Error)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **Kẻ trao:** <@%s>\n", ui.Emojis.ArrowBlack, fromID))
	sb.WriteString(fmt.Sprintf("%s **Kẻ nhận:** <@%s>\n", ui.Emojis.ArrowBlack, toUser.ID))
	sb.WriteString(fmt.Sprintf("> %s **Tài sản dịch chuyển:** %s\n\n", ui.Emojis.Bank, coinDisplay(amount)))
	sb.WriteString("_Vực sâu đã chứng giám sự trao đổi này._")

	embed := view.NewEmbed(ui.Emojis.Bank+" KHẾ ƯỚC CHUYỂN GIAO", sb.String())
	embed.Color = 0x00FFFF // Xanh cyan huyền ảo
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Sổ cái vực sâu đã lưu vết. | " + GetRandomTip(b.GetPrefix(ctx))}

	_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) handleHistory(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID := interactionUserID(i)
	limit := int64(10)
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "limit" {
			v := opt.IntValue()
			if v > 0 && v <= 20 {
				limit = v
			}
		}
	}

	logs, err := b.wallet.History(ctx, userID, limit)
	if err != nil || len(logs) == 0 {
		respondEphemeral(s, i, ui.Emojis.Empty+" Không có giao dịch nào.")
		return
	}

	currentBal, _ := b.wallet.GetBalance(ctx, userID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s **Chủ tài khoản:** <@%s>\n", ui.Emojis.User, userID))
	if currentBal != nil {
		sb.WriteString(fmt.Sprintf("%s **Số dư hiện tại:** %s %s\n", ui.Emojis.Bank, formatCoins(currentBal.Balance), ui.Emojis.Coin))
	}
	sb.WriteString(fmt.Sprintf("%s **Trích xuất %d giao dịch gần nhất:**\n", ui.Emojis.Book, len(logs)))

	sb.WriteString("```diff\n")
	sb.WriteString("  THỜI GIAN       | BIẾN ĐỘNG    | NỘI DUNG\n")
	sb.WriteString("------------------|--------------|------------------------------\n")

	vnZone := time.FixedZone("ICT", 7*3600) // Chuyển đổi về múi giờ Việt Nam (UTC+7)
	for _, tx := range logs {
		prefix := " "
		if tx.Amount > 0 {
			prefix = "+"
		} else if tx.Amount < 0 {
			prefix = "-"
		}

		absAmount := tx.Amount
		if absAmount < 0 {
			absAmount = -absAmount
		}

		amountStr := fmt.Sprintf("%s%s", prefix, formatCoins(absAmount))
		desc := strings.ReplaceAll(tx.Description, "\n", " ")

		sb.WriteString(fmt.Sprintf("%s %s | %-12s | %s\n",
			prefix,
			tx.CreatedAt.In(vnZone).Format("01/02 15:04:05"),
			amountStr,
			desc,
		))
	}
	sb.WriteString("```")

	embed := view.GameEmbed(ui.Emojis.History+" Sổ Cái Giao Dịch", ui.Emojis.RewardBC+"\n\n"+sb.String())
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Abyss Casino Audit Log • %d giao dịch | %s", len(logs), GetRandomTip(b.GetPrefix(ctx)))}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral, // chỉ người hỏi mới thấy lịch sử của mình
		},
	})
}

func (b *Bot) handleAdmin(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// 1. CHỐNG TIMEOUT: Defer ngầm
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	adminID := interactionUserID(i)
	isOwner, _ := b.userRepo.IsOwner(ctx, adminID)

	opts := i.ApplicationCommandData().Options
	if len(opts) == 0 {
		return
	}
	sub := opts[0]

	switch sub.Name {
	case "give":
		if !isOwner {
			followupEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể dùng chức năng này.")
			return
		}
		var target *discordgo.User
		var amount int64
		for _, opt := range sub.Options {
			if opt.Name == "user" {
				target = opt.UserValue(s)
			} else if opt.Name == "amount" {
				amount = opt.IntValue()
			}
		}
		if target == nil {
			return
		}
		if err := b.wallet.AdminGive(ctx, target.ID, adminID, amount); err != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" "+friendlyErr(err))
			return
		}

		msg := ui.Emojis.Success + " Đã cộng " + coinDisplay(amount) + " cho <@" + target.ID + ">"
		followupEphemeral(s, i, msg)

	case "take":
		if !isOwner {
			followupEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể dùng chức năng này.")
			return
		}
		var target *discordgo.User
		var amount int64
		for _, opt := range sub.Options {
			if opt.Name == "user" {
				target = opt.UserValue(s)
			} else if opt.Name == "amount" {
				amount = opt.IntValue()
			}
		}
		if target == nil {
			return
		}
		if err := b.wallet.AdminTake(ctx, target.ID, adminID, amount); err != nil {
			respondEphemeral(s, i, ui.Emojis.Error+" "+friendlyErr(err))
			return
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: ui.Emojis.Success + " Đã trừ " + coinDisplay(amount) + " của <@" + target.ID + ">",
			},
		})

	case "setrole":
		// Chỉ Owner mới có thể thay đổi role
		actorRole := b.GetRole(ctx, s, i.GuildID, adminID, i.Member)
		if actorRole < RoleOwner {
			respondEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể đặt vai trò.")
			return
		}
		var target *discordgo.User
		var role string
		for _, opt := range sub.Options {
			if opt.Name == "user" {
				target = opt.UserValue(s)
			} else if opt.Name == "role" {
				role = opt.StringValue()
			}
		}
		if target == nil {
			return
		}

		// NEW: Bảo vệ Owner tuyệt đối
		targetRole := b.GetRole(ctx, s, i.GuildID, target.ID, nil)
		if targetRole == RoleOwner {
			log.Printf("[PERMISSION_DENIED] reason=cannot_modify_env_owner actorId=%s targetId=%s", adminID, target.ID)
			followupEphemeral(s, i, ui.Emojis.Error+" Người này là Owner hệ thống, không thể thay đổi quyền.")
			return
		}

		if err := b.userRepo.SetRole(ctx, target.ID, role); err != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Lỗi khi đặt vai trò.")
			return
		}
		log.Printf("[PERMISSION_CHANGE] actorId=%s targetId=%s requestedRole=%s action=setrole result=success", adminID, target.ID, role)
		followupEphemeral(s, i, ui.Emojis.Success+" Đã đặt vai trò **"+role+"** cho <@"+target.ID+">")

	case "whitelist":
		actorRole := b.GetRole(ctx, s, i.GuildID, adminID, i.Member)
		if actorRole < RoleOwner {
			followupEphemeral(s, i, ui.Emojis.Block+" Chỉ Owner mới có thể quản lý whitelist.")
			return
		}
		var action string
		var channel *discordgo.Channel
		for _, opt := range sub.Options {
			if opt.Name == "action" {
				action = opt.StringValue()
			} else if opt.Name == "channel" {
				channel = opt.ChannelValue(s)
			}
		}
		if channel == nil {
			return
		}
		var err error
		if action == "add" {
			err = b.configRepo.AddWhitelistedChannel(ctx, channel.ID)
		} else {
			err = b.configRepo.RemoveWhitelistedChannel(ctx, channel.ID)
		}
		if err != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Lỗi khi cập nhật whitelist.")
			return
		}
		verb := "thêm vào"
		if action == "remove" {
			verb = "xóa khỏi"
		}
		followupEphemeral(s, i, ui.Emojis.Success+" Đã "+verb+" whitelist: <#"+channel.ID+">")
	}
}

// ── Hàm tiện ích ─────────────────────────────────────────────────────────────

// friendlyErr chuyển lỗi hệ thống thành thông báo tiếng Việt thân thiện.
func friendlyErr(err error) string {
	switch err {
	case repository.ErrInsufficientBalance:
		return "Số dư không đủ."
	case repository.ErrUserNotFound:
		return "Người dùng chưa có tài khoản."
	case repository.ErrSelfTransfer:
		return "Không thể chuyển tiền cho chính mình."
	case repository.ErrInvalidAmount:
		return "Số tiền không hợp lệ."
	default:
		return "Lỗi hệ thống, vui lòng thử lại."
	}
}

func ptrFloat(v float64) *float64 { return &v }

// Dòng này tránh lỗi "imported and not used" cho strconv — dùng trong handleHistory.
var _ = strconv.FormatInt

// handleGameHistory hiển thị danh sách các ván đấu theo danh mục
func (b *Bot) handleGameHistory(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var gameName string
	var targetUserID string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "game" {
			gameName = opt.StringValue()
		} else if opt.Name == "user" && opt.UserValue(s) != nil {
			targetUserID = opt.UserValue(s).ID
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	var title string
	var gameLabel string
	if gameName == "taixiu" {
		gameLabel = "Tài Xỉu"
		title = "LIST TÀI XỈU"
	} else if gameName == "baucua" {
		gameLabel = "Bầu Cua"
		title = "LIST BẦU CUA"
	} else if gameName == "baicao" {
		gameLabel = "Bài Cào"
		title = "LIST BÀI CÀO"
	} else {
		gameLabel = "Xì Dách"
		title = "LIST XÌ DÁCH"
	}

	var sb strings.Builder
	if targetUserID != "" {
		entry, err := b.userRepo.GetGameHistoryForUser(context.Background(), gameName, targetUserID)
		if err != nil {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: ui.Emojis.Error + " Lỗi khi truy vấn lịch sử game.",
			})
			return
		}
		if entry == nil {
			_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: ui.Emojis.Empty + " Chưa có dữ liệu game cho người dùng này.",
			})
			return
		}

		targetDisplay := fmt.Sprintf("<@%s>", targetUserID)

		sb.WriteString(fmt.Sprintf("**Lịch sử %s của %s**\n\n", gameLabel, targetDisplay))
		profitXu := int64(math.Round(entry.Profit * 1000000))
		profitStr := formatCoins(profitXu)
		if profitXu > 0 {
			profitStr = "+" + profitStr
		}
		sb.WriteString(fmt.Sprintf("%s Lời/Lỗ: **%s %s**\n", ui.Emojis.User, profitStr, ui.Emojis.Coin))
		sb.WriteString(fmt.Sprintf("%s Lượt chơi: **%d**\n", ui.Emojis.Dice, entry.Plays))
	} else {
		entries, err := b.userRepo.GetGameLeaderboard(context.Background(), gameName)
		if err != nil {
			entries = []repository.LeaderboardEntry{}
		}

		sb.WriteString(fmt.Sprintf("**%s**\n\n", title))
		if len(entries) == 0 {
			sb.WriteString("_Chưa có dữ liệu lịch sử cho game này._\n")
		}

		for idx, e := range entries {
			profitXu := int64(math.Round(e.Profit * 1000000))
			profitStr := formatCoins(profitXu)
			if profitXu > 0 {
				profitStr = "+" + profitStr
			}

			// Bỏ fetch s.User để tối ưu API, dùng trực tiếp ping <@id> Discord tự render
			sb.WriteString(fmt.Sprintf("**%d**. <@%s>: **%s %s** | Round: **%d**\n", idx+1, e.UserID, profitStr, ui.Emojis.Coin, e.Plays))
		}
	}

	embed := view.GameEmbed(ui.Emojis.History+" Lịch Sử Game", sb.String())

	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
}

func (b *Bot) handleHostCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer immediately
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	userID := interactionUserID(i)
	gameName := ""
	if b.repostBlackjackTable(s, i.ChannelID, userID) {
		gameName = "Xì Dách"
	} else if b.repostTaixiuTable(s, i.ChannelID, userID) {
		gameName = "Tài Xỉu"
	} else if b.repostBaicaoTable(s, i.ChannelID, userID) {
		gameName = "Bài Cào"
	} else if b.repostBaucuaTable(s, i.ChannelID, userID) {
		gameName = "Bầu Cua"
	}

	if gameName != "" {
		followupEphemeral(s, i, "Đã kéo bàn "+gameName+" của bạn xuống đây!")
		return
	}
	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "Bạn không có bàn chơi nào (Tài Xỉu, Xì Dách, Bài Cào, Bầu Cua) đang hoạt động tại kênh này.",
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}
