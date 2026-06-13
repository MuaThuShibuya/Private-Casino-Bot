package view

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ui"
)

// NewEmbed tạo mẫu embed cơ bản với theme hiện tại.
func NewEmbed(title, description string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       ui.Theme.Colors.Neutral,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

// ErrorEmbed tạo embed lỗi nhanh.
func ErrorEmbed(message string) *discordgo.MessageEmbed {
	embed := NewEmbed(ui.Theme.Emojis.Error+" Lỗi", message)
	embed.Color = ui.Theme.Colors.Error
	return embed
}

// SuccessEmbed tạo embed thông báo thành công.
func SuccessEmbed(title, message string) *discordgo.MessageEmbed {
	embed := NewEmbed(title, message)
	embed.Color = ui.Theme.Colors.Success
	return embed
}

// WarningEmbed tạo embed cảnh báo.
func WarningEmbed(message string) *discordgo.MessageEmbed {
	embed := NewEmbed(ui.Theme.Emojis.Warning+" Cảnh báo", message)
	embed.Color = ui.Theme.Colors.Warning
	return embed
}

// GameEmbed tạo embed màu chủ đạo cho game và các nội dung chính.
func GameEmbed(title, message string) *discordgo.MessageEmbed {
	embed := NewEmbed(title, message)
	embed.Color = ui.Theme.Colors.Game
	return embed
}

// Field giúp tạo trường embed reusable.
func Field(name, value string, inline bool) *discordgo.MessageEmbedField {
	return &discordgo.MessageEmbedField{Name: name, Value: value, Inline: inline}
}
