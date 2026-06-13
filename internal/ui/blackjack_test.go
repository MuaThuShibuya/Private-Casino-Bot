package ui

import (
	"fmt"
	"strings"
	"testing"

	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

func TestBlackjackResultUI_StrictRequirements(t *testing.T) {
	// Giả lập Table và Kết Quả từ Backend truyền sang
	table := &games.BlackjackTable{
		HostID: "host",
		Players: map[string]*games.PlayerState{
			"host": {Hand: []games.Card{{Suit: games.SuitHeart, Rank: 10}, {Suit: games.SuitSpade, Rank: 8}}},                                   // 18 điểm
			"p1":   {Hand: []games.Card{{Suit: games.SuitHeart, Rank: 10}, {Suit: games.SuitSpade, Rank: 5}}},                                   // 15 điểm (Non/Đền)
			"p2":   {Hand: []games.Card{{Suit: games.SuitHeart, Rank: 10}, {Suit: games.SuitSpade, Rank: 10}, {Suit: games.SuitClub, Rank: 8}}}, // 28 điểm (Già/Đền)
		},
		PlayerIDs: []string{"host", "p1", "p2"},
	}

	// Dữ liệu từ Backend (Deltas & DB)
	results := map[string]BJPlayerResult{
		"p1": {Status: "ĐỀN BÀN (15)", Net: -15000, Balance: -15000}, // Người chơi dùng Tín dụng nên số dư âm
		"p2": {Status: "ĐỀN BÀN (28)", Net: -15000, Balance: 0},
	}
	hostNet := int64(30000)
	hostNewBalance := int64(130000)

	// Render Embed
	embed := BuildBlackjackResultEmbed(table, results, hostNet, hostNewBalance)
	textOutput := embed.Description

	// Assertions - Các nguyên tắc không được phá
	if !strings.Contains(textOutput, "-15.000") {
		t.Errorf("UI không hiển thị đúng số dư âm/Net âm: %s", textOutput)
	}
	if !strings.Contains(textOutput, "Lời/Lỗ: **-15.000**") {
		t.Errorf("UI không hiển thị lỗ -15.000: %s", textOutput)
	}
	if !strings.Contains(textOutput, "ĐỀN BÀN (15)") || !strings.Contains(textOutput, "ĐỀN BÀN (28)") {
		t.Errorf("UI không hiển thị đúng Text Đền Bàn: %s", textOutput)
	}
	if strings.Contains(textOutput, "Được đền thay") {
		t.Errorf("UI in nhầm chữ 'Được đền thay' cho người Đền Bàn!")
	}
}

func TestBlackjackResultUI_ContinueButton(t *testing.T) {
	hostID := "host123"
	bet := int64(5000)
	duration := 90

	// Render nút bấm kết quả
	comps := BlackjackResultButtons(hostID, bet, duration, nil)
	ar := comps[0].(discordgo.ActionsRow)
	continueBtn := ar.Components[0].(discordgo.Button)

	expectedID := "xd:continue:host123:5000:90"
	if continueBtn.CustomID != expectedID {
		t.Errorf("CustomID của nút Continue không đúng. Got '%s', want '%s'", continueBtn.CustomID, expectedID)
	}
}

func TestBlackjackUI_Buttons(t *testing.T) {
	// Lấy nút Rút Bài (Hit) thông qua hàm Render chung
	comps := BlackjackPlayingButtons("table_id", nil)

	// Phân tách Component Discord
	ar := comps[0].(discordgo.ActionsRow)
	var hitBtn discordgo.Button
	for _, c := range ar.Components {
		btn := c.(discordgo.Button)
		if strings.Contains(btn.CustomID, "xd:hit") {
			hitBtn = btn
		}
	}

	if hitBtn.Disabled {
		t.Errorf("Nút Rút bài (Hit) KHÔNG ĐƯỢC PHÉP bị vô hiệu hóa ở tầng UI để duy trì quyền tự quyết Ngũ Linh.")
	}
}

func TestBlackjackUI_CountdownDisplay(t *testing.T) {
	t.Run("UI Trạng thái Waiting phải hiện đúng DurationSec", func(t *testing.T) {
		table := &games.BlackjackTable{
			Status:      "waiting",
			DurationSec: 90, // Bàn thiết lập 90 giây
			BetAmount:   5000,
			HostID:      "host",
		}

		embed := BuildBlackjackEmbed(table, "Đang chờ")
		desc := embed.Description

		if !strings.Contains(desc, "Thời gian rút/dừng: **90s**") {
			t.Errorf("Lỗi: Trạng thái Waiting không hiển thị đúng thời gian cài đặt 90s. Description: %s", desc)
		}
	})

	t.Run("UI Trạng thái Playing phải dùng Discord Timestamp của Deadline", func(t *testing.T) {
		fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		table := &games.BlackjackTable{
			Status:   "playing",
			Deadline: fixedTime, // Backend gửi sang fixedTime
			HostID:   "host",
			Players: map[string]*games.PlayerState{
				"host": {Hand: []games.Card{{Suit: games.SuitHeart, Rank: 10}}},
			},
		}

		embed := BuildBlackjackEmbed(table, "Ván đang diễn ra")
		desc := embed.Description

		expectedTimestamp := fmt.Sprintf("<t:%d:R>", fixedTime.Unix())
		if !strings.Contains(desc, expectedTimestamp) {
			t.Errorf("Lỗi: Embed không render đúng định dạng thời gian Discord %s. Description: %s", expectedTimestamp, desc)
		}
	})
}

func TestBlackjackUI_BuildBlackjackEmbed_WaitingFlows(t *testing.T) {
	table := &games.BlackjackTable{
		Status:      "waiting",
		HostID:      "host1",
		BetAmount:   10000,
		DurationSec: 60,
		Players: map[string]*games.PlayerState{
			"host1": {},
		},
		PlayerIDs: []string{"host1"},
	}

	t.Run("Bàn mới mở chưa ai tham gia", func(t *testing.T) {
		embed := BuildBlackjackEmbed(table, "Đang chờ")
		desc := embed.Description

		if !strings.Contains(desc, "Chưa có ai tham gia") {
			t.Errorf("Phải thông báo chưa có ai tham gia khi chỉ có mỗi Host")
		}
	})

	t.Run("Bàn đã có khách tham gia", func(t *testing.T) {
		table.PlayerIDs = append(table.PlayerIDs, "p1")
		table.Players["p1"] = &games.PlayerState{}

		embed := BuildBlackjackEmbed(table, "Đang chờ")
		desc := embed.Description

		if strings.Contains(desc, "Chưa có ai tham gia") {
			t.Errorf("Đã có p1 thì KHÔNG được báo chưa có ai tham gia")
		}
		if !strings.Contains(desc, "<@p1>") {
			t.Errorf("Phải hiển thị người chơi p1 trong danh sách")
		}
	})
}
