package games

type BaucuaSide string

const (
	SideBau BaucuaSide = "bau"
	SideCua BaucuaSide = "cua"
	SideTom BaucuaSide = "tom"
	SideCa  BaucuaSide = "ca"
	SideNai BaucuaSide = "nai"
	SideGa  BaucuaSide = "ga"
)

var BaucuaFaces = []BaucuaSide{SideBau, SideCua, SideTom, SideCa, SideNai, SideGa}

type BaucuaResult struct {
	Face1 BaucuaSide
	Face2 BaucuaSide
	Face3 BaucuaSide
}

func PlayBaucua() BaucuaResult {
	return BaucuaResult{
		Face1: BaucuaFaces[RandInt(6)],
		Face2: BaucuaFaces[RandInt(6)],
		Face3: BaucuaFaces[RandInt(6)],
	}
}

// MatchCount trả về số lần một mặt xuất hiện trong kết quả (0, 1, 2, 3)
func (r BaucuaResult) MatchCount(side string) int {
	count := 0
	if string(r.Face1) == side {
		count++
	}
	if string(r.Face2) == side {
		count++
	}
	if string(r.Face3) == side {
		count++
	}
	return count
}
