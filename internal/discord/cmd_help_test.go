package discord

import (
	"strings"
	"testing"
)

func TestBuildHelpEmbed(t *testing.T) {
	t.Run("User thường gọi Help -> Bị ẩn lệnh Admin", func(t *testing.T) {
		embed := buildHelpEmbed(false, "!")
		for _, field := range embed.Fields {
			if strings.Contains(field.Name, "QUYỀN NĂNG QUẢN TRỊ") {
				t.Errorf("Lỗi: Người dùng thường nhìn thấy Menu Quản trị!")
			}
			if strings.Contains(field.Value, "!delete") || strings.Contains(field.Value, "/reset") {
				t.Errorf("Lỗi: Lệnh nguy hiểm bị lọt ra public help.")
			}
		}
	})

	t.Run("Admin/Owner gọi Help -> Nhìn thấy đầy đủ", func(t *testing.T) {
		embed := buildHelpEmbed(true, "!")
		fullText := ""
		for _, field := range embed.Fields {
			fullText += field.Name + field.Value
		}
		if !strings.Contains(fullText, "QUYỀN NĂNG QUẢN TRỊ") || !strings.Contains(fullText, "!delete") {
			t.Errorf("Lỗi: Không hiển thị lệnh Admin cho người có quyền.")
		}
	})
}
