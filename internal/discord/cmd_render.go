package discord

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/monitoring"
	"github.com/discordcasino/v2/internal/ui"
	"github.com/discordcasino/v2/internal/view"
)

func (b *Bot) cmdRender(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleAdmin {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Lệnh này chỉ dành cho Admin và Owner.", 4*time.Second)
		return
	}

	channel, progress, err := sendRenderPrivateProgress(s, m, ui.Emojis.Loading+" Đang kiểm tra trạng thái bot và Render...")
	if err != nil {
		return
	}

	health := b.getMonitor().Health()
	if latency := s.HeartbeatLatency(); latency > 0 {
		health.GatewayLatency = latency
		health.GatewayLatencySeen = true
	}

	embed := renderStatusEmbed(health)
	if b.render == nil || !b.render.Configured() {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   ui.Emojis.Warning + " Render API",
			Value:  "Chưa cấu hình. Cần `RENDER_API_KEY` và `RENDER_SERVICE_ID` trong môi trường server.",
			Inline: false,
		})
	} else {
		renderCtx, renderCancel := context.WithTimeout(ctx, 6*time.Second)
		snapshot, renderErr := b.render.Snapshot(renderCtx)
		renderCancel()
		if renderErr != nil {
			b.getMonitor().RecordError("Render API", renderErrorLabel(renderErr))
			log.Printf("[Render] Không tải được trạng thái Render: %s", renderErrorLabel(renderErr))
			health = b.getMonitor().Health()
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   ui.Emojis.Error + " Render API",
				Value:  renderErrorLabel(renderErr),
				Inline: false,
			})
		} else {
			appendRenderServiceFields(embed, snapshot)
		}
	}

	if errorsFound := formatRecentErrors(health.RecentErrors, 4); errorsFound != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   ui.Emojis.Warning + " Nhật ký lỗi gần đây",
			Value:  errorsFound,
			Inline: false,
		})
	}
	embed.Timestamp = time.Now().UTC().Format(time.RFC3339)
	if _, err := s.ChannelMessageEditEmbed(channel.ID, progress.ID, embed); err != nil {
		log.Printf("[Render] Không thể cập nhật bảng trạng thái: %v", err)
	}
}

func (b *Bot) cmdUpdateRender(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	role := b.GetRole(ctx, s, m.GuildID, m.Author.ID, m.Member)
	if role < RoleOwner {
		sendTemp(s, m.ChannelID, ui.Emojis.Block+" Chỉ Owner mới được trigger deploy Render.", 4*time.Second)
		return
	}
	if b.render == nil || !b.render.Configured() {
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Render API chưa cấu hình. Hãy thêm `RENDER_API_KEY` và `RENDER_SERVICE_ID` trên Render.", 7*time.Second)
		return
	}

	channel, progress, err := sendRenderPrivateProgress(s, m, ui.Emojis.Loading+" Đang xác minh service và yêu cầu Render build commit mới nhất...")
	if err != nil {
		return
	}

	deploy, deployErr := b.render.TriggerDeploy(ctx)
	if deployErr != nil {
		label := renderErrorLabel(deployErr)
		b.getMonitor().RecordError("Render deploy", label)
		log.Printf("[Render] Không thể trigger deploy: %s", label)
		embed := view.ErrorEmbed(label)
		embed.Color = 0x1E0B2D
		if _, err := s.ChannelMessageEditEmbed(channel.ID, progress.ID, embed); err != nil {
			log.Printf("[Render] Không thể cập nhật kết quả !upd: %v", err)
		}
		return
	}

	description := fmt.Sprintf("%s Đã gửi yêu cầu build/deploy commit mới nhất từ nhánh đã cấu hình.", ui.Emojis.Success)
	description += fmt.Sprintf("\n**Trạng thái:** `%s`", displayDeployStatus(deploy.Status))
	if deploy.ID != "" {
		description += fmt.Sprintf("\n**Deploy ID:** `%s`", deploy.ID)
	}
	embed := view.NewEmbed(ui.Emojis.Crown+" RENDER DEPLOY", description)
	embed.Color = 0x1E0B2D
	if b.render != nil {
		if snapshot, err := b.render.Snapshot(ctx); err == nil && snapshot.Service.DashboardURL != "" {
			embed.URL = snapshot.Service.DashboardURL
			embed.Footer = &discordgo.MessageEmbedFooter{Text: "Mở Render Dashboard để theo dõi build và logs"}
		}
	}
	if _, err := s.ChannelMessageEditEmbed(channel.ID, progress.ID, embed); err != nil {
		log.Printf("[Render] Không thể cập nhật kết quả !upd: %v", err)
	}
}

func sendRenderPrivateProgress(s *discordgo.Session, m *discordgo.MessageCreate, content string) (*discordgo.Channel, *discordgo.Message, error) {
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		log.Printf("[Render] Không thể mở DM cho user_id=%s: %v", m.Author.ID, err)
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không mở được DM để gửi thông tin Render riêng tư. Hãy bật nhận tin nhắn trực tiếp từ bot.", 6*time.Second)
		return nil, nil, err
	}
	progress, err := s.ChannelMessageSend(channel.ID, content)
	if err != nil {
		log.Printf("[Render] Không thể gửi DM trạng thái cho user_id=%s: %v", m.Author.ID, err)
		sendTemp(s, m.ChannelID, ui.Emojis.Error+" Không gửi được DM trạng thái Render.", 6*time.Second)
		return nil, nil, err
	}
	return channel, progress, nil
}

func renderStatusEmbed(health monitoring.HealthSnapshot) *discordgo.MessageEmbed {
	embed := view.NewEmbed(ui.Emojis.Crown+" BOT INFO", "Tổng quan hoạt động · cập nhật "+relativeAge(health.LastUpdated))
	embed.Color = 0x1E0B2D
	uptime := time.Since(health.StartedAt).Round(time.Second)
	if uptime < 0 {
		uptime = 0
	}
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name: "Tình trạng",
			Value: fmt.Sprintf("Bot %s\nDiscord %s\nMongoDB %s",
				statusLabel(health.ProcessStatus, false),
				statusLabel(health.GatewayStatus, health.GatewayStale),
				statusLabel(health.DatabaseStatus, health.DatabaseStale)),
			Inline: true,
		},
		{
			Name: "Kết nối",
			Value: fmt.Sprintf("Discord · %s\nMongoDB · %s\nReconnect · %d",
				formatLatency(health.GatewayLatency, health.GatewayLatencySeen),
				formatLatency(health.DatabaseLatency, health.DatabaseLatencySeen),
				health.Reconnects),
			Inline: true,
		},
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	cpuUsage := "Chưa có mẫu đo"
	if health.ProcessCPUSeen {
		cpuUsage = fmt.Sprintf("%.1f%%", health.ProcessCPUPercent)
	}
	commandLatency := "Chưa có mẫu đo"
	if health.CommandLatency.Count > 0 {
		commandLatency = fmt.Sprintf("P50 %s · P95 %s",
			health.CommandLatency.P50.Round(time.Millisecond),
			health.CommandLatency.P95.Round(time.Millisecond))
	}
	embed.Fields = append(embed.Fields,
		&discordgo.MessageEmbedField{
			Name: "Hiệu năng",
			Value: fmt.Sprintf("CPU %s · RAM Go %.1f MiB\nLệnh %s",
				cpuUsage, float64(mem.HeapAlloc)/(1024*1024), commandLatency),
			Inline: true,
		},
		&discordgo.MessageEmbedField{
			Name:   "Thời gian chạy",
			Value:  uptime.String(),
			Inline: true,
		},
	)
	if errorsFound := formatRecentErrors(health.RecentErrors, 3); errorsFound != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("Sự cố gần đây · %d/15 phút", health.Errors15m),
			Value:  errorsFound,
			Inline: false,
		})
	} else {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Sự cố · 15 phút",
			Value:  fmt.Sprintf("%d ghi nhận", health.Errors15m),
			Inline: true,
		})
	}
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "Độ trễ càng thấp càng tốt · P95: 95% lệnh nhanh hơn · RAM Go không phải tổng RAM máy chủ",
	}
	return embed
}

func appendRenderServiceFields(embed *discordgo.MessageEmbed, snapshot monitoring.RenderSnapshot) {
	service := snapshot.Service
	autoDeploy := service.AutoDeployTrigger
	if autoDeploy == "" {
		autoDeploy = service.AutoDeploy
	}
	if autoDeploy == "" {
		autoDeploy = "không có dữ liệu"
	}
	serviceState := ui.Emojis.Success + " Hoạt động"
	if service.Suspended == "suspended" {
		serviceState = ui.Emojis.Warning + " Tạm ngưng"
	}
	serviceInfo := fmt.Sprintf("%s · %s\nAuto-deploy: %s · API %s",
		discordSafeText(service.Type, 32), serviceState,
		discordSafeText(autoDeploy, 32), formatLatency(snapshot.APILatency, snapshot.APILatency > 0))
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   ui.Emojis.Settings + " Render",
		Value:  serviceInfo,
		Inline: false,
	})

	if len(snapshot.Deploys) == 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "Deploy gần nhất", Value: "Chưa có dữ liệu.", Inline: false,
		})
		return
	}

	deploy := snapshot.Deploys[0]
	deployInfo := fmt.Sprintf("%s", displayDeployStatus(deploy.Status))
	if deploy.CreatedAt != "" {
		deployInfo += "\n" + discordSafeText(deploy.CreatedAt, 32)
	}
	if isFailedDeploy(deploy.Status) {
		deployInfo += "\nXem build log trên Render"
	}
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   ui.Emojis.Dice + " Deploy gần nhất",
		Value:  deployInfo,
		Inline: false,
	})
}

func formatRecentErrors(events []monitoring.Event, limit int) string {
	if len(events) == 0 {
		return ""
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	var lines []string
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		lines = append(lines, fmt.Sprintf("`%s` **%s:** %s",
			event.Time.Format("15:04:05"), discordSafeText(event.Component, 32), discordSafeText(event.Message, 140)))
	}
	return strings.Join(lines, "\n")
}

func renderErrorLabel(err error) string {
	switch {
	case errors.Is(err, monitoring.ErrRenderNotConfigured):
		return "Chưa cấu hình Render API."
	case errors.Is(err, monitoring.ErrDeployInProgress):
		return "Đang có deploy chạy; đã bỏ qua để tránh trigger trùng."
	case errors.Is(err, monitoring.ErrGitSourceMissing):
		return "Không xác minh được Git repository/branch của service; đã hủy thao tác."
	case errors.Is(err, monitoring.ErrServiceSuspended):
		return "Render service đang suspended; đã hủy thao tác."
	case errors.Is(err, monitoring.ErrRenderTimeout):
		return "Render API timeout; không xác minh được trạng thái."
	}
	var apiErr *monitoring.RenderAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Render API từ chối xác thực/quyền truy cập (HTTP " + fmt.Sprint(apiErr.StatusCode) + ")."
		default:
			return fmt.Sprintf("Render API trả HTTP %d.", apiErr.StatusCode)
		}
	}
	return "Không thể kết nối Render API."
}

func statusLabel(status string, stale bool) string {
	if stale {
		return ui.Emojis.Warning + " DỮ LIỆU CŨ"
	}
	switch status {
	case "connected", "running":
		return ui.Emojis.Success + " " + strings.ToUpper(status)
	case "connecting", "reconnecting", "starting", "degraded":
		return ui.Emojis.Warning + " " + strings.ToUpper(status)
	case "disconnected", "stopping", "error":
		return ui.Emojis.Error + " " + strings.ToUpper(status)
	default:
		return ui.Emojis.Warning + " CHƯA XÁC MINH"
	}
}

func formatLatency(value time.Duration, seen bool) string {
	if !seen {
		return "Chưa có mẫu đo"
	}
	return value.Round(time.Millisecond).String()
}

func displayDeployStatus(status string) string {
	if status == "" {
		return "unknown"
	}
	return strings.ReplaceAll(status, "_", " ")
}

func isFailedDeploy(status string) bool {
	switch status {
	case "build_failed", "update_failed", "pre_deploy_failed":
		return true
	default:
		return false
	}
}

func discordSafeText(value string, limit int) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "`", "'")
	value = strings.ReplaceAll(value, "@", "@\u200b")
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-3]) + "..."
	}
	return value
}

func relativeAge(updated time.Time) string {
	if updated.IsZero() {
		return "Chưa có dữ liệu"
	}
	return time.Since(updated).Round(time.Second).String() + " trước"
}
