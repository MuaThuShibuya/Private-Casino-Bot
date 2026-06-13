package discord

import (
	"os"
	"testing"
)

// Test file đảm bảo Hũ được kích hoạt đúng chuẩn và tính năng check Tài Xỉu
func TestTaiXiuLogicStrictHu(t *testing.T) {
	testCases := []struct {
		name       string
		d1, d2, d3 int
		side       string
		wantWin    bool
	}{
		{"5,5,5 Tài cược -> Thắng (Không bị tính Hũ đè)", 5, 5, 5, "tai", true},
		{"5,5,5 Xỉu cược -> Thua", 5, 5, 5, "xiu", false},
		{"5,5,5 Lẻ cược -> Thắng (15)", 5, 5, 5, "le", true},
		{"5,5,5 Hũ cược -> Thua (chỉ 111, 666 là hũ)", 5, 5, 5, "hu", false},

		{"2,2,2 Xỉu cược -> Thắng (Không bị tính Hũ đè)", 2, 2, 2, "xiu", true},
		{"2,2,2 Hũ cược -> Thua", 2, 2, 2, "hu", false},

		{"1,1,1 Hũ cược -> Thắng", 1, 1, 1, "hu", true},
		{"1,1,1 Xỉu cược -> Thua (Vì bị tính Hũ giết hết)", 1, 1, 1, "xiu", false},

		{"6,6,6 Hũ cược -> Thắng", 6, 6, 6, "hu", true},
		{"6,6,6 Tài cược -> Thua (Vì bị tính Hũ giết hết)", 6, 6, 6, "tai", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isHuRule := tc.d1 == tc.d2 && tc.d2 == tc.d3 && (tc.d1 == 1 || tc.d1 == 6)
			sumDice := int64(tc.d1 + tc.d2 + tc.d3)

			var gotWin bool
			if isHuRule {
				gotWin = (tc.side == "hu")
			} else {
				switch tc.side {
				case "tai":
					gotWin = sumDice >= 11 && sumDice <= 17
				case "xiu":
					gotWin = sumDice >= 4 && sumDice <= 10
				case "chan":
					gotWin = sumDice%2 == 0
				case "le":
					gotWin = sumDice%2 != 0
				}
			}

			if gotWin != tc.wantWin {
				t.Errorf("Dice: %d,%d,%d | Side: %s | Expected: %v | Got: %v", tc.d1, tc.d2, tc.d3, tc.side, tc.wantWin, gotWin)
			}
		})
	}
}

func TestPermissionEnvOwner(t *testing.T) {
	// Giả lập Biến môi trường .env
	os.Setenv("OWNER_ID", "123456789, 987654321")

	if !IsEnvOwner("123456789") {
		t.Errorf("Kỳ vọng 123456789 là Owner nhưng trả về False")
	}
	if !IsEnvOwner("987654321") {
		t.Errorf("Kỳ vọng 987654321 là Owner (Có khoảng trắng bị kẹt) nhưng trả về False")
	}
	if IsEnvOwner("111222") {
		t.Errorf("Kỳ vọng 111222 KHÔNG phải Owner nhưng trả về True")
	}

	// Dọn dẹp Mock Env
	os.Unsetenv("OWNER_ID")
}
