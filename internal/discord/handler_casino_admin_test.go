package discord_test

import (
	"context"
	"testing"

	"github.com/discordcasino/v2/internal/discord"
	"github.com/discordcasino/v2/internal/repository"
)

func TestCasinoAdmin_BackendSecurityGuard(t *testing.T) {
	// 1. Dựng DB Test Memory (Mock tương đương Pattern của Project)
	db, err := repository.NewDatabase("mongodb://localhost:27017", "casino_admin_test_db")
	if err != nil {
		t.Skip("Bỏ qua Integration Test do không có MongoDB kết nối.")
	}
	defer db.Disconnect()

	userRepo := repository.NewUserRepository(db)
	configRepo := repository.NewConfigRepository(db)

	ctx := context.Background()
	// Xóa sạch rác
	db.Users().Drop(ctx)

	// Setup Data:
	ownerID := "9999999999999"
	hackerID := "1111111111111"

	_ = userRepo.SetRole(ctx, ownerID, "owner")

	t.Run("Kiểm tra quyền Backend thông qua GetEffectiveRole", func(t *testing.T) {
		// Giả sử Hacker bắt gói tin Discord và ấn gửi Button ID 'gcfg:taixiu:toggle'
		// (Giả lập hàm kiểm tra quyền y hệt ở đầu hàm handleCasinoAdminInteraction)

		hackerRole := discord.GetEffectiveRole(ctx, userRepo, configRepo, nil, "guild_1", hackerID, nil)
		if hackerRole >= discord.RoleOwner {
			t.Errorf("LỖI BẢO MẬT: Hacker có quyền truy cập Panel CC!")
		}

		ownerRole := discord.GetEffectiveRole(ctx, userRepo, configRepo, nil, "guild_1", ownerID, nil)
		if ownerRole < discord.RoleOwner {
			t.Errorf("Lỗi: Owner bị chặn khỏi Panel CC của chính mình!")
		}
	})

	t.Run("Bảng CC tự lưu Prefix qua DB", func(t *testing.T) {
		cfg, _ := configRepo.Get(ctx)
		cfg.Prefix = "$"
		configRepo.Save(ctx, cfg)

		// Kiểm tra việc Restart Bot (gọi lại Get) Config vẫn còn
		reloadedCfg, _ := configRepo.Get(ctx)
		if reloadedCfg.Prefix != "$" {
			t.Errorf("Đổi Prefix thất bại, DB không lưu trữ trạng thái.")
		}
	})
}
