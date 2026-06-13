package games

import "fmt"

const (
	HandTypePoint = 1
	HandTypeBaCao = 2
	HandTypeSap   = 3
)

type BaicaoRank struct {
	Type  int // 1: Điểm thường, 2: Ba Cào, 3: Sáp
	Value int // Giá trị điểm (0-9) hoặc Giá trị Sáp (2-14)
	Label string
}

// GetBaicaoRank đánh giá sức mạnh của 3 lá bài cào
func GetBaicaoRank(hand []Card) BaicaoRank {
	if len(hand) != 3 {
		return BaicaoRank{}
	}

	// 1. Kiểm tra Sáp
	if hand[0].Rank == hand[1].Rank && hand[1].Rank == hand[2].Rank {
		val := hand[0].Rank
		if val == 1 {
			val = 14 // Trong Bài Cào, Sáp Át là lớn nhất
		}
		rankStrs := []string{"", "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
		return BaicaoRank{Type: HandTypeSap, Value: val, Label: fmt.Sprintf("Sáp %s", rankStrs[hand[0].Rank])}
	}

	// 2. Kiểm tra Ba Cào (Ba Tiên: 3 lá J, Q, K bất kỳ)
	isBaCao := true
	for _, c := range hand {
		if c.Rank < 11 || c.Rank > 13 {
			isBaCao = false
			break
		}
	}
	if isBaCao {
		return BaicaoRank{Type: HandTypeBaCao, Value: 0, Label: "Ba Cào"}
	}

	// 3. Tính điểm thường
	pts := 0
	for _, c := range hand {
		pts += c.Value() // func Value() đã quy định J,Q,K = 10; A = 1
	}
	finalPt := pts % 10
	label := fmt.Sprintf("%d Điểm", finalPt)
	if finalPt == 0 {
		label = "Bù (0 Điểm)"
	}
	return BaicaoRank{Type: HandTypePoint, Value: finalPt, Label: label}
}
