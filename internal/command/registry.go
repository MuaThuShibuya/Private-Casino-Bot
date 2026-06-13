package command

import (
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// Registry quản lý danh sách lệnh và router chung.
type Registry struct {
	mu            sync.RWMutex
	commands      map[string]Command
	slashCommands []*discordgo.ApplicationCommand
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

// Register đăng ký một command mới.
func (r *Registry) Register(cmd Command) error {
	meta := cmd.Meta()
	if meta.Name == "" {
		return fmt.Errorf("command name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.commands[meta.Name]; ok {
		return fmt.Errorf("command %q already registered", meta.Name)
	}
	r.commands[meta.Name] = cmd

	if meta.IsSlash {
		r.slashCommands = append(r.slashCommands, &discordgo.ApplicationCommand{
			Name:        meta.Name,
			Description: meta.Description,
			Options:     meta.Options,
		})
	}

	return nil
}

// Commands returns all slash commands metadata for registration with Discord.
func (r *Registry) Commands() []*discordgo.ApplicationCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]*discordgo.ApplicationCommand(nil), r.slashCommands...)
}

// RouteSlashCommand tìm handler theo tên lệnh và chạy nó.
func (r *Registry) RouteSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	name := i.ApplicationCommandData().Name
	r.mu.RLock()
	cmd, ok := r.commands[name]
	r.mu.RUnlock()
	if !ok {
		return false
	}

	cmd.Handle(CommandContext{Session: s, Interaction: i})
	return true
}

// Chain middleware pipeline cho handler.
func Chain(handler HandlerFunc, middlewares ...MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
