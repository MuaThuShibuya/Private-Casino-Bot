package games

import (
	"crypto/rand"
	"math/big"
)

// RandInt trả về số nguyên ngẫu nhiên bảo mật trong khoảng [0, n).
// Dùng crypto/rand thay vì math/rand để kết quả không thể đoán trước hay can thiệp.
func RandInt(n int64) int64 {
	if n <= 0 {
		return 0
	}
	max := big.NewInt(n)
	result, err := rand.Int(rand.Reader, max)
	if err != nil {
		// crypto/rand thất bại là lỗi hệ thống nghiêm trọng — không thể tiếp tục
		panic("crypto/rand không khả dụng: " + err.Error())
	}
	return result.Int64()
}

// RollDice tung 3 xúc xắc độc lập, mỗi cái trả về giá trị trong [1, 6].
func RollDice() (int, int, int) {
	return int(RandInt(6)) + 1, int(RandInt(6)) + 1, int(RandInt(6)) + 1
}

// ShuffleDeck xáo trộn bộ bài bằng thuật toán Fisher-Yates với crypto/rand.
// Dùng generic [T any] để tái sử dụng cho mọi kiểu slice.
func ShuffleDeck[T any](deck []T) {
	n := int64(len(deck))
	for i := n - 1; i > 0; i-- {
		j := RandInt(i + 1)
		deck[i], deck[j] = deck[j], deck[i]
	}
}
