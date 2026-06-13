package games

type TaixiuSide string

const (
	SideTai TaixiuSide = "tai"
	SideXiu TaixiuSide = "xiu"
)

type TaixiuResult struct {
	Dice1, Dice2, Dice3 int
	Sum                 int
	IsTriple            bool
	Winner              TaixiuSide
}

func PlayTaixiu() TaixiuResult {
	d1, d2, d3 := RollDice()
	return newTaixiuResult(d1, d2, d3)
}

func newTaixiuResult(d1, d2, d3 int) TaixiuResult {
	sum := d1 + d2 + d3
	isTriple := d1 == d2 && d2 == d3

	var winner TaixiuSide
	if !isTriple {
		if sum >= 11 {
			winner = SideTai
		} else {
			winner = SideXiu
		}
	}

	return TaixiuResult{
		Dice1:    d1,
		Dice2:    d2,
		Dice3:    d3,
		Sum:      sum,
		IsTriple: isTriple,
		Winner:   winner,
	}
}

func (r TaixiuResult) Label() string {
	if r.IsTriple {
		return "Tam Giống"
	}
	if r.Winner == SideTai {
		return "Tài"
	}
	return "Xỉu"
}

// IsWin kiểm tra xem cửa cược (side) có thắng với kết quả hiện tại không.
func (r TaixiuResult) IsWin(side string) bool {
	if side == "hu" {
		return r.IsTriple
	}
	if r.IsTriple {
		return false // Ra hũ thì các cửa khác đều thua
	}
	switch side {
	case "tai":
		return r.Winner == SideTai
	case "xiu":
		return r.Winner == SideXiu
	case "chan":
		return r.Sum%2 == 0
	case "le":
		return r.Sum%2 != 0
	}
	return false
}
