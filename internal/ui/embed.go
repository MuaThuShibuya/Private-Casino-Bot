package ui

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

// NewEmbed tạo một embed chuẩn với theme hiện tại.
func NewEmbed(title, description string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       Theme.Colors.Neutral,
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

// SuccessEmbed tạo embed thành công với màu success.
func SuccessEmbed(title, message string) *discordgo.MessageEmbed {
	embed := NewEmbed(title, message)
	embed.Color = Theme.Colors.Success
	return embed
}

// ErrorEmbed tạo embed lỗi với màu lỗi.
func ErrorEmbed(message string) *discordgo.MessageEmbed {
	embed := NewEmbed(Theme.Emojis.Error+" Lỗi", message)
	embed.Color = Theme.Colors.Error
	return embed
}

// WarningEmbed tạo embed cảnh báo.
func WarningEmbed(message string) *discordgo.MessageEmbed {
	embed := NewEmbed(Theme.Emojis.Warning+" Cảnh báo", message)
	embed.Color = Theme.Colors.Warning
	return embed
}

// GameEmbed tạo embed dành cho giao diện game.
func GameEmbed(title, message string) *discordgo.MessageEmbed {
	embed := NewEmbed(title, message)
	embed.Color = Theme.Colors.Game
	return embed
}

// Field giúp tạo một trường embed reusable.
func Field(name, value string, inline bool) *discordgo.MessageEmbedField {
	return &discordgo.MessageEmbedField{Name: name, Value: value, Inline: inline}
}
