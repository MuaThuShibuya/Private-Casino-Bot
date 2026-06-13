package games

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Bộ bài — 4 chất
type Suit int

const (
	SuitHeart   Suit = iota // Cơ ♥
	SuitDiamond             // Rô ♦
	SuitClub                // Nhép ♣
	SuitSpade               // Pích ♠
)

// Card đại diện cho 1 lá bài.
type Card struct {
	Suit Suit
	Rank int // 1=Át, 2–10, 11=J, 12=Q, 13=K
}

// Value trả về giá trị điểm của lá bài (J/Q/K = 10, Át xử lý riêng trong HandValue).
func (c Card) Value() int {
	if c.Rank >= 10 {
		return 10
	}
	return c.Rank
}

// HandValue tính điểm tối ưu cho bộ bài (Át = 11 hoặc 1, tự chọn).
func HandValue(hand []Card) int {
	total := 0
	aces := 0
	for _, c := range hand {
		if c.Rank == 1 {
			aces++
		} else {
			total += c.Value()
		}
	}

	sums := map[int]bool{
		total: true,
	}

	for i := 0; i < aces; i++ {
		next := map[int]bool{}
		for sum := range sums {
			next[sum+1] = true
			next[sum+10] = true
			next[sum+11] = true
		}
		sums = next
	}

	bestSafe := -1
	minOver := int(^uint(0) >> 1)

	for sum := range sums {
		if sum <= 21 && sum > bestSafe {
			bestSafe = sum
		}
		if sum > 21 && sum < minOver {
			minOver = sum
		}
	}
	if bestSafe >= 0 {
		return bestSafe
	}
	return minOver
}

// IsBlackjack kiểm tra natural 21 (2 lá đầu = 21).
func IsBlackjack(hand []Card) bool {
	return len(hand) == 2 && HandValue(hand) == 21
}

// IsXiBan kiểm tra 2 lá Át (Xì Bàn)
func IsXiBan(hand []Card) bool {
	return len(hand) == 2 && hand[0].Rank == 1 && hand[1].Rank == 1
}

// IsXiDach kiểm tra 1 Át và 1 lá 10/J/Q/K (Xì Dách)
func IsXiDach(hand []Card) bool {
	if len(hand) != 2 {
		return false
	}
	hasA := hand[0].Rank == 1 || hand[1].Rank == 1
	has10 := hand[0].Value() == 10 || hand[1].Value() == 10
	return hasA && has10
}

// IsNguLinh kiểm tra Ngũ Linh (5 lá <= 21)
func IsNguLinh(hand []Card) bool {
	return len(hand) == 5 && HandValue(hand) <= 21
}

// IsBust kiểm tra vượt quá 21.
func IsBust(hand []Card) bool {
	return HandValue(hand) > 21
}

type HandRank int

const (
	RankBust HandRank = iota
	RankNormal
	RankNguLinh
	RankXiDach
	RankXiBan
)

type HandResult struct {
	Rank       HandRank
	Points     int
	Multiplier float64
	Label      string
	IsDenBan   bool
	IsQuac     bool
	IsNon      bool
	IsGia      bool
}

func EvaluateHand(hand []Card, isHost bool, payoutXiBan, payoutXiDach, payoutNguLinh float64) HandResult {
	res := HandResult{}
	res.Points = HandValue(hand)

	if IsXiBan(hand) {
		res.Rank = RankXiBan
		res.Multiplier = payoutXiBan
		res.Label = "Xì Bàn"
		return res
	}
	if IsXiDach(hand) {
		res.Rank = RankXiDach
		res.Multiplier = payoutXiDach
		res.Label = "Xì Dách"
		return res
	}
	if IsNguLinh(hand) {
		res.Rank = RankNguLinh
		res.Multiplier = payoutNguLinh
		res.Label = "Ngũ Linh"
		return res
	}

	res.Multiplier = 1.0

	if res.Points >= 28 {
		res.Rank = RankBust
		res.IsGia = true
		res.IsDenBan = true
		res.Label = fmt.Sprintf("Già (%d)", res.Points)
		return res
	}
	if res.Points > 21 {
		res.Rank = RankBust
		res.IsQuac = true
		res.Label = fmt.Sprintf("Quắc (%d)", res.Points)
		return res
	}
	if isHost {
		if res.Points < 15 {
			res.Rank = RankBust
			res.IsNon = true
			res.IsDenBan = true
			res.Label = fmt.Sprintf("Non (%d)", res.Points)
		} else {
			res.Rank = RankNormal
			res.Label = fmt.Sprintf("%d điểm", res.Points)
		}
	} else {
		if res.Points <= 15 {
			res.Rank = RankBust
			res.IsNon = true
			res.IsDenBan = true
			res.Label = fmt.Sprintf("Non (%d)", res.Points)
		} else {
			res.Rank = RankNormal
			res.Label = fmt.Sprintf("%d điểm", res.Points)
		}
	}
	return res
}

type PlayerMatchResult struct {
	Net    int64
	Status string
}

func CalculateBlackjackSettlement(hostID string, playerIDs []string, hands map[string][]Card, betAmount int64, payoutXiBan, payoutXiDach, payoutNguLinh float64) (map[string]int64, map[string]PlayerMatchResult) {
	deltas := make(map[string]int64)
	playerResults := make(map[string]PlayerMatchResult)

	hostRes := EvaluateHand(hands[hostID], true, payoutXiBan, payoutXiDach, payoutNguLinh)
	hostHasSpecial := hostRes.Rank >= RankNguLinh

	playerResMap := make(map[string]HandResult)
	var denBanPlayers []string

	for _, uid := range playerIDs {
		if uid == hostID {
			continue
		}
		res := EvaluateHand(hands[uid], false, payoutXiBan, payoutXiDach, payoutNguLinh)
		playerResMap[uid] = res

		if !hostHasSpecial && !hostRes.IsDenBan && res.IsDenBan {
			denBanPlayers = append(denBanPlayers, uid)
		}
	}

	type RawMatch struct {
		PlayerNet int64
		HostNet   int64
		Status    string
	}
	rawMatches := make(map[string]RawMatch)

	for _, uid := range playerIDs {
		if uid == hostID {
			continue
		}
		res := playerResMap[uid]
		var pNet, hNet int64
		var status string

		if hostHasSpecial || res.Rank >= RankNguLinh {
			if hostRes.Rank > res.Rank {
				loss := int64(math.Round(float64(betAmount) * hostRes.Multiplier))
				pNet = -loss
				hNet = loss
				status = "Thua"
			} else if hostRes.Rank < res.Rank {
				win := int64(math.Round(float64(betAmount) * res.Multiplier))
				pNet = win
				hNet = -win
				status = res.Label
			} else {
				pNet = 0
				hNet = 0
				status = "Hòa"
			}
		} else if hostRes.IsDenBan {
			if res.IsDenBan {
				pNet = 0
				hNet = 0
				status = "Hòa (Cùng Đền Bàn)"
			} else if res.IsQuac {
				pNet = 0
				hNet = 0
				status = "Hòa (Cái Đền Bàn, Khách Quắc)"
			} else {
				pNet = betAmount
				hNet = -betAmount
				if hostRes.IsGia {
					status = "Thắng (Cái Già Đền Bàn)"
				} else {
					status = "Thắng (Cái Non Đền Bàn)"
				}
			}
		} else {
			hostValid := hostRes.Rank == RankNormal
			playerValid := res.Rank == RankNormal

			if hostValid && playerValid {
				if res.Points > hostRes.Points {
					pNet = betAmount
					hNet = -betAmount
					status = "Thắng"
				} else if res.Points < hostRes.Points {
					pNet = -betAmount
					hNet = betAmount
					status = "Thua"
				} else {
					pNet = 0
					hNet = 0
					status = "Hòa"
				}
			} else if hostValid && !playerValid {
				pNet = -betAmount
				hNet = betAmount
				if res.IsQuac {
					status = "Quắc"
				} else {
					status = "Thua"
				}
			} else if !hostValid && playerValid {
				pNet = betAmount
				hNet = -betAmount
				if hostRes.IsQuac {
					status = "Thắng (Cái Quắc)"
				} else {
					status = "Thắng"
				}
			} else {
				if hostRes.Points > 21 {
					if res.Points > 21 {
						pNet = 0
						hNet = 0
						status = "Quắc (Hòa)"
						if res.IsGia {
							status = "Hòa"
						}
					} else {
						pNet = betAmount
						hNet = -betAmount
						status = "Thắng (Cái Quắc)"
					}
				} else {
					if res.Points > 21 {
						pNet = -betAmount
						hNet = betAmount
						status = "Quắc"
					} else {
						if res.Points > hostRes.Points {
							pNet = betAmount
							hNet = -betAmount
							status = "Thắng"
						} else if res.Points < hostRes.Points {
							pNet = -betAmount
							hNet = betAmount
							status = "Thua"
						} else {
							pNet = 0
							hNet = 0
							status = "Hòa"
						}
					}
				}
			}
		}
		rawMatches[uid] = RawMatch{PlayerNet: pNet, HostNet: hNet, Status: status}
	}

	if len(denBanPlayers) > 0 {
		var totalBurden int64 = 0
		var hostNetFromNormal int64 = 0

		denBanMap := make(map[string]bool)
		for _, uid := range denBanPlayers {
			denBanMap[uid] = true
		}
		// Sắp xếp UID để việc chia phần dư (remainder) luôn ổn định, không phụ thuộc map
		sort.Strings(denBanPlayers)

		for _, uid := range playerIDs {
			if uid == hostID || denBanMap[uid] {
				continue
			}
			match := rawMatches[uid]
			if match.PlayerNet > 0 {
				totalBurden += match.PlayerNet
				deltas[uid] += match.PlayerNet
				playerResults[uid] = PlayerMatchResult{Status: "Thắng (Được đền)", Net: match.PlayerNet}
			} else if match.PlayerNet < 0 {
				totalBurden += -match.PlayerNet
				hostNetFromNormal += -match.PlayerNet
				deltas[uid] += 0
				playerResults[uid] = PlayerMatchResult{Status: "Thua (Được đền thay)", Net: 0}
			} else {
				deltas[uid] += 0
				playerResults[uid] = PlayerMatchResult{Status: "Hòa", Net: 0}
			}
		}

		burdenShare := totalBurden / int64(len(denBanPlayers))
		remainder := totalBurden % int64(len(denBanPlayers))

		for i, dUid := range denBanPlayers {
			share := burdenShare
			// Phân bổ nốt phần dư lẻ (vài xu) cho những người đứng đầu danh sách
			if int64(i) < remainder {
				share++
			}

			hostGetsBaseBet := hostRes.Rank == RankNormal || hostRes.Rank >= RankNguLinh
			baseLoss := int64(0)
			if hostGetsBaseBet {
				baseLoss = betAmount
				deltas[hostID] += betAmount
			}

			loss := baseLoss + share
			deltas[dUid] -= loss
			playerResults[dUid] = PlayerMatchResult{Status: fmt.Sprintf("ĐỀN BÀN (%d)", playerResMap[dUid].Points), Net: -loss}
		}
		deltas[hostID] += hostNetFromNormal
	} else {
		for _, uid := range playerIDs {
			if uid == hostID {
				continue
			}
			match := rawMatches[uid]
			deltas[uid] += match.PlayerNet
			deltas[hostID] += match.HostNet
			playerResults[uid] = PlayerMatchResult{Status: match.Status, Net: match.PlayerNet}
		}
	}

	return deltas, playerResults
}

type SettlementAction struct {
	UserID    string
	Delta     int64
	Refund    int64
	Shortfall int64
}

func GetSettlementActions(hostID string, playerIDs []string, deltas map[string]int64, betAmount int64, hostLockedAmount int64) []SettlementAction {
	var actions []SettlementAction

	for _, uid := range playerIDs {
		if uid == hostID {
			continue
		}
		delta := deltas[uid]
		refund := betAmount + delta
		var shortfall int64
		if refund < 0 {
			shortfall = -refund
			refund = 0
		}
		actions = append(actions, SettlementAction{
			UserID:    uid,
			Delta:     delta,
			Refund:    refund,
			Shortfall: shortfall,
		})
	}

	hostDelta := deltas[hostID]
	hostRefund := hostLockedAmount + hostDelta
	var hostShortfall int64
	if hostRefund < 0 {
		hostShortfall = -hostRefund
		hostRefund = 0
	}

	actions = append(actions, SettlementAction{
		UserID:    hostID,
		Delta:     hostDelta,
		Refund:    hostRefund,
		Shortfall: hostShortfall,
	})

	return actions
}

// BuildDeck tạo bộ bài 52 lá chuẩn.
func BuildDeck() []Card {
	deck := make([]Card, 0, 52)
	for suit := 0; suit < 4; suit++ {
		for rank := 1; rank <= 13; rank++ {
			deck = append(deck, Card{Suit: Suit(suit), Rank: rank})
		}
	}
	return deck
}

// Trạng thái của từng người chơi
type PlayerState struct {
	UserID   string
	Balance  int64
	Hand     []Card
	Status   string // "playing", "stand", "bust"
	HasStood bool
}

// Bàn chơi Xì Dách nhiều người (PvP)
type BlackjackTable struct {
	Mu               sync.Mutex
	TableID          string
	ChannelID        string
	GuildID          string
	MessageID        string
	ControlID        string
	ActionMsgID      string
	Deadline         time.Time
	StartedAt        time.Time
	HostID           string
	BetAmount        int64
	HostLockedAmount int64
	DurationSec      int
	Players          map[string]*PlayerState
	PlayerIDs        []string // Lưu thứ tự tham gia
	Deck             []Card
	Status           string // "waiting", "playing", "finished"
}

func NewBlackjackTable(channelID, guildID, hostID string, bet int64, durationSec int, hostBalance int64) *BlackjackTable {
	deck := BuildDeck()
	ShuffleDeck(deck)
	t := &BlackjackTable{
		TableID:          fmt.Sprintf("%d-%s", time.Now().UnixNano(), hostID),
		ChannelID:        channelID,
		GuildID:          guildID,
		HostID:           hostID,
		BetAmount:        bet,
		HostLockedAmount: bet, // Base locked amount
		DurationSec:      durationSec,
		Players:          make(map[string]*PlayerState),
		Deck:             deck,
		Status:           "waiting",
	}
	t.AddPlayer(hostID, hostBalance) // Host mặc định tham gia bàn
	return t
}

func (t *BlackjackTable) AddPlayer(userID string, balance int64) bool {
	if len(t.Players) >= 12 {
		return false
	}
	if _, exists := t.Players[userID]; exists {
		return false
	}
	t.Players[userID] = &PlayerState{
		UserID:  userID,
		Balance: balance,
		Status:  "playing",
	}
	t.PlayerIDs = append(t.PlayerIDs, userID)
	return true
}

func (t *BlackjackTable) deal() Card {
	if len(t.Deck) == 0 {
		newDeck := BuildDeck()
		ShuffleDeck(newDeck)
		t.Deck = newDeck
	}
	c := t.Deck[0]
	t.Deck = t.Deck[1:]
	return c
}

func (t *BlackjackTable) StartGame() {
	t.Status = "playing"
	t.StartedAt = time.Now()
	t.Deadline = t.StartedAt.Add(time.Duration(t.DurationSec) * time.Second)
	for i := 0; i < 2; i++ {
		for _, uid := range t.PlayerIDs {
			p := t.Players[uid]
			p.Hand = append(p.Hand, t.deal())
		}
	}
}

func (t *BlackjackTable) Hit(userID string) (Card, string, bool) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	p, exists := t.Players[userID]
	if !exists || t.Status != "playing" {
		return Card{}, "Bàn hoặc người chơi không ở trạng thái hợp lệ.", false
	}
	if p.Status == "bust" {
		return Card{}, "Bạn đã quắc, không thể rút thêm!", false
	}
	if p.HasStood || p.Status == "stand" {
		return Card{}, "Bạn đã dằn bài, không thể thao tác thêm trong ván này.", false
	}
	if len(p.Hand) >= 5 {
		return Card{}, "Bạn đã đủ 5 lá, không thể rút thêm!", false
	}
	c := t.deal()
	p.Hand = append(p.Hand, c)
	if IsBust(p.Hand) {
		p.Status = "bust"
		p.HasStood = true
	} else if len(p.Hand) == 5 {
		p.Status = "stand" // Ngũ linh tự động dằn
		p.HasStood = true
	}
	return c, "", true
}

func (t *BlackjackTable) Stand(userID string, minStand int) (string, bool) {
	t.Mu.Lock()
	defer t.Mu.Unlock()
	p, exists := t.Players[userID]
	if !exists || t.Status != "playing" {
		return "Bàn không ở trạng thái hợp lệ.", false
	}
	if p.HasStood || p.Status == "stand" {
		return "Bạn đã dằn bài, không thể thao tác thêm trong ván này.", false
	}
	if p.Status == "bust" {
		return "Bạn đã quắc, không thể dằn.", false
	}
	if HandValue(p.Hand) < minStand && len(p.Hand) < 5 && !IsXiBan(p.Hand) && !IsXiDach(p.Hand) {
		return fmt.Sprintf("Bài dưới %d (Non) không thể Dằn.", minStand), false
	}

	p.Status = "stand"
	p.HasStood = true
	return "", true
}
