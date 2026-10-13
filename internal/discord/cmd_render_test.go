package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/discordcasino/v2/internal/monitoring"
)

func TestRenderStatusShowsUnknownUntilMetricsExist(t *testing.T) {
	embed := renderStatusEmbed(monitoring.NewMonitor().Health())
	var foundHeartbeat, foundDatabase, foundCommand bool
	for _, field := range embed.Fields {
		switch field.Name {
		case "Gateway heartbeat":
			foundHeartbeat = strings.Contains(field.Value, "Chưa có mẫu đo")
		case "MongoDB ping":
			foundDatabase = strings.Contains(field.Value, "Chưa có mẫu đo")
		case "Command handler latency":
			foundCommand = strings.Contains(field.Value, "Chưa có mẫu đo")
		}
	}
	if !foundHeartbeat || !foundDatabase || !foundCommand {
		t.Fatalf("dashboard should report missing samples rather than inventing metrics: %+v", embed.Fields)
	}
}

func TestRenderStatusExplainsMetricMeaning(t *testing.T) {
	embed := renderStatusEmbed(monitoring.NewMonitor().Health())
	for _, field := range embed.Fields {
		if field.Name == "Cách đọc các chỉ số" {
			for _, explanation := range []string{"Gateway heartbeat", "MongoDB ping", "P50", "một lõi CPU", "không bao gồm mọi dòng log"} {
				if !strings.Contains(field.Value, explanation) {
					t.Errorf("metrics explanation is missing %q", explanation)
				}
			}
			return
		}
	}
	t.Fatal("metrics explanation field missing")
}

func TestPrivateCommandAllowlist(t *testing.T) {
	for _, command := range []string{"help", "info", "balance", "cash", "history", "render", "upd"} {
		if !isPrivateCommandAllowed(command) {
			t.Errorf("expected %q to be allowed in DMs", command)
		}
	}
	for _, command := range []string{"pay", "admin", "taixiu", "delete", "unknown"} {
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
		if field.Name == "Command handler latency" {
			if !strings.Contains(field.Value, "P50 25ms") || !strings.Contains(field.Value, "n=1") {
				t.Fatalf("unexpected command latency output: %s", field.Value)
			}
			return
		}
	}
	t.Fatal("command latency field missing")
}
