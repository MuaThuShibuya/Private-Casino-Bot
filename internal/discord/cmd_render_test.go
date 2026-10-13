package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/discordcasino/v2/internal/monitoring"
)

func TestRenderStatusShowsUnknownUntilMetricsExist(t *testing.T) {
	embed := renderStatusEmbed(monitoring.NewMonitor().Health())
	var foundConnection, foundPerformance bool
	for _, field := range embed.Fields {
		switch field.Name {
		case "Kết nối":
			foundConnection = strings.Contains(field.Value, "Chưa có mẫu đo")
		case "Hiệu năng":
			foundPerformance = strings.Contains(field.Value, "Chưa có mẫu đo")
		}
	}
	if !foundConnection || !foundPerformance {
		t.Fatalf("info should report missing samples rather than inventing metrics: %+v", embed.Fields)
	}
}

func TestRenderStatusExplainsMetricMeaning(t *testing.T) {
	embed := renderStatusEmbed(monitoring.NewMonitor().Health())
	if embed.Footer == nil {
		t.Fatal("info footer is missing metric explanations")
	}
	for _, explanation := range []string{"Độ trễ", "P95", "RAM Go"} {
		if !strings.Contains(embed.Footer.Text, explanation) {
			t.Errorf("short metric explanation is missing %q", explanation)
		}
	}
}

func TestRenderInfoDoesNotExposeIdentifiersOrLinks(t *testing.T) {
	sensitive := []string{
		"secret-service-name",
		"https://git.example/private.git",
		"secret-branch",
		"deadbeef1234567890",
		"srv-sensitive123",
		"https://dashboard.render.com/web/srv-sensitive123",
	}
	snapshot := monitoring.RenderSnapshot{
		Service: monitoring.RenderService{
			ID:           sensitive[4],
			Name:         sensitive[0],
			Type:         "web_service",
			Repo:         sensitive[1],
			Branch:       sensitive[2],
			DashboardURL: sensitive[5],
		},
		Deploys: []monitoring.RenderDeploy{{
			ID:        "deploy-sensitive",
			Status:    "live",
			CreatedAt: "2026-10-13T09:00:00Z",
			Commit: struct {
				ID        string `json:"id"`
				Message   string `json:"message"`
				CreatedAt string `json:"createdAt"`
			}{ID: sensitive[3]},
		}},
		APILatency: 50 * time.Millisecond,
	}
	embed := renderStatusEmbed(monitoring.NewMonitor().Health())
	appendRenderServiceFields(embed, snapshot)

	if embed.URL != "" {
		t.Fatalf("info should not include a service/dashboard URL: %q", embed.URL)
	}
	for _, field := range embed.Fields {
		for _, value := range sensitive {
			if strings.Contains(field.Value, value) {
				t.Errorf("info exposed sensitive value %q in field %q", value, field.Name)
			}
		}
		if strings.Contains(field.Value, "deploy-sensitive") {
			t.Errorf("info exposed deploy identifier in field %q", field.Name)
		}
	}
}

func TestPrivateCommandAllowlist(t *testing.T) {
	for _, command := range []string{"help", "info", "balance", "cash", "history", "upd"} {
		if !isPrivateCommandAllowed(command) {
			t.Errorf("expected %q to be allowed in DMs", command)
		}
	}
	for _, command := range []string{"render", "pay", "admin", "taixiu", "delete", "unknown"} {
		if isPrivateCommandAllowed(command) {
			t.Errorf("expected %q to be blocked in DMs", command)
		}
	}
}

func TestDiscordSafeTextPreservesUnicodeAndNeutralizesMentions(t *testing.T) {
	got := discordSafeText("á😀@everyone`text`", 8)
	if !strings.HasPrefix(got, "á😀@\u200b") || strings.Contains(got, "@everyone") || strings.Contains(got, "`") {
		t.Fatalf("text was not safely normalized: %q", got)
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected bounded text, got %q", got)
	}
}

func TestRenderStatusUsesObservedCommandLatency(t *testing.T) {
	monitor := monitoring.NewMonitor()
	monitor.RecordCommand(25 * time.Millisecond)
	embed := renderStatusEmbed(monitor.Health())
	for _, field := range embed.Fields {
		if field.Name == "Hiệu năng" {
			if !strings.Contains(field.Value, "P50 25ms") {
				t.Fatalf("unexpected command latency output: %s", field.Value)
			}
			return
		}
	}
	t.Fatal("performance field missing")
}
