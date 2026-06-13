package ui

// UIColor định nghĩa màu sắc chuẩn cho embed.
type UIColor struct {
	Neutral int
	Success int
	Warning int
	Error   int
	Game    int
}

// UITheme gộp các thành phần thiết kế chung của ứng dụng.
type UITheme struct {
	Emojis *UIEmoji
	Colors *UIColor
}

const (
	DefaultColorNeutral = 0x7EC8E5 // Màu xanh dương biển nhạt cho theme chính
	DefaultColorSuccess = 0x5AA9E6 // Màu xanh lá cây cho thành công
	DefaultColorWarning = 0xFAA61A // Màu cam cho cảnh báo
	DefaultColorError   = 0xED4245 // Màu đỏ cho lỗi
	DefaultColorGame    = 0x5AA9E6 // Màu xanh dương cho game
)

// Theme là theme hiện tại của bot.
// Bạn có thể đổi giá trị này tại runtime để thay toàn bộ biểu tượng / màu sắc.
var Theme = &UITheme{
	Emojis: Emojis,
	Colors: &UIColor{
		Neutral: DefaultColorNeutral,
		Success: DefaultColorSuccess,
		Warning: DefaultColorWarning,
		Error:   DefaultColorError,
		Game:    DefaultColorGame,
	},
}

// SetTheme thay đổi theme hiện tại để dễ chuyển đổi style.
func SetTheme(theme *UITheme) {
	if theme == nil {
		return
	}
	if theme.Emojis != nil {
		Theme.Emojis = theme.Emojis
	}
	if theme.Colors != nil {
		Theme.Colors = theme.Colors
	}
}
