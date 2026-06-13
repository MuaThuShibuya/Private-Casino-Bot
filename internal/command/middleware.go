package command

import (
	"log"
	"time"
)

// LoggingMiddleware ghi nhận request vào log trước và sau khi xử lý.
func LoggingMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx CommandContext) {
		log.Printf("[command] start: %s", commandName(ctx))
		start := time.Now()
		next(ctx)
		log.Printf("[command] done: %s duration=%s", commandName(ctx), time.Since(start))
	}
}

// RecoverMiddleware ngăn panic lan ra toàn bộ bot.
func RecoverMiddleware(next HandlerFunc) HandlerFunc {
	return func(ctx CommandContext) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[command] recovered panic: %v", r)
			}
		}()
		next(ctx)
	}
}

func commandName(ctx CommandContext) string {
	if ctx.Interaction != nil {
		return ctx.Interaction.ApplicationCommandData().Name
	}
	return "unknown"
}
