package command

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

// CommandMetadata chứa thông tin chung của một lệnh.
// Đây là lớp metadata dùng cho cả slash command và prefix command.
type CommandMetadata struct {
	Name        string
	Description string
	Category    string
	Usage       string
	Example     string
	Cooldown    time.Duration
	Permissions []string
	Options     []*discordgo.ApplicationCommandOption
	IsSlash     bool
	IsPrefix    bool
}

// CommandContext chứa dữ liệu cần thiết cho handler xử lý interaction.
type CommandContext struct {
	Session     *discordgo.Session
	Interaction *discordgo.InteractionCreate
	Message     *discordgo.MessageCreate
}

// HandlerFunc là kiểu hàm xử lý command.
type HandlerFunc func(ctx CommandContext)

// MiddlewareFunc là hàm bọc handler để tạo pipeline.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// Command interface định nghĩa một lệnh.
type Command interface {
	Meta() CommandMetadata
	Handle(ctx CommandContext)
}

// NewHandler giúp tạo Command từ metadata và handler.
type NewHandler struct {
	Metadata CommandMetadata
	Handler  HandlerFunc
}

func (c NewHandler) Meta() CommandMetadata     { return c.Metadata }
func (c NewHandler) Handle(ctx CommandContext) { c.Handler(ctx) }
