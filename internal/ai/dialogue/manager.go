package dialogue

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/discordcasino/v2/internal/ui"
)

type Event string

const (
	WinBig             Event = "WIN_BIG"
	WinSmall           Event = "WIN_SMALL"
	Lose               Event = "LOSE"
	LoseStreak         Event = "LOSE_STREAK"
	AllIn              Event = "ALL_IN"
	Jackpot            Event = "JACKPOT"
	Comeback           Event = "COMEBACK"
	TableIdle          Event = "TABLE_IDLE"
	NewPlayer          Event = "NEW_PLAYER"
	DealerDominating   Event = "DEALER_DOMINATING"
	PlayerRage         Event = "PLAYER_RAGE"
	PlayerInsultsCoder Event = "PLAYER_INSULTS_CODER"
	MythicLines        Event = "MYTHIC_LINES"
)

type ButlerConfig struct {
	Metadata   map[string]string            `json:"metadata"`
	Frames     []string                     `json:"frames"`
	Categories map[string]CategoryDialogues `json:"categories"`
}

type CategoryDialogues struct {
	Common []string `json:"common,omitempty"`
	Rare   []string `json:"rare,omitempty"`
	Mythic []string `json:"mythic,omitempty"`
}

type NPCManager struct {
	mu              sync.RWMutex
	config          ButlerConfig
	rng             *rand.Rand
	recentLines     []string
	userCooldowns   sync.Map
	globalCooldown  time.Time
	channelActivity sync.Map
}

var (
	instance *NPCManager
	once     sync.Once
)

func GetManager() *NPCManager {
	once.Do(func() {
		instance = &NPCManager{
			rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
			recentLines: make([]string, 0, 5),
		}
		if err := instance.LoadConfig("internal/ai/dialogue/abyss_butler.json"); err != nil {
			log.Printf("[NPC] Không thể tải hệ thống hội thoại: %v", err)
		}
	})
	return instance
}

func (m *NPCManager) LoadConfig(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg ButlerConfig
	if err := json.Unmarshal(file, &cfg); err != nil {
		return err
	}

	m.mu.Lock()
	m.config = cfg
	m.mu.Unlock()
	return nil
}

func (m *NPCManager) MarkActivity(channelID string) {
	m.channelActivity.Store(channelID, time.Now())
}

// MaybeSpeak là hàm chính xử lý logic Cooldown, Memory và Weights
func (m *NPCManager) MaybeSpeak(event Event, variables map[string]string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Kiểm tra Global Cooldown (10s)
	if time.Now().Before(m.globalCooldown) {
		return ""
	}

	// 2. Kiểm tra Per-User Cooldown (30s)
	userID := variables["user"]
	if userID != "" {
		if lastTime, ok := m.userCooldowns.Load(userID); ok {
			if time.Now().Before(lastTime.(time.Time)) {
				return ""
			}
		}
	}

	// 3. Logic Table Idle (Phải im lặng > 3 phút mới được nói câu Idle)
	if event == TableIdle {
		channelID := variables["channel"]
		if channelID != "" {
			if lastActive, ok := m.channelActivity.Load(channelID); ok {
				if time.Since(lastActive.(time.Time)) < 3*time.Minute {
					return ""
				}
			}
		}
	}

	// 4. Random Chance (Trừ sự kiện lớn, Quản gia chỉ có 60% cơ hội lên tiếng)
	if event != Jackpot && event != PlayerInsultsCoder && event != Comeback {
		if m.rng.Intn(100) > 60 {
			return ""
		}
	}

	catData, exists := m.config.Categories[string(event)]
	if !exists {
		return ""
	}

	var pool []string
	roll := m.rng.Intn(100)
	if roll < 5 && len(catData.Mythic) > 0 {
		pool = catData.Mythic
	} else if roll < 30 && len(catData.Rare) > 0 {
		pool = catData.Rare
	} else if len(catData.Common) > 0 {
		pool = catData.Common
	} else {
		pool = catData.Mythic // Fallback
	}

	if len(pool) == 0 {
		return ""
	}

	// Lấy Random chống trùng lặp (Memory 5 câu gần nhất)
	var line string
	for i := 0; i < 3; i++ {
		line = pool[m.rng.Intn(len(pool))]
		isRepeated := false
		for _, recent := range m.recentLines {
			if line == recent {
				isRepeated = true
				break
			}
		}
		if !isRepeated {
			break
		}
	}

	// Cập nhật State
	if len(m.recentLines) >= 5 {
		m.recentLines = m.recentLines[1:]
	}
	m.recentLines = append(m.recentLines, line)
	m.globalCooldown = time.Now().Add(10 * time.Second)
	if userID != "" {
		m.userCooldowns.Store(userID, time.Now().Add(30*time.Second))
	}

	// 1. Render Frame
	frame := m.config.Frames[m.rng.Intn(len(m.config.Frames))]
	output := strings.Replace(frame, "{dialogue}", line, 1)

	// 2. Render Context Variables
	for key, val := range variables {
		if key == "user" {
			val = "<@" + val + ">"
		}
		output = strings.ReplaceAll(output, "{"+key+"}", val)
	}

	// 3. Render Emoji Tokens
	output = ui.ReplaceEmojiTokens(output)

	return output
}
