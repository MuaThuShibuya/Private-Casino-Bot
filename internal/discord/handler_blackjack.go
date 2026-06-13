package discord

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/ai/dialogue"
	"github.com/discordcasino/v2/internal/games"
	"github.com/discordcasino/v2/internal/models"
	"github.com/discordcasino/v2/internal/repository"
	"github.com/discordcasino/v2/internal/ui"
	"go.mongodb.org/mongo-driver/bson"
)

var bjTables sync.Map
var bjBatcher = NewGameUIBatcher()

type BJState struct {
	mu               sync.Mutex
	ControlMessageID string
}

var bjStates sync.Map

// checkBlackjackTableStatus kiểm tra và báo lỗi với các trạng thái khóa bàn
func checkBlackjackTableStatus(s *discordgo.Session, i *discordgo.InteractionCreate, status string, prefix string) bool {
	switch status {
	case "resolving":
		replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý thanh toán, vui lòng chờ.")
		return false
	case "settlement_failed":
		replyEphemeral(s, i, ui.Emojis.Error+" Giao dịch chưa chốt do lỗi hệ thống. Cược đang được giữ an toàn. Vui lòng chờ Admin xử lý.")
		return false
	case "canceling":
		replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý hủy bàn, vui lòng chờ.")
		return false
	case "cancel_refund_failed":
		replyEphemeral(s, i, ui.Emojis.Error+" Hủy bàn chưa hoàn tất do lỗi hoàn tiền. Vui lòng chờ Admin xử lý.")
		return false
	case "result_render_failed":
		replyEphemeral(s, i, ui.Emojis.Error+" Tiền đã xử lý xong nhưng Discord gửi bảng kết quả thất bại. Vui lòng dùng "+prefix+"host hoặc báo Admin để khôi phục kết quả.")
		return false
	case "cancel_ui_failed":
		replyEphemeral(s, i, ui.Emojis.Error+" Bàn đã hoàn tiền nhưng cập nhật giao diện thất bại. Vui lòng dùng "+prefix+"host hoặc báo Admin để khôi phục giao diện.")
		return false
	}
	return true
}

func (b *Bot) safeRefreshMessage(
	s *discordgo.Session,
	tableID string,
	channelID string,
	oldMsgID string,
	sendData *discordgo.MessageSend,
	reason string,
) (string, error) {
	if oldMsgID != "" {
		editData := &discordgo.MessageEdit{
			Channel: channelID,
			ID:      oldMsgID,
		}

		if sendData.Content != "" {
			editData.Content = &sendData.Content
		} else {
			empty := ""
			editData.Content = &empty
		}

		if len(sendData.Embeds) > 0 {
			editData.Embeds = &sendData.Embeds
		}

		if len(sendData.Components) > 0 {
			editData.Components = &sendData.Components
		}

		if _, err := s.ChannelMessageEditComplex(editData); err == nil {
			return oldMsgID, nil
		} else {
			log.Printf("[BJ_UI_EDIT_FAIL] table=%s msg=%s reason=%s err=%v", tableID, oldMsgID, reason, err)
		}
	}

	newMsg, err := s.ChannelMessageSendComplex(channelID, sendData)
	if err != nil {
		log.Printf("[BJ_UI_RECREATE_FAIL] table=%s oldMsg=%s reason=%s err=%v", tableID, oldMsgID, reason, err)
		return oldMsgID, err
	}

	if oldMsgID != "" && oldMsgID != newMsg.ID {
		_ = s.ChannelMessageDelete(channelID, oldMsgID)
	}

	return newMsg.ID, nil
}

func (b *Bot) cmdBlackjack(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, _ := b.configRepo.IsChannelWhitelisted(ctx, m.ChannelID)
	if !ok {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Lệnh casino không được phép ở kênh này.", 3*time.Second)
		return
	}

	cfg := b.casinoConfigSvc.GetConfig("xidach")
	if !cfg.Enabled {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Game Xì Dách hiện đang bị đóng để bảo trì/cấu hình.", 5*time.Second)
		return
	}

	if len(args) > 0 && strings.ToLower(args[0]) == "stop" {
		if series := GetAutoSeries(m.ChannelID, "xidach"); series != nil {
			if series.OwnerID == m.Author.ID || b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member) >= RoleAdmin {
				StopAutoSeries(m.ChannelID, "xidach")
				sendTemp(s, m.ChannelID, ui.Emojis.Success+" Đã dừng chuỗi tự động tạo bàn Xì Dách.", 5*time.Second)
			} else {
				sendTemp(s, m.ChannelID, ui.Emojis.Error+" Bạn không có quyền dừng chuỗi này.", 5*time.Second)
			}
		} else {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Không có chuỗi tự động nào đang chạy.", 5*time.Second)
		}
		return
	}

	var betAmount int64
	var durationSec int
	var count int = 1
	var err error

	parsedArgs := []string{}
	isTimeProvided := false
	if len(args) > 0 && strings.HasSuffix(strings.ToLower(args[0]), "s") {
		isTimeProvided = true
		timeStr := strings.TrimSuffix(strings.ToLower(args[0]), "s")
		durationSec, err = strconv.Atoi(timeStr)
		parsedArgs = args[1:]
	} else if len(args) == 2 && len(args[1]) > 2 {
		durationSec, err = strconv.Atoi(args[0])
		parsedArgs = args[1:]
	} else if len(args) == 3 {
		durationSec, err = strconv.Atoi(args[0])
		parsedArgs = args[1:]
	} else {
		durationSec = 20
		parsedArgs = args
	}

	if err == nil && len(parsedArgs) >= 1 {
		betAmount, err = strconv.ParseInt(parsedArgs[0], 10, 64)
	} else if !isTimeProvided {
		err = fmt.Errorf("invalid")
	}

	if err == nil && len(parsedArgs) == 2 {
		count, err = strconv.Atoi(parsedArgs[1])
	}

	if err != nil || betAmount <= 0 || durationSec < 5 || durationSec > 120 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Cú pháp không hợp lệ. Ví dụ: `"+b.GetPrefix(ctx)+"xd 45s 1000` hoặc `"+b.GetPrefix(ctx)+"xd 45s 1000 5`", 5*time.Second)
		return
	}
	if count < 1 {
		count = 1
	}
	if count > 1000 {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Số lần tự động tạo bàn tối đa là 1000.", 5*time.Second)
		return
	}

	if count > 1 {
		series := &AutoGameSeries{
			SeriesID: m.ChannelID + "_xidach", GameType: "xidach", ChannelID: m.ChannelID,
			OwnerID: m.Author.ID, GuildID: m.GuildID, TotalCount: count,
			Params: GameCreateParams{BetAmount: betAmount, DurationSec: durationSec},
		}
		if !StartAutoSeries(series) {
			sendTemp(s, m.ChannelID, ui.Emojis.Warning+" Kênh này đang có chuỗi tự động Xì Dách chạy rồi.", 5*time.Second)
			return
		}
		b.startBlackjackRound(s, m.ChannelID, m.Author.ID, m.GuildID, betAmount, durationSec, series)
	} else {
		b.startBlackjackRound(s, m.ChannelID, m.Author.ID, m.GuildID, betAmount, durationSec, nil)
	}
}

func (b *Bot) startBlackjackRound(s *discordgo.Session, channelID, hostID, guildID string, betAmount int64, durationSec int, series *AutoGameSeries) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hostBalance, err := b.wallet.GetBalance(ctx, hostID)
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi lấy số dư của bạn.", 5*time.Second)
		if series != nil {
			StopAutoSeries(channelID, "xidach")
		}
		return
	}

	// Giữ tiền của Host trước
	if err = b.SafeBetDeduct(ctx, s, guildID, hostID, betAmount); err != nil {
		msg := fmt.Sprintf("%s Bạn không đủ %s để tiếp tục làm Cái.", ui.Emojis.Take, coinDisplay(betAmount))
		if series != nil {
			msg += fmt.Sprintf("\n%s **Đã dừng chuỗi tự động tạo bàn (Đã tạo %d/%d bàn).**", ui.Emojis.Warning, series.CurrentIndex, series.TotalCount)
			StopAutoSeries(channelID, "xidach")
			s.ChannelMessageSend(channelID, msg)
		} else {
			sendTemp(s, channelID, msg, 5*time.Second)
		}
		return
	}

	t := games.NewBlackjackTable(channelID, guildID, hostID, betAmount, durationSec, hostBalance.Balance)
	bjTables.Store(t.TableID, t)

	statusStr := ui.Emojis.Loading + " Bàn đang mở... Đợi khách tham gia!"
	if series != nil {
		series.CurrentIndex++
		statusStr += fmt.Sprintf(" *(Tự động: %d/%d)*", series.CurrentIndex, series.TotalCount)
	}

	embed := ui.BuildBlackjackEmbed(t, statusStr)
	msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embed:      embed,
		Components: ui.BlackjackWaitingButtons(t.TableID, s),
	})
	if err != nil {
		sendTemp(s, channelID, ui.Emojis.Error+" Lỗi khi tạo bàn.", 5*time.Second)
		bjTables.Delete(t.TableID)
		b.wallet.BetSettle(ctx, hostID, betAmount, true) // Hoàn tiền Host
		if series != nil {
			StopAutoSeries(channelID, "xidach")
		}
	} else {
		t.MessageID = msg.ID
		// Auto-cancel (Timeout) nếu bàn nằm chờ quá 3 phút
		go func(table *games.BlackjackTable) {
			time.Sleep(3 * time.Minute)
			table.Mu.Lock()
			if table.Status == "waiting" {
				table.Status = "canceling"
				cancelKey := "xidach:cancel:" + table.TableID
				table.Mu.Unlock()

				errTx := b.wallet.ExecuteTx(context.Background(), func(sc context.Context) error {
					if err := b.userRepo.StartTx(sc, cancelKey); err != nil {
						return err
					}
					if err := b.wallet.BetSettle(sc, table.HostID, table.HostLockedAmount, true); err != nil {
						return err
					}
					for _, uid := range table.PlayerIDs {
						if uid != table.HostID {
							if err := b.wallet.BetSettle(sc, uid, table.BetAmount, true); err != nil {
								return err
							}
						}
					}
					return b.userRepo.CompleteTx(sc, cancelKey, bson.M{
						"reason": "timeout", "table_id": table.TableID,
					})
				})

				if errTx != nil {
					if errors.Is(errTx, repository.ErrRoundAlreadyResolved) {
						return
					} // Tránh double cancel
					table.Mu.Lock()
					table.Status = "cancel_refund_failed"
					table.Mu.Unlock()
					log.Printf("[BJ_AUTO_CANCEL_ERROR] Table %s refund failed: %v", table.TableID, errTx)
					s.ChannelMessageSend(table.ChannelID, ui.Emojis.Error+" Hủy bàn tự động chưa hoàn tất do lỗi hoàn tiền. Vui lòng chờ Admin xử lý.")
					return
				}

				embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Xì Dách tự động hủy do hết thời gian chờ. Tiền cọc đã hoàn lại.")}
				comps := []discordgo.MessageComponent{}
				_, errEdit := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
					ID: table.MessageID, Channel: table.ChannelID, Embeds: &embeds, Components: &comps,
				})

				if errEdit != nil {
					table.Mu.Lock()
					table.Status = "cancel_ui_failed"
					table.Mu.Unlock()
					log.Printf("[BJ_AUTO_CANCEL_UI_ERROR] Table %s edit failed: %v", table.TableID, errEdit)
					return
				}

				table.Mu.Lock()
				table.Status = "cancelled_no_players"
				table.Mu.Unlock()
				bjTables.Delete(table.TableID)
			} else {
				table.Mu.Unlock()
			}
		}(t)
	}
}

// checkAllPlayersDone kiểm tra xem tất cả người chơi đã dằn/quắc hết chưa
func checkAllPlayersDone(t *games.BlackjackTable) bool {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	if t.Status != "playing" {
		return false
	}
	for _, p := range t.Players {
		if !p.HasStood {
			return false
		}
	}
	return true
}

func (b *Bot) handleBlackjackButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	parts := strings.Split(i.MessageComponentData().CustomID, ":")
	if len(parts) < 3 {
		return
	}
	action := parts[1]
	tableID := parts[2]
	userID := interactionUserID(i)

	// ANTI-EXPLOIT: Nếu bàn đang xử lý giao dịch hoặc bị lỗi, chặn ngay lập tức toàn bộ thao tác
	if val, ok := bjTables.Load(tableID); ok {
		tStatus := val.(*games.BlackjackTable).Status
		if !checkBlackjackTableStatus(s, i, tStatus, b.GetPrefix(context.Background())) {
			return
		}
	}

	if action == "continue" {
		hostID := parts[2]
		if userID != hostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ chủ bàn mới có thể tạo ván mới.")
			return
		}
		betAmount, _ := strconv.ParseInt(parts[3], 10, 64)
		durationSec := 20
		if len(parts) > 4 {
			durationSec, _ = strconv.Atoi(parts[4])
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := b.SafeBetDeduct(ctx, s, i.GuildID, hostID, betAmount); err != nil {
			followupEphemeral(s, i, ui.Emojis.Take+" Số dư của bạn không đủ để tiếp tục làm Cái.")
			return
		}

		comps := []discordgo.MessageComponent{}
		s.ChannelMessageEditComplex(&discordgo.MessageEdit{ID: i.Message.ID, Channel: i.ChannelID, Components: &comps})

		StopAutoSeries(i.ChannelID, "xidach")
		hostBalance, err := b.wallet.GetBalance(ctx, hostID)
		if err != nil {
			b.wallet.BetSettle(ctx, hostID, betAmount, true)
			followupEphemeral(s, i, ui.Emojis.Error+" Lỗi khi lấy số dư của bạn. Đã hoàn tiền cọc.")
			return
		}
		if durationSec < 5 || durationSec > 120 {
			durationSec = 20 // Fallback to default if invalid duration from button
		}
		newT := games.NewBlackjackTable(i.ChannelID, i.GuildID, hostID, betAmount, durationSec, hostBalance.Balance)
		bjTables.Store(newT.TableID, newT)
		embed := ui.BuildBlackjackEmbed(newT, ui.Emojis.Loading+" Bàn đang mở... Đợi khách tham gia!")
		msg, err := s.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{Embed: embed, Components: ui.BlackjackWaitingButtons(newT.TableID, s)})
		if err != nil {
			bjTables.Delete(newT.TableID)
			b.wallet.BetSettle(ctx, hostID, betAmount, true)
		} else {
			newT.MessageID = msg.ID
		}
		return
	}

	val, exists := bjTables.Load(tableID)
	if !exists {
		replyEphemeral(s, i, ui.Emojis.Error+" Bàn đã kết thúc hoặc không tồn tại.")
		return
	}
	t := val.(*games.BlackjackTable)

	actionLockKey := "blackjack:action:" + t.TableID + ":" + userID

	switch action {
	case "join":
		lockKey := fmt.Sprintf("join:%s:%s", t.TableID, userID)
		if !b.TryLockAction(lockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Đang xử lý, vui lòng không bấm spam.")
			return
		}
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		t.Mu.Lock()
		if t.Status != "waiting" {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Loading+" Bàn đã bắt đầu, không thể tham gia nữa.")
			return
		}
		if _, ok := t.Players[userID]; ok {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Warning+" Bạn đã có mặt trong bàn.")
			return
		}
		if len(t.Players) >= 12 {
			t.Mu.Unlock()
			followupEphemeral(s, i, ui.Emojis.Block+" Bàn đã đầy (Tối đa 12 người).")
			return
		}
		t.Mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		playerBalance, err := b.wallet.GetBalance(ctx, userID)
		if err != nil {
			followupEphemeral(s, i, ui.Emojis.Error+" Lỗi khi lấy số dư của bạn để tham gia.")
			return
		}

		cfg := b.casinoConfigSvc.GetConfig("xidach")
		xdCfg := models.DefaultXidachConfig
		if cfg.XidachCfg != nil {
			xdCfg = *cfg.XidachCfg
		}
		maxMult := xdCfg.PayoutXiBan
		if xdCfg.PayoutBlackjack > maxMult {
			maxMult = xdCfg.PayoutBlackjack
		}
		if xdCfg.PayoutNguLinh > maxMult {
			maxMult = xdCfg.PayoutNguLinh
		}
		hostEscrow := int64(math.Round(float64(t.BetAmount) * maxMult))

		// Trừ tiền người chơi
		if err := b.SafeBetDeduct(ctx, s, i.GuildID, userID, t.BetAmount); err != nil {
			followupEphemeral(s, i, ui.Emojis.Take+" Bạn không đủ số dư / hạn mức tín dụng để tham gia.")
			return
		}
		// ESCROW: Trừ tiền Host tương ứng với số tiền người chơi vừa cược để đảm bảo năng lực trả thưởng
		if err := b.SafeBetDeduct(ctx, s, i.GuildID, t.HostID, hostEscrow); err != nil {
			b.wallet.BetSettle(ctx, userID, t.BetAmount, true) // Hoàn lại cho khách
			followupEphemeral(s, i, ui.Emojis.Take+" Tham gia thất bại: Nhà cái (Host) đã hết số dư dự phòng để thế chân cho khách mới!")
			return
		}

		t.Mu.Lock()
		_, alreadyJoined := t.Players[userID]
		// Kiểm tra lại trạng thái để chống lỗi người chơi ấn Tham Gia cùng lúc Host ấn Bắt Đầu
		if t.Status != "waiting" || alreadyJoined || len(t.Players) >= 12 {
			t.Mu.Unlock()

			errTx := b.wallet.ExecuteTx(ctx, func(sc context.Context) error {
				if err := b.wallet.BetSettle(sc, userID, t.BetAmount, true); err != nil {
					return err
				}
				if err := b.wallet.BetSettle(sc, t.HostID, hostEscrow, true); err != nil {
					return err
				}
				return nil
			})

			if errTx != nil {
				followupEphemeral(s, i, ui.Emojis.Error+" Hoàn tiền thất bại do lỗi hệ thống, vui lòng báo Admin kiểm tra ID: "+userID)
			} else {
				followupEphemeral(s, i, ui.Emojis.Warning+" Bàn đã đổi trạng thái hoặc đã đầy trong lúc bạn tham gia, hệ thống đã hoàn lại tiền!")
			}
			return
		}
		t.HostLockedAmount += hostEscrow // Ghi nhận cọc của Host an toàn
		t.AddPlayer(userID, playerBalance.Balance)
		embed := ui.BuildBlackjackEmbed(t, ui.Emojis.Loading+" Đang chờ người chơi...")
		oldMainID := t.MessageID
		tableID := t.TableID
		channelID := t.ChannelID
		t.Mu.Unlock()

		newMainID, err := b.safeRefreshMessage(s, tableID, channelID, oldMainID, &discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: ui.BlackjackWaitingButtons(tableID, s),
		}, "join_refresh")

		if err == nil && newMainID != oldMainID {
			t.Mu.Lock()
			t.MessageID = newMainID
			t.Mu.Unlock()
		}

	case "start":
		if userID != t.HostID {
			replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Nhà cái mới có thể Bắt Đầu ván đấu.")
			return
		}

		// Kiểm tra quyền Admin/Owner trước khi Lock để tránh Deadlock gây lag bot
		ctxCheck, cancelCheck := context.WithTimeout(context.Background(), 2*time.Second)
		isPrivileged := b.GetRole(ctxCheck, s, i.GuildID, t.HostID, i.Member) >= RoleAdmin
		cancelCheck()

		t.Mu.Lock()
		if t.Status != "waiting" {
			t.Mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Bàn đã bắt đầu hoặc đã kết thúc.")
			return
		}
		if len(t.Players) < 2 {
			t.Mu.Unlock()
			replyEphemeral(s, i, ui.Emojis.Warning+" Phải có ít nhất 1 người chơi khác tham gia để bắt đầu!")
			return
		}
		t.StartGame()
		log.Printf("[BJ_TIMER_CREATE] table=%s duration=%d started=%s deadline=%s",
			t.TableID,
			t.DurationSec,
			t.StartedAt.Format(time.RFC3339Nano),
			t.Deadline.Format(time.RFC3339Nano),
		)

		// --- ĐỒNG BỘ CƠ CHẾ RIG TỪ LỆNH !cc ---
		cfg := b.casinoConfigSvc.GetConfig("xidach")
		rigProb := getRigProb(cfg.RigMode)

		if isPrivileged && rigProb > 0 && games.RandInt(100) < int64(rigProb) {
			bestUid := t.HostID
			bestScore := -1
			for uid, p := range t.Players {
				val := games.HandValue(p.Hand)
				if games.IsXiBan(p.Hand) {
					val = 100
				} else if games.IsXiDach(p.Hand) {
					val = 90
				}
				if val > bestScore {
					bestScore = val
					bestUid = uid
				}
			}
			if bestUid != t.HostID {
				t.Players[t.HostID].Hand, t.Players[bestUid].Hand = t.Players[bestUid].Hand, t.Players[t.HostID].Hand
			}
		}
		// ---------------------------------------------------------

		// Kiểm tra lật bàn ngay nếu Host có Xì Dách / Xì Bàn
		hostHand := t.Players[t.HostID].Hand
		if games.IsXiBan(hostHand) || games.IsXiDach(hostHand) {
			t.Mu.Unlock()

			// Cảnh báo ngay trong chat cho kịch tính
			s.ChannelMessageSend(t.ChannelID, ui.Emojis.Warning+" **Nhà cái lật bài! Có Xì Dách / Xì Bàn!**")

			// PHẢI ACK INTERACTION ĐỂ TRÁNH LỖI "FAILED" VÀ MẤT NÚT TẠO BÀN
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
			b.resolveBlackjack(s, t, "host_special")
			return
		}

		embedDealing := ui.BuildBlackjackEmbed(t, ui.Emojis.CardGame+" Đang chia bài...")

		// Khởi tạo state hỗ trợ
		bjStates.Store(t.TableID, &BJState{})
		t.Mu.Unlock()

		updateEmbed(s, i, embedDealing, []discordgo.MessageComponent{})

		// Tạo hiệu ứng chờ ngắn trước khi cho phép rút bài (giảm lag)
		go func() {
			time.Sleep(500 * time.Millisecond)
			t.Mu.Lock()
			if t.Status != "playing" {
				t.Mu.Unlock()
				return
			}
			embedPlaying := ui.BuildBlackjackEmbed(t, ui.Emojis.Play+" **Ván đấu đang diễn ra!**")
			tableID := t.TableID
			channelID := t.ChannelID
			messageID := t.MessageID
			t.Mu.Unlock()

			s.ChannelMessageEditEmbed(channelID, messageID, embedPlaying)
			// Gửi bảng điều khiển riêng
			ctrlMsg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content:    "\u200B",
				Components: ui.BlackjackPlayingButtons(tableID, s),
			})

			if err == nil {
				if val, ok := bjStates.Load(tableID); ok {
					state := val.(*BJState)
					state.mu.Lock()
					state.ControlMessageID = ctrlMsg.ID
					state.mu.Unlock()
				}
			} else {
				log.Printf("[BJ_START_CTRL_FAIL] table=%s err=%v", tableID, err)
				s.ChannelMessageSend(channelID, ui.Emojis.Warning+" Không thể tạo bảng điều khiển do lỗi Discord. Vui lòng gõ `"+b.GetPrefix(context.Background())+"host` để kéo lại bàn.")
			}

			b.runBlackjackCountdown(s, t)
		}()

	case "hit":
		if !b.TryLockAction(actionLockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Thao tác rút bài đang xử lý, vui lòng không bấm spam.")
			return
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		if val, ok := bjStates.Load(t.TableID); ok {
			state := val.(*BJState)
			state.mu.Lock()
			ctrlID := state.ControlMessageID
			state.mu.Unlock()
			if i.Message.ID != ctrlID {
				followupEphemeral(s, i, ui.Emojis.Warning+" Bảng điều khiển này đã cũ. Vui lòng kéo xuống bảng mới nhất.")
				return
			}
		}

		// THỰC THI ATOMIC BÊN TRONG GAME LOGIC
		c, errMsg, ok := t.Hit(userID)
		if !ok {
			followupEphemeral(s, i, ui.Emojis.WhiteEnd+" "+errMsg)
			return
		}

		t.Mu.Lock()
		p, _ := t.Players[userID]
		t.Mu.Unlock()

		// Thay thế việc gửi tin nhắn bằng cách đưa vào Batcher để gom nhóm
		eventMsg := fmt.Sprintf("%s <@%s> vừa rút thêm bài", ui.Emojis.ArrowWhite, userID)
		bjBatcher.QueueEvent(t.TableID, GameUIEvent{UserID: userID, Message: eventMsg}, func(events []GameUIEvent) {
			b.flushBlackjackUI(s, t, events)
		})

		msgContent := fmt.Sprintf("%s **Bạn vừa rút được lá:** %s\n**Bài hiện tại của bạn:**\n# %s\n> Tổng điểm: **%d**", ui.Emojis.CardGame, ui.CardString(c), ui.HandString(p.Hand), games.HandValue(p.Hand))
		if p.Status == "bust" {
			msgContent += fmt.Sprintf("\n%s **Bạn đã QUẮC!**", ui.Emojis.WhiteEnd)
		} else if p.Status == "stand" && len(p.Hand) == 5 {
			msgContent += fmt.Sprintf("\n%s **Bạn đã đủ 5 lá (Ngũ Linh)!** Tự động dằn.", ui.Emojis.Success)
		} else if games.HandValue(p.Hand) == 21 {
			msgContent += fmt.Sprintf("\n%s *Bạn đang 21 điểm, vẫn có thể rút để săn Ngũ Linh nhưng cẩn thận rủi ro Quắc/Già.*", ui.Emojis.Warning)
		}

		followupEphemeral(s, i, msgContent)

		if checkAllPlayersDone(t) {
			go b.resolveBlackjack(s, t, "all_done")
		}

	case "stand":
		if !b.TryLockAction(actionLockKey, InteractionLockTTL) {
			replyEphemeral(s, i, ui.Emojis.Loading+" Thao tác đang được xử lý...")
			return
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		if val, ok := bjStates.Load(t.TableID); ok {
			state := val.(*BJState)
			state.mu.Lock()
			ctrlID := state.ControlMessageID
			state.mu.Unlock()
			if i.Message.ID != ctrlID {
				followupEphemeral(s, i, ui.Emojis.Warning+" Bảng điều khiển này đã cũ. Vui lòng kéo xuống bảng mới nhất.")
				return
			}
		}

		cfg := b.casinoConfigSvc.GetConfig("xidach")
		xdCfg := models.DefaultXidachConfig
		if cfg.XidachCfg != nil {
			xdCfg = *cfg.XidachCfg
		}

		minStand := 16 // Người chơi mặc định dằn từ 16
		if userID == t.HostID {
			minStand = xdCfg.DealerStandAt // Chủ bàn dằn theo Config !cc
		}

		errMsg, ok := t.Stand(userID, minStand)
		if !ok {
			followupEphemeral(s, i, ui.Emojis.Warning+" "+errMsg)
			return
		}

		t.Mu.Lock()
		p, _ := t.Players[userID]
		t.Mu.Unlock()

		// Thay thế việc gửi tin nhắn bằng cách đưa vào Batcher để gom nhóm
		eventMsg := fmt.Sprintf("%s <@%s> đã chọn dằn bài", ui.Emojis.ArrowWhite, userID)
		bjBatcher.QueueEvent(t.TableID, GameUIEvent{UserID: userID, Message: eventMsg}, func(events []GameUIEvent) {
			b.flushBlackjackUI(s, t, events)
		})

		followupEphemeral(s, i, fmt.Sprintf("%s **Bạn đã chọn Dừng. Bài cuối cùng của bạn:**\n# %s\n> Tổng điểm: **%d**", ui.Emojis.ArrowWhite, ui.HandString(p.Hand), games.HandValue(p.Hand)))

		if checkAllPlayersDone(t) {
			go b.resolveBlackjack(s, t, "all_done")
		}

	case "view":
		t.Mu.Lock()
		p, ok := t.Players[userID]
		t.Mu.Unlock()
		if !ok {
			replyEphemeral(s, i, ui.Emojis.Warning+" Bạn không tham gia bàn này.")
			return
		}
		if len(p.Hand) == 0 {
			replyEphemeral(s, i, ui.Emojis.Loading+" Bài chưa được chia.")
			return
		}
		replyEphemeral(s, i, fmt.Sprintf("%s **Bài của bạn:**\n# %s\n> Tổng điểm: **%d**", ui.Emojis.CardGame, ui.HandString(p.Hand), games.HandValue(p.Hand)))

	case "cancel":
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if userID != t.HostID {
			if !HasAdminAccess(b, ctx, s, i) {
				replyEphemeral(s, i, ui.Emojis.Block+" Chỉ Nhà cái (Host) hoặc Admin mới có thể hủy bàn.")
				return
			}
		}

		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})

		t.Mu.Lock()
		if t.Status == "finished" || !checkBlackjackTableStatus(s, i, t.Status, b.GetPrefix(ctx)) {
			t.Mu.Unlock()
			return
		}
		t.Status = "canceling"
		t.Mu.Unlock()

		bjBatcher.ForceFlushAndClose(t.TableID)
		StopAutoSeries(t.ChannelID, "xidach")
		cancelKey := "xidach:cancel:" + t.TableID

		errTx := b.wallet.ExecuteTx(ctx, func(sc context.Context) error {
			if err := b.userRepo.StartTx(sc, cancelKey); err != nil {
				return err
			}
			if err := b.wallet.BetSettle(sc, t.HostID, t.HostLockedAmount, true); err != nil {
				return err
			}
			for _, uid := range t.PlayerIDs {
				if uid != t.HostID {
					if err := b.wallet.BetSettle(sc, uid, t.BetAmount, true); err != nil {
						return err
					}
				}
			}
			return b.userRepo.CompleteTx(sc, cancelKey, bson.M{
				"reason": "manual_cancel", "table_id": t.TableID,
			})
		})

		if errTx != nil {
			if errors.Is(errTx, repository.ErrRoundAlreadyResolved) {
				return
			}
			t.Mu.Lock()
			t.Status = "cancel_refund_failed"
			t.Mu.Unlock()
			log.Printf("[BJ_CANCEL_ERROR] Table %s refund failed: %v", t.TableID, errTx)
			// Thông báo đã được handle ở hàm Check, chỉ ghi log ở đây
			return
		}

		embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Xì Dách đã bị hủy bởi <@" + userID + ">. Tiền cược đã được hoàn lại.")}
		comps := []discordgo.MessageComponent{}
		_, errEdit := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			ID: i.Message.ID, Channel: i.ChannelID, Embeds: &embeds, Components: &comps,
		})

		if errEdit != nil {
			t.Mu.Lock()
			t.Status = "cancel_ui_failed"
			t.Mu.Unlock()
			log.Printf("[BJ_CANCEL_UI_ERROR] Table %s edit failed: %v", t.TableID, errEdit)
			return
		}

		t.Mu.Lock()
		t.Status = "finished"
		t.Mu.Unlock()

		b.cleanupBlackjackMessagesExcept(s, t, i.Message.ID)
		bjTables.Delete(t.TableID)
		bjStates.Delete(t.TableID)
		deleteAfter(s, i.ChannelID, i.Message.ID, 3*time.Second)
	}
}

// flushBlackjackUI thực thi Render UI từ Authoritative State sau khi gom sự kiện
func (b *Bot) flushBlackjackUI(s *discordgo.Session, t *games.BlackjackTable, events []GameUIEvent) {
	t.Mu.Lock()
	if t.Status != "playing" {
		t.Mu.Unlock()
		return
	}

	var recentEvents []string
	for _, ev := range events {
		recentEvents = append(recentEvents, ev.Message)
	}

	embed := ui.BuildBlackjackEmbed(t, ui.Emojis.LoadingWhite+" **Ván đấu đang diễn ra!**")
	oldMainID := t.MessageID
	channelID := t.ChannelID
	t.Mu.Unlock()

	// 1. Cập nhật Main Board để in đậm số lá bài mới
	if oldMainID != "" {
		_, err := s.ChannelMessageEditEmbed(channelID, oldMainID, embed)
		if err != nil {
			log.Printf("[BJ_FLUSH_UI_ERROR] table=%s err=%v", t.TableID, err)
		}
	}

	// 2. Gửi tin nhắn mới tổng hợp các thao tác (tránh bị nuốt thông báo do giới hạn edit)
	if len(recentEvents) > 0 {
		msgContent := strings.Join(recentEvents, "\n")
		_, err := s.ChannelMessageSend(channelID, msgContent)
		if err != nil {
			log.Printf("[BJ_FLUSH_MSG_ERROR] table=%s err=%v", t.TableID, err)
		}
	}
}

func (b *Bot) runBlackjackCountdown(s *discordgo.Session, t *games.BlackjackTable) {
	// Capture context for stale check
	tableID := t.TableID
	deadline := t.Deadline

	// Main timer for resolution
	time.AfterFunc(time.Until(deadline), func() {
		log.Printf("[BJ_TIMER_FIRE] table=%s", tableID)
		val, ok := bjTables.Load(tableID)
		if !ok {
			log.Printf("[BJ_TIMER_STALE] table=%s not found in map, probably already resolved.", tableID)
			return
		}
		currentTable := val.(*games.BlackjackTable)

		// Stale check: ensure this timer belongs to the current table instance
		if !currentTable.Deadline.Equal(deadline) {
			log.Printf("[BJ_TIMER_STALE_DEADLINE] table=%s oldDeadline=%s currentDeadline=%s",
				tableID, deadline.Format(time.RFC3339Nano), currentTable.Deadline.Format(time.RFC3339Nano))
			return
		}

		b.resolveBlackjack(s, currentTable, "deadline")
	})

	// Hẹn giờ nhắc nhở 10 giây cuối cùng
	reminderTime := time.Until(deadline) - 10*time.Second
	if reminderTime > 0 {
		time.AfterFunc(reminderTime, func() {
			val, ok := bjTables.Load(tableID)
			if !ok {
				return
			}
			currentTable := val.(*games.BlackjackTable)
			if !currentTable.Deadline.Equal(deadline) {
				return
			}

			currentTable.Mu.Lock()
			status := currentTable.Status
			channelID := currentTable.ChannelID
			currentTable.Mu.Unlock()

			if status == "playing" {
				embed := &discordgo.MessageEmbed{
					Description: fmt.Sprintf("# %s **CÒN 10 GIÂY!**\n> **Hãy nhanh chóng Rút hoặc Dằn bài!**", ui.Emojis.WhiteEnd),
					Color:       0x6B295C,
				}

				var oldCtrlID string
				if val, ok := bjStates.Load(tableID); ok {
					state := val.(*BJState)
					state.mu.Lock()
					oldCtrlID = state.ControlMessageID
					state.mu.Unlock()
				}

				msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
					Embed:      embed,
					Components: ui.BlackjackPlayingButtons(tableID, s),
				})
				if err == nil && msg != nil {
					if oldCtrlID != "" {
						s.ChannelMessageDelete(channelID, oldCtrlID)
					}
					if val, ok := bjStates.Load(tableID); ok {
						state := val.(*BJState)
						state.mu.Lock()
						state.ControlMessageID = msg.ID
						state.mu.Unlock()
					}
					deleteAfter(s, channelID, msg.ID, 10*time.Second)
				}
			}
		})
	}

	// Cập nhật UI mỗi 10s để hạn chế rate-limit nhưng không bị mất nút quá lâu
	uiTicker := time.NewTicker(10 * time.Second)
	defer uiTicker.Stop()

	for range uiTicker.C {
		t.Mu.Lock()
		if t.Status != "playing" {
			t.Mu.Unlock()
			return
		}
		oldMainID := t.MessageID
		tableStatus := t.Status
		t.Mu.Unlock()

		if tableStatus == "resolving" || tableStatus == "settlement_failed" || tableStatus == "canceling" || tableStatus == "cancel_refund_failed" || tableStatus == "finished" {
			return
		}

		// Edit bảng chính (Không xóa để tránh trôi chat quá dài)
		embed := ui.BuildBlackjackEmbed(t, ui.Emojis.LoadingWhite+" **Ván đấu đang diễn ra!**")
		if oldMainID != "" {
			_, err := s.ChannelMessageEditEmbed(t.ChannelID, oldMainID, embed)
			if err != nil {
				log.Printf("[BJ_UI_REFRESH_FAIL] table=%s err=%v", t.TableID, err)
			}
		}

		// Chỉ làm mới nút nếu còn nhiều hơn 10 giây (tránh đè thông báo 10 giây cuối cùng có chứa nút)
		if time.Until(t.Deadline) > 10*time.Second {
			if val, ok := bjStates.Load(t.TableID); ok {
				state := val.(*BJState)
				state.mu.Lock()
				oldCtrlID := state.ControlMessageID
				state.mu.Unlock()

				// Gửi bảng nút bấm mới trước, thành công mới xóa cái cũ
				newCtrlMsg, err := s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
					Content:    "\u200B",
					Components: ui.BlackjackPlayingButtons(t.TableID, s),
				})
				if err == nil {
					if oldCtrlID != "" {
						s.ChannelMessageDelete(t.ChannelID, oldCtrlID)
					}
					state.mu.Lock()
					state.ControlMessageID = newCtrlMsg.ID
					state.mu.Unlock()
				}
			}
		}
	}
}

func (b *Bot) resolveBlackjack(s *discordgo.Session, t *games.BlackjackTable, reason string) {
	// Prevent resolving before deadline unless it's a special case (like host blackjack)
	// Cho phép chênh lệch 1 giây do sai số timer của hệ điều hành. Nếu sớm quá 1s thì hẹn giờ lại thay vì return chết ván.
	if reason == "deadline" && time.Until(t.Deadline) > 1*time.Second {
		log.Printf("[BJ_RESOLVE_BLOCKED_EARLY] table=%s reason=%s remaining=%s. Rescheduling.",
			t.TableID, reason, time.Until(t.Deadline))
		time.AfterFunc(time.Until(t.Deadline), func() {
			b.resolveBlackjack(s, t, "deadline")
		})
		return
	}

	// Khóa buffer batcher để tránh update lọt sau khi ván kết thúc
	bjBatcher.ForceFlushAndClose(t.TableID)

	t.Mu.Lock()
	if t.Status == "finished" || t.Status == "cancelled_no_players" || t.Status == "resolving" {
		t.Mu.Unlock()
		return
	}
	log.Printf("[BJ_STATUS] table=%s %s -> resolving reason=%s", t.TableID, t.Status, reason)
	t.Status = "resolving"
	t.Mu.Unlock()

	log.Printf("[BJ_RESOLVE_START] table=%s reason=%s", t.TableID, reason)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := b.casinoConfigSvc.GetConfig("xidach")
	xdCfg := models.DefaultXidachConfig
	if cfg.XidachCfg != nil {
		xdCfg = *cfg.XidachCfg
	}

	hands := make(map[string][]games.Card)
	for _, uid := range t.PlayerIDs {
		hands[uid] = t.Players[uid].Hand
	}

	deltas, rawResults := games.CalculateBlackjackSettlement(t.HostID, t.PlayerIDs, hands, t.BetAmount, xdCfg.PayoutXiBan, xdCfg.PayoutBlackjack, xdCfg.PayoutNguLinh)
	playerResults := make(map[string]ui.BJPlayerResult)

	for _, uid := range t.PlayerIDs {
		if uid == t.HostID {
			continue
		}
		playerResults[uid] = ui.BJPlayerResult{
			Status: rawResults[uid].Status,
			Net:    rawResults[uid].Net,
		}
	}

	// LẤY TRƯỚC HẠN MỨC TÍN DỤNG ĐỂ SỬ DỤNG TRONG TRANSACTION (TRÁNH GỌI DISCORD API TRONG DB LOCK)
	debtLimits := make(map[string]int64)
	for _, uid := range t.PlayerIDs {
		debtLimits[uid] = b.GetEffectiveDebtLimit(ctx, s, t.GuildID, uid)
	}

	hostDelta := deltas[t.HostID]
	actions := games.GetSettlementActions(t.HostID, t.PlayerIDs, deltas, t.BetAmount, t.HostLockedAmount)
	roundID := "xidach:settlement:" + t.TableID

	var hostNewBalance int64
	errTx := b.wallet.ExecuteTx(ctx, func(sc context.Context) error {
		if err := b.userRepo.StartTx(sc, roundID); err != nil {
			return err
		}
		for _, act := range actions {
			if act.Refund > 0 {
				if err := b.wallet.BetSettle(sc, act.UserID, act.Refund, true); err != nil {
					return err
				}
			}
			if act.Shortfall > 0 {
				// SỬ DỤNG SettleShortfallTx THAY VÌ AdminTake (HỖ TRỢ TÍN DỤNG)
				if err := b.wallet.SettleShortfallTx(sc, act.UserID, act.Shortfall, debtLimits[act.UserID]); err != nil {
					return fmt.Errorf("vượt hạn mức tín dụng khi thanh toán Shortfall (%d): %w", act.Shortfall, err)
				}
			}
			if err := b.userRepo.UpdateGameHistory(sc, act.UserID, "xidach", float64(act.Delta)/1000000.0); err != nil {
				return err
			}
		}

		// LẤY SỐ DƯ SNAPSHOT TRỰC TIẾP TỪ TRONG TRANSACTION DB ĐỂ LƯU VÀO LỊCH SỬ UI
		var storedResults []bson.M
		for uid, res := range playerResults {
			bal, errBal := b.wallet.GetBalance(sc, uid)
			if errBal != nil {
				return errBal // Bắt lỗi GetBalance và Rollback Transaction
			}
			finalBal := int64(0)
			if bal != nil {
				finalBal = bal.Balance
			}
			res.Balance = finalBal
			playerResults[uid] = res // Cập nhật ngược lại map RAM

			storedResults = append(storedResults, bson.M{
				"user_id": uid, "status": res.Status, "net": res.Net, "balance": finalBal,
			})
		}

		hostBal, errHostBal := b.wallet.GetBalance(sc, t.HostID)
		if errHostBal != nil {
			return errHostBal // Bắt lỗi GetBalance và Rollback Transaction
		}
		if hostBal != nil {
			hostNewBalance = hostBal.Balance
		}

		playerHandsDoc := make(map[string][]games.Card)
		for uid, p := range t.Players {
			playerHandsDoc[uid] = p.Hand
		}

		details := bson.M{
			"table_id":           t.TableID,
			"channel_id":         t.ChannelID,
			"guild_id":           t.GuildID,
			"duration_sec":       t.DurationSec,
			"actions":            actions,
			"host_net":           hostDelta,
			"host_id":            t.HostID,
			"player_ids":         t.PlayerIDs,
			"bet_amount":         t.BetAmount,
			"host_locked_amount": t.HostLockedAmount,
			"deltas":             deltas,
			"player_results":     storedResults,
			"resolved_at":        time.Now(),
			"host_balance":       hostNewBalance,
			"player_hands":       playerHandsDoc,
		}
		if err := b.userRepo.CompleteTx(sc, roundID, details); err != nil {
			return err
		}
		return nil
	})

	if errTx != nil {
		if errors.Is(errTx, repository.ErrRoundAlreadyResolved) {
			// Nếu retry mà ván thực tế ĐÃ THANH TOÁN XONG -> Xóa rác và render UI.
			doc, _ := b.userRepo.GetTxDoc(ctx, roundID)
			if doc != nil && doc["status"] == "resolved" {
				log.Printf("[BLACKJACK_RETRY] Table %s already resolved. Rendering UI.", roundID)

				// Khôi phục UI từ DB (Bot restart mất RAM)
				recoveredResults := make(map[string]ui.BJPlayerResult)
				if detailsRaw, ok := doc["details"].(bson.M); ok {
					if pRes, ok := detailsRaw["player_results"].(bson.A); ok {
						for _, item := range pRes {
							m := item.(bson.M)
							recoveredResults[m["user_id"].(string)] = ui.BJPlayerResult{
								Status:  m["status"].(string),
								Net:     m["net"].(int64),
								Balance: m["balance"].(int64),
							}
						}
					}
					hostDelta = detailsRaw["host_net"].(int64)
					hostNewBalance = detailsRaw["host_balance"].(int64)

					// KHÔI PHỤC DỮ LIỆU TỪ DB CHO BỘ NHỚ RAM ĐỂ RENDER (PHÒNG TRƯỜNG HỢP RESTART BOT)
					if t.TableID == "" {
						if v, ok := detailsRaw["table_id"].(string); ok {
							t.TableID = v
						}
					}
					if t.ChannelID == "" {
						if v, ok := detailsRaw["channel_id"].(string); ok {
							t.ChannelID = v
						}
					}
					if t.GuildID == "" {
						if v, ok := detailsRaw["guild_id"].(string); ok {
							t.GuildID = v
						}
					}
					if t.HostID == "" {
						if v, ok := detailsRaw["host_id"].(string); ok {
							t.HostID = v
						}
					}
					if t.BetAmount == 0 {
						if v, ok := detailsRaw["bet_amount"].(int64); ok {
							t.BetAmount = v
						} else if v, ok := detailsRaw["bet_amount"].(int32); ok {
							t.BetAmount = int64(v)
						}
					}
					if t.HostLockedAmount == 0 {
						if v, ok := detailsRaw["host_locked_amount"].(int64); ok {
							t.HostLockedAmount = v
						} else if v, ok := detailsRaw["host_locked_amount"].(int32); ok {
							t.HostLockedAmount = int64(v)
						}
					}
					if t.DurationSec == 0 {
						switch v := detailsRaw["duration_sec"].(type) {
						case int32:
							t.DurationSec = int(v)
						case int64:
							t.DurationSec = int(v)
						case int:
							t.DurationSec = v
						}
					}
					if t.PlayerIDs == nil {
						if pIDs, ok := detailsRaw["player_ids"].(bson.A); ok {
							for _, id := range pIDs {
								if sid, ok := id.(string); ok {
									t.PlayerIDs = append(t.PlayerIDs, sid)
								}
							}
						}
					}
					if handsRaw, ok := detailsRaw["player_hands"].(bson.M); ok {
						if t.Players == nil {
							t.Players = make(map[string]*games.PlayerState)
						}
						for uid, handArray := range handsRaw {
							if t.Players[uid] == nil {
								t.Players[uid] = &games.PlayerState{UserID: uid}
							}
							var hand []games.Card
							if arr, ok := handArray.(bson.A); ok {
								for _, c := range arr {
									if cdoc, ok := c.(bson.M); ok {
										rank := int(cdoc["rank"].(int32))
										suit := games.Suit(cdoc["suit"].(int32))
										hand = append(hand, games.Card{Rank: rank, Suit: suit})
									}
								}
							}
							t.Players[uid].Hand = hand
						}
					}
				}

				t.Mu.Lock()
				log.Printf("[BJ_STATUS] table=%s %s -> finished reason=retry_resolved", t.TableID, t.Status)
				t.Status = "finished"
				t.Mu.Unlock()

				errRender := b.renderBlackjackResult(s, t, recoveredResults, hostDelta, hostNewBalance, t.DurationSec)
				if errRender != nil {
					log.Printf("[BJ_RETRY_RENDER_RESULT_FAIL] table=%s err=%v", t.TableID, errRender)
					t.Mu.Lock()
					t.Status = "result_render_failed"
					t.Mu.Unlock()
					s.ChannelMessageSend(t.ChannelID, ui.Emojis.Error+" Tiền đã xử lý nhưng gửi bảng kết quả khôi phục thất bại do Discord Rate-limit. Vui lòng dùng lệnh "+b.GetPrefix(ctx)+"host để kéo lại kết quả hoặc báo Admin.")
					return
				}

				b.cleanupBlackjackMessages(s, t)
				bjTables.Delete(t.TableID)
			} else {
				log.Printf("[BLACKJACK_RETRY_ERROR] Table %s: Đang kẹt ở processing hoặc lỗi DB", roundID)
			}
			return
		}
		log.Printf("[BLACKJACK_TX_ERROR] Table %s | Players: %d | Err: %v", t.TableID, len(t.PlayerIDs), errTx)
		t.Mu.Lock()
		log.Printf("[BJ_STATUS] table=%s %s -> settlement_failed reason=tx_error", t.TableID, t.Status)
		t.Status = "settlement_failed" // KHÓA BÀN HOÀN TOÀN
		t.Mu.Unlock()
		s.ChannelMessageSend(t.ChannelID, ui.Emojis.Error+" Giao dịch chưa chốt do lỗi hệ thống. Cược đang được giữ an toàn. Vui lòng chờ Admin xử lý.")
		return
	}

	log.Printf("[BJ_RESOLVE_DONE] table=%s", t.TableID)
	t.Mu.Lock()
	log.Printf("[BJ_STATUS] table=%s %s -> finished reason=resolve_success", t.TableID, t.Status)
	t.Status = "finished"
	t.Mu.Unlock()

	errRender := b.renderBlackjackResult(s, t, playerResults, hostDelta, hostNewBalance, t.DurationSec)
	if errRender != nil {
		log.Printf("[BJ_RENDER_RESULT_FAIL] table=%s err=%v", t.TableID, errRender)
		t.Mu.Lock()
		t.Status = "result_render_failed"
		t.Mu.Unlock()
		s.ChannelMessageSend(t.ChannelID, ui.Emojis.Error+" Tiền đã xử lý nhưng gửi bảng kết quả thất bại do Discord Rate-limit. Vui lòng dùng lệnh "+b.GetPrefix(ctx)+"host để kéo lại kết quả hoặc báo Admin.")
		return
	}

	b.cleanupBlackjackMessages(s, t)
	bjTables.Delete(t.TableID)

	// Xử lý tạo ván tiếp theo nếu có chuỗi Auto
	series := GetAutoSeries(t.ChannelID, "xidach")
	if series != nil && series.Active && series.OwnerID == t.HostID {
		if series.CurrentIndex < series.TotalCount {
			go func() {
				time.Sleep(6 * time.Second)
				if GetAutoSeries(t.ChannelID, "xidach") != nil {
					b.startBlackjackRound(s, t.ChannelID, t.HostID, series.GuildID, series.Params.BetAmount, series.Params.DurationSec, series)
				}
			}()
		} else {
			StopAutoSeries(t.ChannelID, "xidach")
			s.ChannelMessageSend(t.ChannelID, fmt.Sprintf("%s <@%s> Đã hoàn thành chuỗi tự động tạo bàn Xì Dách.", ui.Emojis.Success, t.HostID))
		}
	}
}

func (b *Bot) cleanupBlackjackMessages(s *discordgo.Session, t *games.BlackjackTable) {
	if val, ok := bjStates.Load(t.TableID); ok {
		state := val.(*BJState)
		state.mu.Lock()
		if state.ControlMessageID != "" {
			s.ChannelMessageDelete(t.ChannelID, state.ControlMessageID)
			state.ControlMessageID = ""
		}
		state.mu.Unlock()
	}
	bjStates.Delete(t.TableID)
	if t.MessageID != "" {
		_ = s.ChannelMessageDelete(t.ChannelID, t.MessageID)
	}
}

func (b *Bot) cleanupBlackjackMessagesExcept(s *discordgo.Session, t *games.BlackjackTable, keepMsgID string) {
	if val, ok := bjStates.Load(t.TableID); ok {
		state := val.(*BJState)
		state.mu.Lock()
		if state.ControlMessageID != "" && state.ControlMessageID != keepMsgID {
			s.ChannelMessageDelete(t.ChannelID, state.ControlMessageID)
			state.ControlMessageID = ""
		}
		state.mu.Unlock()
	}
	bjStates.Delete(t.TableID)
	if t.MessageID != "" && t.MessageID != keepMsgID {
		_ = s.ChannelMessageDelete(t.ChannelID, t.MessageID)
	}
}

func (b *Bot) renderBlackjackResult(s *discordgo.Session, t *games.BlackjackTable, playerResults map[string]ui.BJPlayerResult, hostDelta int64, hostNewBalance int64, durationSec int) error {

	fmt.Printf("[GAME AUDIT] XIDACH | Table: %s | HostNet: %d\n", t.MessageID, hostDelta)

	var highestWin int64
	var bestPlayer string
	for uid, res := range playerResults {
		if res.Net > highestWin {
			highestWin = res.Net
			bestPlayer = uid
		}
	}

	npcDialog := ""
	if hostDelta > highestWin*2 && hostDelta > 0 {
		dialogue.GetManager().MarkActivity(t.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.DealerDominating, map[string]string{"game": "Xì Dách", "channel": t.ChannelID})
	} else if highestWin > 0 {
		dialogue.GetManager().MarkActivity(t.ChannelID)
		npcDialog = dialogue.GetManager().MaybeSpeak(dialogue.WinBig, map[string]string{"user": bestPlayer, "amount": formatCoins(highestWin), "game": "Xì Dách", "channel": t.ChannelID})
	}

	embed := ui.BuildBlackjackResultEmbed(t, playerResults, hostDelta, hostNewBalance)

	// CACHE KẾT QUẢ ĐỂ KHÔI PHỤC KHI BỊ RATELIMIT
	comps := ui.BlackjackResultButtons(t.HostID, t.BetAmount, durationSec, s)
	CacheGameResult(t.ChannelID, "Xì Dách", npcDialog, embed, comps)

	_, err := s.ChannelMessageSendComplex(t.ChannelID, &discordgo.MessageSend{
		Content:    npcDialog,
		Embed:      embed,
		Components: comps,
	})
	return err
}

func (b *Bot) cmdHost(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	userID := m.Author.ID
	gameName := ""
	if b.repostBlackjackTable(s, m.ChannelID, userID) {
		gameName = "Xì Dách"
	} else if b.repostTaixiuTable(s, m.ChannelID, userID) {
		gameName = "Tài Xỉu"
	} else if b.repostBaicaoTable(s, m.ChannelID, userID) {
		gameName = "Bài Cào"
	} else if b.repostBaucuaTable(s, m.ChannelID, userID) {
		gameName = "Bầu Cua"
	}

	if gameName != "" {
		sendTemp(s, m.ChannelID, fmt.Sprintf("<@%s> Đã kéo bàn **%s** của bạn xuống đây!", userID, gameName), 5*time.Second)
	} else {
		sendTemp(s, m.ChannelID, fmt.Sprintf("<@%s>, bạn không phải là chủ bàn của bất kỳ ván game nào đang hoạt động trong kênh này.", userID), 5*time.Second)
	}
}

func (b *Bot) repostBlackjackTable(s *discordgo.Session, channelID, userID string) bool {
	var targetTable *games.BlackjackTable

	bjTables.Range(func(key, value interface{}) bool {
		table := value.(*games.BlackjackTable)
		if table.ChannelID == channelID && table.HostID == userID &&
			(table.Status == "waiting" || table.Status == "playing" || table.Status == "result_render_failed" || table.Status == "cancel_ui_failed") {
			targetTable = table
			return false
		}
		return true
	})

	if targetTable == nil {
		return false
	}

	targetTable.Mu.Lock()
	oldMainID := targetTable.MessageID
	status := targetTable.Status
	tableID := targetTable.TableID

	var oldCtrlID string
	if val, ok := bjStates.Load(tableID); ok {
		state := val.(*BJState)
		state.mu.Lock()
		oldCtrlID = state.ControlMessageID
		state.mu.Unlock()
	}

	var embedWaiting, embedPlaying *discordgo.MessageEmbed
	if status == "waiting" {
		embedWaiting = ui.BuildBlackjackEmbed(targetTable, ui.Emojis.Loading+" Bàn đang mở... Đợi khách tham gia!")
	} else if status == "playing" {
		embedPlaying = ui.BuildBlackjackEmbed(targetTable, ui.Emojis.Play+" **Ván đấu đang diễn ra!**")
	}
	targetTable.Mu.Unlock()

	if status == "waiting" {
		msg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{embedWaiting},
			Components: ui.BlackjackWaitingButtons(tableID, s),
		})
		if err != nil {
			return false // Nếu gửi lỗi, không làm gì cả, giữ lại bàn cũ
		}
		targetTable.Mu.Lock()
		targetTable.MessageID = msg.ID
		targetTable.Mu.Unlock()

		if oldMainID != "" && oldMainID != msg.ID {
			_ = s.ChannelMessageDelete(channelID, oldMainID)
		}
	} else if status == "playing" {
		mainMsg, errMain := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embedPlaying},
		})
		if errMain != nil {
			return false
		}
		targetTable.Mu.Lock()
		targetTable.MessageID = mainMsg.ID
		targetTable.Mu.Unlock()

		ctrlMsg, errCtrl := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content:    "\u200B",
			Components: ui.BlackjackPlayingButtons(tableID, s),
		})

		if val, ok := bjStates.Load(tableID); ok {
			state := val.(*BJState)
			state.mu.Lock()
			if errCtrl == nil && ctrlMsg != nil {
				state.ControlMessageID = ctrlMsg.ID
				if oldCtrlID != "" && oldCtrlID != ctrlMsg.ID {
					_ = s.ChannelMessageDelete(channelID, oldCtrlID)
				}
			} else {
				state.ControlMessageID = oldCtrlID
			}
			state.mu.Unlock()
		}

		if oldMainID != "" && oldMainID != mainMsg.ID {
			_ = s.ChannelMessageDelete(channelID, oldMainID)
		}
	} else if status == "cancel_ui_failed" {
		embeds := []*discordgo.MessageEmbed{errorEmbed("Bàn Xì Dách đã bị hủy bởi <@" + userID + ">. Tiền cược đã được hoàn lại.")}
		newMsg, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds: embeds,
		})
		if err == nil {
			targetTable.Mu.Lock()
			targetTable.Status = "finished"
			targetTable.Mu.Unlock()
			b.cleanupBlackjackMessages(s, targetTable)
			if oldMainID != "" && oldMainID != newMsg.ID {
				_ = s.ChannelMessageDelete(channelID, oldMainID)
			}
			bjTables.Delete(tableID)
		} else {
			return false
		}
	} else if status == "result_render_failed" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		roundID := "xidach:settlement:" + tableID
		doc, _ := b.userRepo.GetTxDoc(ctx, roundID)
		if doc != nil && doc["status"] == "resolved" {
			recoveredResults := make(map[string]ui.BJPlayerResult)
			var hostDelta, hostNewBalance int64
			if detailsRaw, ok := doc["details"].(bson.M); ok {
				if pRes, ok := detailsRaw["player_results"].(bson.A); ok {
					for _, item := range pRes {
						m := item.(bson.M)
						recoveredResults[m["user_id"].(string)] = ui.BJPlayerResult{
							Status:  m["status"].(string),
							Net:     m["net"].(int64),
							Balance: m["balance"].(int64),
						}
					}
				}
				hostDelta = detailsRaw["host_net"].(int64)
				hostNewBalance = detailsRaw["host_balance"].(int64)
			}

			errRender := b.renderBlackjackResult(s, targetTable, recoveredResults, hostDelta, hostNewBalance, targetTable.DurationSec)
			if errRender == nil {
				targetTable.Mu.Lock()
				targetTable.Status = "finished"
				targetTable.Mu.Unlock()
				b.cleanupBlackjackMessages(s, targetTable)
				if oldMainID != "" {
					_ = s.ChannelMessageDelete(channelID, oldMainID)
				}
				bjTables.Delete(tableID)
			} else {
				return false
			}
		} else {
			s.ChannelMessageSend(channelID, ui.Emojis.Error+" Chưa hỗ trợ khôi phục kết quả từ RAM/DB do thiếu dữ liệu hoặc lỗi DB.")
		}
	}

	return true
}

// rescueStuckBlackjackTable hỗ trợ lệnh !restore tìm và refund bàn Xì Dách kẹt lỗi hệ thống
func (b *Bot) rescueStuckBlackjackTable(ctx context.Context, s *discordgo.Session, channelID string) bool {
	restored := false
	bjTables.Range(func(key, value interface{}) bool {
		table := value.(*games.BlackjackTable)
		if table.ChannelID == channelID && (table.Status == "settlement_failed" || table.Status == "cancel_refund_failed") {
			table.Mu.Lock()
			table.Status = "canceling" // Khóa chống Race Condition
			table.Mu.Unlock()

			// Thực hiện hoàn trả cọc thủ công để chống thất thoát do lỗi DB
			for _, uid := range table.PlayerIDs {
				if uid == table.HostID {
					_ = b.wallet.BetSettle(ctx, uid, table.HostLockedAmount, true)
				} else {
					_ = b.wallet.BetSettle(ctx, uid, table.BetAmount, true)
				}
			}

			table.Mu.Lock()
			table.Status = "finished"
			table.Mu.Unlock()
			bjTables.Delete(table.TableID)

			s.ChannelMessageSend(channelID, fmt.Sprintf("%s **[CỨU HỘ KHẨN CẤP]** Đã phát hiện bàn Xì Dách kẹt lỗi hệ thống (`%s`). Đã tự động hoàn trả toàn bộ cọc để chống thất thoát tài sản!", ui.Emojis.Shield, table.TableID))
			restored = true
			return false // Xử lý 1 bàn mỗi lần
		}
		return true
	})
	return restored
}
