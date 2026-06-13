package discord

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ai/dialogue"
)

// ProcessAIMessage kiểm tra và xử lý nếu tin nhắn phù hợp để AI phản hồi
func (b *Bot) ProcessAIMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if b.ShouldAIReply(s, m) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if b.CanUseAI(ctx, m.ChannelID, m.GuildID, m.Author.ID) {
			// Anti-spam riêng cho AI (tránh ảnh hưởng lệnh command), dùng silent drop
			if b.rateLimiter.Allow(m.Author.ID + "_ai") {
				b.handleAIDialogue(s, m)
			}
		}
	}
}

// IsWhitelistedChannel kiểm tra xem kênh có nằm trong danh sách cho phép hay không
func (b *Bot) IsWhitelistedChannel(ctx context.Context, channelID string) bool {
	ok, err := b.configRepo.IsChannelWhitelisted(ctx, channelID)
	return err == nil && ok
}

// CanUseAI kiểm tra xem người dùng và kênh hiện tại có được phép tương tác với AI giả lập không
func (b *Bot) CanUseAI(ctx context.Context, channelID, guildID, userID string) bool {
	// 1. Bỏ qua DM hoặc Server không hợp lệ
	if guildID == "" {
		return false
	}

	// 2. Validate whitelist channel (Hệ thống whitelist đã có sẵn)
	if !b.IsWhitelistedChannel(ctx, channelID) {
		return false
	}

	// 3. (Optional) Mở rộng validate role hoặc check blacklist ở đây
	return true
}

// ShouldAIReply xác định xem bot có nên phản hồi tin nhắn này như một AI giả lập không
func (b *Bot) ShouldAIReply(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	// 1. Phản hồi nếu được reply trực tiếp
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		if m.ReferencedMessage.Author.ID == s.State.User.ID {
			return true
		}
	}

	// 2. Phản hồi nếu được mention
	for _, mention := range m.Mentions {
		if mention.ID == s.State.User.ID {
			return true
		}
	}

	// 3. Phản hồi nếu có từ khóa trigger
	lowerContent := strings.ToLower(m.Content)
	insults := []string{"bài l", "do code", "dev lỏ", "dm bot", "đm code", "bot ngu", "game rác", "code lỏ", "dev ngu", "bot l", "bot đần", "bot dở", "bot vcl", "bot cc", "bot chó", "bot cặc", "bot đéo", "bot khốn", "bot lol", "bot lồn", "đen", "xui", "đen đủi", "đen như chó", "đen như cc", "đen như bot"}
	for _, word := range insults {
		if strings.Contains(lowerContent, word) {
			return true
		}
	}

	return false
}

// handleAIDialogue xử lý logic chọn và trả về câu thoại giả lập AI
func (b *Bot) handleAIDialogue(s *discordgo.Session, m *discordgo.MessageCreate) {
	lowerContent := strings.ToLower(m.Content)

	// Kiểm tra trigger
	insults := []string{"bịp", "bài lỏ", "bài l", "do code", "dev lỏ", "dm bot", "đm code", "bot ngu", "game rác", "code lỏ", "dev ngu", "bot l", "bot đần", "bot dở", "bot vcl", "bot cc", "bot chó", "bot cặc", "bot đéo", "bot khốn", "bot lol", "bot lồn", "đen", "xui", "đen đủi", "đen như chó", "đen như cc", "đen như bot"}
	isInsult := false
	for _, word := range insults {
		if strings.Contains(lowerContent, word) {
			isInsult = true
			break
		}
	}

	dialogue.GetManager().MarkActivity(m.ChannelID)

	var reply string
	if isInsult {
		// Gọi preset trả lời khi bị chửi bới
		reply = dialogue.GetManager().MaybeSpeak(dialogue.PlayerInsultsCoder, map[string]string{
			"user":    m.Author.ID,
			"channel": m.ChannelID,
		})
	} else {
		// Silent ignore nếu gọi AI nhưng không đúng nội dung trigger,
		// Giúp tránh spam bot trả lời vô nghĩa hoặc làm phiền user.
		return
	}

	if reply != "" {
		s.ChannelMessageSend(m.ChannelID, reply)
	}
}
