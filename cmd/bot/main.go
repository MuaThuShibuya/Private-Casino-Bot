package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/discordcasino/v2/internal/discord"
	"github.com/discordcasino/v2/internal/logger"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/service"
)

const version = "2.0"

func main() {
	logger.Banner(version)

	// Khi dev local: tải từ file .env
	// Khi chạy trên Render: biến được set trực tiếp trong dashboard
	if err := godotenv.Load(); err != nil {
		logger.Warn("Config", "Không tìm thấy .env — dùng biến môi trường hệ thống")
	} else {
		logger.Info("Config", "Đã tải .env")
	}

	mongoURI := mustEnv("MONGODB_URI")
	dbName := getEnv("DB_NAME", "casino")
	botToken := mustEnv("DISCORD_TOKEN")
	guildID := getEnv("GUILD_ID", "")
	port := getEnv("PORT", "8080")

	logger.Info("Config", "DB=%s  GuildID=%q  Port=%s", dbName, guildID, port)

	// ── HTTP keep-alive ──────────────────────────────────────────────────────
	go startKeepAlive(port)

	// ── Kết nối database ─────────────────────────────────────────────────────
	logger.Info("DB", "Đang kết nối MongoDB Atlas...")
	db, err := repository.NewDatabase(mongoURI, dbName)
	if err != nil {
		logger.Fatal("DB", "Kết nối thất bại: %v", err)
	}
	defer db.Disconnect()
	logger.OK("DB", "Kết nối thành công — database: %s", dbName)

	// ── Repositories & Services ──────────────────────────────────────────────
	walletRepo := repository.NewWalletRepository(db)
	userRepo := repository.NewUserRepository(db)
	configRepo := repository.NewConfigRepository(db)
	gameConfigRepo := repository.NewGameConfigRepository(db)

	walletSvc := service.NewWalletService(walletRepo, userRepo)
	casinoConfigSvc := service.NewCasinoConfigService(gameConfigRepo)
	// Nạp cấu hình từ DB vào RAM ngay lúc khởi động
	_ = casinoConfigSvc.LoadAll(context.Background())
	logger.OK("DB", "Repositories sẵn sàng")

	// ── Seed owner ────────────────────────────────────────────────────────────
	if ownerIDs := getEnv("OWNER_ID", ""); ownerIDs != "" {
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer seedCancel()

		for _, ownerID := range strings.Split(ownerIDs, ",") {
			ownerID = strings.TrimSpace(ownerID)
			if ownerID == "" {
				continue
			}
			if err := seedOwner(seedCtx, userRepo, ownerID); err != nil {
				logger.Warn("Seed", "Không thể seed owner %s: %v", ownerID, err)
			}
		}
	}

	// ── Bot Discord ───────────────────────────────────────────────────────────
	logger.Info("Bot", "Đang khởi tạo bot Discord...")
	bot, err := discord.NewBot(botToken, walletSvc, userRepo, configRepo, casinoConfigSvc, guildID)
	if err != nil {
		logger.Fatal("Bot", "Khởi tạo thất bại: %v", err)
	}

	if err := bot.Start(); err != nil {
		logger.Fatal("Bot", "Khởi chạy thất bại: %v", err)
	}
	defer bot.Stop()

	logger.OK("Bot", "Bot đang chạy — nhấn Ctrl+C để dừng")
	fmt.Println()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop

	fmt.Println()
	logger.Warn("Main", "Nhận tín hiệu %s — đang tắt bot...", sig)
	logger.OK("Main", "Bot đã dừng sạch")
}

// startKeepAlive chạy HTTP server — Render dùng để health check, UptimeRobot ping để giữ alive.
func startKeepAlive(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		ts := time.Now().UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `","ts":"` + ts + `"}`))
		logger.Info("HTTP", "GET /health — 200 OK")
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	logger.Info("HTTP", "Keep-alive server lắng nghe trên :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("HTTP", "Server lỗi: %v", err)
	}
}

// seedOwner tạo hoặc cập nhật user thành role=owner khi khởi động.
func seedOwner(ctx context.Context, userRepo *repository.UserRepository, ownerID string) error {
	if _, err := userRepo.FindOrCreate(ctx, ownerID); err != nil {
		return err
	}
	if err := userRepo.SetRole(ctx, ownerID, models.RoleOwner); err != nil {
		return err
	}
	logger.OK("Seed", "Owner đã được set: %s", ownerID)
	return nil
}

// mustEnv đọc biến môi trường bắt buộc — dừng chương trình nếu thiếu.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		logger.Fatal("Config", "Biến môi trường bắt buộc %q chưa được set", key)
	}
	return v
}

// getEnv đọc biến môi trường, trả về fallback nếu không có.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Giữ import context để tránh lỗi compiler
var _ = context.Background
