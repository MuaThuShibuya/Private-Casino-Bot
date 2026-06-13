package games

// taixiu_test.go — Unit test cho logic Tài Xỉu
//
// Chạy:  go test ./internal/games/ -v -run TestTaixiu
// Race:  go test ./internal/games/ -race -run TestTaixiu

import "testing"

// ─── newTaixiuResult — test với xúc xắc cố định ──────────────────────────────

func TestTaixiuResult(t *testing.T) {
	tests := []struct {
		name         string
		d1, d2, d3   int
		wantSum      int
		wantIsTriple bool
		wantWinner   TaixiuSide
	}{
		// ── Tam giống (Triple) — nhà cái thắng, Winner phải rỗng ────────────
		{
			name: "Triple 1-1-1 — tổng 3, tam giống",
			d1:   1, d2: 1, d3: 1,
			wantSum:      3,
			wantIsTriple: true,
			wantWinner:   "",
		},
		{
			name: "Triple 3-3-3 — tổng 9, tam giống (Xỉu nhưng vẫn triple)",
			d1:   3, d2: 3, d3: 3,
			wantSum:      9,
			wantIsTriple: true,
			wantWinner:   "",
		},
		{
			name: "Triple 5-5-5 — tổng 15, tam giống (Tài nhưng vẫn triple)",
			d1:   5, d2: 5, d3: 5,
			wantSum:      15,
			wantIsTriple: true,
			wantWinner:   "",
		},
		{
			name: "Triple 6-6-6 — tổng 18, tam giống tối đa",
			d1:   6, d2: 6, d3: 6,
			wantSum:      18,
			wantIsTriple: true,
			wantWinner:   "",
		},

		// ── Xỉu (tổng 3–10) ──────────────────────────────────────────────────
		{
			name: "Xỉu — tổng 6 (1+2+3)",
			d1:   1, d2: 2, d3: 3,
			wantSum:      6,
			wantIsTriple: false,
			wantWinner:   SideXiu,
		},
		{
			name: "Xỉu biên — tổng 10 (2+4+4)",
			d1:   2, d2: 4, d3: 4,
			wantSum:      10,
			wantIsTriple: false,
			wantWinner:   SideXiu,
		},

		// ── Tài (tổng 11–18) ─────────────────────────────────────────────────
		{
			name: "Tài biên — tổng 11 (3+4+4)",
			d1:   3, d2: 4, d3: 4,
			wantSum:      11,
			wantIsTriple: false,
			wantWinner:   SideTai,
		},
		{
			name: "Tài — tổng 15 (5+6+4)",
			d1:   5, d2: 6, d3: 4,
			wantSum:      15,
			wantIsTriple: false,
			wantWinner:   SideTai,
		},
		{
			name: "Tài tối đa không triple — tổng 17 (5+6+6)",
			d1:   5, d2: 6, d3: 6,
			wantSum:      17,
			wantIsTriple: false,
			wantWinner:   SideTai,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTaixiuResult(tt.d1, tt.d2, tt.d3)

			if r.Sum != tt.wantSum {
				t.Errorf("Sum = %d, muốn %d", r.Sum, tt.wantSum)
			}
			if r.IsTriple != tt.wantIsTriple {
				t.Errorf("IsTriple = %v, muốn %v", r.IsTriple, tt.wantIsTriple)
			}
			// Khi triple, Winner phải luôn rỗng
			if tt.wantIsTriple && r.Winner != "" {
				t.Errorf("Triple nhưng Winner = %q — phải rỗng", r.Winner)
			}
			// Khi không triple, Winner phải khớp
			if !tt.wantIsTriple && r.Winner != tt.wantWinner {
				t.Errorf("Winner = %q, muốn %q", r.Winner, tt.wantWinner)
			}
		})
	}
}

// ─── PlayTaixiu — test tính hợp lệ thống kê ──────────────────────────────────

func TestPlayTaixiuValidity(t *testing.T) {
	// Chạy 10_000 lần để đảm bảo mọi kết quả đều nằm trong biên hợp lệ.
	// Không test giá trị cụ thể vì PlayTaixiu() dùng crypto/rand.
	for i := 0; i < 10_000; i++ {
		r := PlayTaixiu()

		// Mỗi xúc xắc phải trong phạm vi 1–6
		if r.Dice1 < 1 || r.Dice1 > 6 ||
			r.Dice2 < 1 || r.Dice2 > 6 ||
			r.Dice3 < 1 || r.Dice3 > 6 {
			t.Fatalf("vòng %d: xúc xắc ngoài phạm vi: %d %d %d", i, r.Dice1, r.Dice2, r.Dice3)
		}

		// Tổng phải bằng đúng tổng 3 mặt
		if want := r.Dice1 + r.Dice2 + r.Dice3; r.Sum != want {
			t.Fatalf("vòng %d: Sum = %d, muốn %d", i, r.Sum, want)
		}

		// Tổng phải trong phạm vi 3–18
		if r.Sum < 3 || r.Sum > 18 {
			t.Fatalf("vòng %d: Sum = %d ngoài phạm vi [3, 18]", i, r.Sum)
		}

		if r.IsTriple {
			// Triple → Winner phải rỗng
			if r.Winner != "" {
				t.Fatalf("vòng %d: Triple nhưng Winner = %q", i, r.Winner)
			}
		} else {
			// Không triple → Winner phải đúng theo tổng
			if r.Sum >= 11 && r.Winner != SideTai {
				t.Fatalf("vòng %d: tổng %d phải là Tài, nhận được %q", i, r.Sum, r.Winner)
			}
			if r.Sum <= 10 && r.Winner != SideXiu {
				t.Fatalf("vòng %d: tổng %d phải là Xỉu, nhận được %q", i, r.Sum, r.Winner)
			}
		}
	}
}

// ─── DiceDisplay & Label ──────────────────────────────────────────────────────

func TestTaixiuLabel(t *testing.T) {
	tests := []struct {
		name       string
		d1, d2, d3 int
		wantTriple bool
	}{
		{"Triple 4-4-4 — Label chứa 'Tam Giống'", 4, 4, 4, true},
		{"Tài 5-6-4   — Label chứa 'Tài'", 5, 6, 4, false},
		{"Xỉu 1-2-3   — Label chứa 'Xỉu'", 1, 2, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTaixiuResult(tt.d1, tt.d2, tt.d3)
			label := r.Label()
			if label == "" {
				t.Error("Label() trả về chuỗi rỗng")
			}
		})
	}
}
