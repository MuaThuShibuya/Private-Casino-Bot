// internal\discord\cmd_moderation.go
package discord

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ui"
)

// DeleteOptions lưu cấu hình lệnh xóa
type DeleteOptions struct {
	Amount     int
	IncludeOld bool
}

func validateDeleteAmount(input string) (int, error) {
	amount, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid")
	}
	if amount > 100 {
		return 0, fmt.Errorf("exceeds_limit")
	}
	return amount, nil
}

func ParseDeleteArgs(args []string) (DeleteOptions, error) {
	opts := DeleteOptions{}
	if len(args) == 0 {
		return opts, fmt.Errorf("missing_amount")
	}

	for _, arg := range args {
		lowerArg := strings.ToLower(strings.TrimSpace(arg))
		if lowerArg == "--old" || lowerArg == "old" {
			opts.IncludeOld = true
		} else if amt, err := validateDeleteAmount(lowerArg); err == nil {
			opts.Amount = amt
		} else if err.Error() == "exceeds_limit" {
			return opts, fmt.Errorf("exceeds_limit")
		} else {
			return opts, fmt.Errorf("invalid_arg")
		}
	}

	if opts.Amount == 0 {
		return opts, fmt.Errorf("missing_amount")
	}
	return opts, nil
}

type DeletePlan struct {
	BulkMessageIDs []string
	OldMessageIDs  []string
	SkippedPinned  int
	SkippedOld     int
}

func BuildDeletePlan(messages []*discordgo.Message, now time.Time, includeOld bool) DeletePlan {
	plan := DeletePlan{}
	// Mốc 14 ngày của Discord (trừ đi 1 phút an toàn)
	fourteenDaysAgo := now.Add(-14 * 24 * time.Hour).Add(1 * time.Minute)

	for _, msg := range messages {
		if msg.Pinned {
			plan.SkippedPinned++
			continue
		}

		if msg.Timestamp.Before(fourteenDaysAgo) || msg.Timestamp.Equal(fourteenDaysAgo) {
			if includeOld {
				plan.OldMessageIDs = append(plan.OldMessageIDs, msg.ID)
			} else {
				plan.SkippedOld++
			}
			continue
		}

		plan.BulkMessageIDs = append(plan.BulkMessageIDs, msg.ID)
	}
	return plan
}

func (b *Bot) cmdDelete(s *discordgo.Session, m *discordgo.MessageCreate, cmd string, args []string) {
	// Timeout cực dài (15 phút) để lệnh clear có đủ thời gian dọn dẹp hàng ngàn tin nhắn
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// 1. Kiểm tra config Moderation từ DB thông qua service có sẵn
	cfg := b.casinoConfigSvc.GetConfig("moderation")
	if !cfg.Enabled {
		return
	}

	// 2. Validate Quyền User
	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	hasPerm := role >= RoleAdmin
	if !hasPerm {
		userPerms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
		if err == nil && userPerms&discordgo.PermissionManageMessages == discordgo.PermissionManageMessages {
			hasPerm = true
		}
	}

	if !hasPerm {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Bạn không có quyền xóa tin nhắn (Cần quyền Admin/Owner hoặc Manage Messages).", 5*time.Second)
		return
	}

	// Xử lý lệnh dừng dọn dẹp
	lockKey := "purge_" + m.ChannelID
	if len(args) > 0 && strings.ToLower(args[0]) == "stop" {
		if _, ok := b.actionLocks.Load(lockKey); ok {
			b.actionLocks.Delete(lockKey)
			sendTemp(s, m.ChannelID, ui.Emojis.Success+" Đã gửi tín hiệu dừng tác vụ dọn dẹp tin nhắn.", 5*time.Second)
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có tác vụ dọn dẹp nào đang chạy ở kênh này.", 5*time.Second)
		}
		return
	}

	// 3. Validate Quyền Bot
	botPerms, err := s.State.UserChannelPermissions(s.State.User.ID, m.ChannelID)
	if err == nil && botPerms&discordgo.PermissionManageMessages == 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Bot thiếu quyền `Manage Messages` trong kênh này để thực hiện lệnh.", 5*time.Second)
		return
	}

	// Khóa kênh chống chạy nhiều lệnh Purge cùng lúc gây Rate Limit / Bug
	if !b.TryLockAction(lockKey, 15*time.Minute) {
		sendTemp(s, m.ChannelID, ui.Emojis.Loading+" Kênh này đang có tác vụ dọn dẹp tin nhắn. Vui lòng chờ hoàn tất.", 5*time.Second)
		return
	}
	// Dù lệnh thành công hay lỗi, cũng phải nhả khóa khi kết thúc
	defer func() {
		b.actionLocks.Delete(lockKey)
	}()

	// NẾU LÀ LỆNH CLEAR: XÓA TOÀN BỘ KÊNH (GIỮ LẠI TIN NHẮN GHIM)
	if cmd == "clear" {
		sendTemp(s, m.ChannelID, ui.Emojis.Loading+" Đang tiến hành quét và dọn dẹp toàn bộ tin nhắn trong kênh...", 5*time.Second)

		beforeID := m.ID
		totalDeleted := 0
		skippedPinned := 0
		isStopped := false

		for {
			if _, ok := b.actionLocks.Load(lockKey); !ok {
				isStopped = true
				break
			}

			// Liên tục lùi về quá khứ lấy 100 tin nhắn
			messages, err := s.ChannelMessages(m.ChannelID, 100, beforeID, "", "")
			if err != nil || len(messages) == 0 {
				break
			}
			beforeID = messages[len(messages)-1].ID // Trỏ mốc thời gian về tin nhắn cũ nhất trong lô

			plan := BuildDeletePlan(messages, time.Now(), true) // Xóa cả tin nhắn cũ

			if len(plan.BulkMessageIDs) > 0 {
				if len(plan.BulkMessageIDs) == 1 {
					s.ChannelMessageDelete(m.ChannelID, plan.BulkMessageIDs[0])
				} else {
					s.ChannelMessagesBulkDelete(m.ChannelID, plan.BulkMessageIDs)
				}
				totalDeleted += len(plan.BulkMessageIDs)
			}

			for _, id := range plan.OldMessageIDs {
				if _, ok := b.actionLocks.Load(lockKey); !ok {
					isStopped = true
					break
				}
				s.ChannelMessageDelete(m.ChannelID, id)
				totalDeleted++
				time.Sleep(500 * time.Millisecond) // Nghỉ nửa giây chống Rate Limit
			}
			skippedPinned += plan.SkippedPinned

			if isStopped {
				break
			}

			time.Sleep(1 * time.Second) // Giãn cách giữa các lần Call API
		}

		_ = s.ChannelMessageDelete(m.ChannelID, m.ID) // Xóa lệnh gốc
		msgStr := fmt.Sprintf("%s Đã dọn dẹp sạch sẽ **%d** tin nhắn trong kênh.", ui.Emojis.Success, totalDeleted)
		if isStopped {
			msgStr = fmt.Sprintf("%s Tác vụ dọn dẹp bị dừng. Đã xóa **%d** tin nhắn.", ui.Emojis.Warning, totalDeleted)
		}
		if skippedPinned > 0 {
			msgStr += fmt.Sprintf(" *(Đã tự động bỏ qua %d tin nhắn được Ghim)*", skippedPinned)
		}
		sendTemp(s, m.ChannelID, msgStr, 10*time.Second)
		log.Printf("[AUDIT LOG] Action: CLEAR_ALL | Guild: %s | Channel: %s | Executor: %s | Deleted: %d | Stopped: %v", m.GuildID, m.ChannelID, m.Author.ID, totalDeleted, isStopped)
		return
	}

	// 4. Validate Input
	opts, err := ParseDeleteArgs(args)
	if err != nil {
		prefix := b.GetPrefix(ctx)
		if err.Error() == "exceeds_limit" {
			sendTemp(s, m.ChannelID, ui.Emojis.Error+" Chỉ có thể xóa tối đa 100 tin nhắn mỗi lần.", 5*time.Second)
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+fmt.Sprintf(" Cách dùng: `%sdelete [số lượng] [--old]` (Tối đa 100).", prefix), 5*time.Second)
		}
		return
	}

	// Lấy tin nhắn (amount + 1 để bao gồm cả tin nhắn lệnh hiện tại cần xóa)
	messages, err := s.ChannelMessages(m.ChannelID, opts.Amount+1, "", "", "")
	if err != nil {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Lỗi khi lấy danh sách tin nhắn.", 5*time.Second)
		return
	}

	plan := BuildDeletePlan(messages, time.Now(), opts.IncludeOld)

	deletedCount := 0
	failedCount := 0
	isStopped := false

	// 5. Xóa Bulk siêu tốc cho các tin nhắn mới (< 14 ngày)
	if len(plan.BulkMessageIDs) > 0 {
		if len(plan.BulkMessageIDs) == 1 {
			err = s.ChannelMessageDelete(m.ChannelID, plan.BulkMessageIDs[0])
		} else {
			err = s.ChannelMessagesBulkDelete(m.ChannelID, plan.BulkMessageIDs)
		}

		if err != nil {
			log.Printf("[MODERATION] Bulk delete failed in channel %s: %v", m.ChannelID, err)
			failedCount += len(plan.BulkMessageIDs)
		} else {
			deletedCount += len(plan.BulkMessageIDs)
		}
	}

	// 6. Xóa thủ công chậm rãi cho các tin nhắn đã quá cũ (> 14 ngày)
	for _, id := range plan.OldMessageIDs {
		if _, ok := b.actionLocks.Load(lockKey); !ok {
			isStopped = true
			break
		}
		err := s.ChannelMessageDelete(m.ChannelID, id)
		if err != nil {
			failedCount++
		} else {
			deletedCount++
		}
		// Delay 500ms để chống Discord cấm API Rate Limit
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[AUDIT LOG] Action: DELETE_MESSAGES | Guild: %s | Channel: %s | Executor: %s | Requested: %d | IncludeOld: %v | Deleted: %d | OldDeleted: %d | SkippedOld: %d | SkippedPinned: %d | Failed: %d | Stopped: %v",
		m.GuildID, m.ChannelID, m.Author.ID, opts.Amount, opts.IncludeOld, deletedCount, len(plan.OldMessageIDs), plan.SkippedOld, plan.SkippedPinned, failedCount, isStopped)

	// Thông báo kết quả
	if deletedCount > 0 || plan.SkippedPinned > 0 || plan.SkippedOld > 0 {
		msgStr := fmt.Sprintf("%s Đã xóa **%d** tin nhắn.", ui.Emojis.Success, deletedCount)
		if isStopped {
			msgStr = fmt.Sprintf("%s Tác vụ dọn dẹp bị dừng. Đã xóa **%d** tin nhắn.", ui.Emojis.Warning, deletedCount)
		}
		var skips []string
		if plan.SkippedPinned > 0 {
			skips = append(skips, fmt.Sprintf("%d ghim", plan.SkippedPinned))
		}
		if plan.SkippedOld > 0 {
			skips = append(skips, fmt.Sprintf("%d quá cũ", plan.SkippedOld))
		}
		if failedCount > 0 {
			skips = append(skips, fmt.Sprintf("Lỗi: %d", failedCount))
		}
		if len(skips) > 0 {
			msgStr += fmt.Sprintf(" (Bỏ qua: %s)", strings.Join(skips, ", "))
		}
		sendTemp(s, m.ChannelID, msgStr, 5*time.Second)
	} else if failedCount > 0 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không thể xóa tin nhắn do lỗi API hoặc giới hạn của Discord.", 5*time.Second)
	}
}

// cmdStop là Global Kill-Switch chuyên nghiệp để dừng mọi auto game và purge
func (b *Bot) cmdStop(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stoppedAny := false
	var stoppedTasks []string

	// 1. Dừng dọn dẹp tin nhắn (Purge/Clear)
	lockKey := "purge_" + m.ChannelID
	if _, ok := b.actionLocks.Load(lockKey); ok {
		role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
		hasPerm := role >= RoleAdmin
		if !hasPerm {
			userPerms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
			if err == nil && userPerms&discordgo.PermissionManageMessages == discordgo.PermissionManageMessages {
				hasPerm = true
			}
		}
		if hasPerm {
			b.actionLocks.Delete(lockKey)
			stoppedTasks = append(stoppedTasks, "Dọn dẹp tin nhắn (Clear/Delete)")
			stoppedAny = true
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Block+" Bạn không có quyền dừng tác vụ dọn dẹp tin nhắn.", 5*time.Second)
			return
		}
	}

	// 2. Dừng Auto Game Series
	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	gamesMap := map[string]string{"taixiu": "Tài Xỉu", "baucua": "Bầu Cua", "xidach": "Xì Dách", "baicao": "Bài Cào"}

	for gameKey, gameName := range gamesMap {
		if series := GetAutoSeries(m.ChannelID, gameKey); series != nil {
			if series.OwnerID == m.Author.ID || role >= RoleAdmin {
				StopAutoSeries(m.ChannelID, gameKey)
				stoppedTasks = append(stoppedTasks, "Auto "+gameName)
				stoppedAny = true
			}
		}
	}

	if stoppedAny {
		msg := fmt.Sprintf("%s **Đã dừng thành công các tác vụ:**\n> %s", ui.Emojis.Success, strings.Join(stoppedTasks, "\n> "))
		s.ChannelMessageSend(m.ChannelID, msg)
	} else {
		sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có tác vụ tự động tạo bàn hoặc dọn dẹp nào đang chạy trong kênh này.", 5*time.Second)
	}
}
