package ui

import (
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/discordcasino/v2/internal/games"
)

// UIEmoji định nghĩa cấu trúc lưu trữ toàn bộ các biểu tượng dùng trong giao diện.
type UIEmoji struct {
	// ==========================================
	// 🌐 HỆ THỐNG & UI CHUNG
	// ==========================================
	Header          string
	Tick            string
	Heart           string
	HeartHost       string
	Star            string
	StarPurple      string
	WhiteStar       string
	BlackWingLeft   string
	BlackWingRight  string
	WingLeft        string
	WingRight       string
	Flower          string
	Dice            string
	Gift            string
	Exclaim         string
	Book            string
	Clock           string
	Closed          string
	CardBack        string
	CardBackside    string
	Fire            string
	FireFB          string
	ArrowBlack      string
	BlueStar        string
	ArrowXoan       string
	WhiteEnd        string
	KingPurple      string
	ButterflyPurple string
	WhiteStarCircle string
	LoadingWhite    string
	ArrowWhite      string

	// Trạng thái & Hệ thống
	Success   string
	SuccessFB string
	Error     string
	ErrorFB   string
	Warning   string
	WarningFB string
	Loading   string
	LoadingFB string
	Lock      string
	LockFB    string
	Shield    string
	ShieldFB  string
	Block     string
	BlockFB   string
	CandyPink string
	Hentai    string

	// UI & Menu
	Bank         string
	BankFB       string
	Coin         string
	CoinFB       string
	History      string
	HistoryFB    string
	AdminPanel   string
	AdminPanelFB string
	Give         string
	GiveFB       string
	Take         string
	TakeFB       string
	Trash        string
	TrashFB      string
	Broom        string
	BroomFB      string
	Restart      string
	RestartFB    string
	Help         string
	HelpFB       string
	Settings     string
	SettingsFB   string
	Tag          string
	TagFB        string
	User         string
	UserFB       string
	Crown        string
	CrownFB      string
	Party        string
	PartyFB      string
	Empty        string
	EmptyFB      string
	Dot          string
	DotFB        string
	ListDot      string
	ListDotFB    string
	Money        string
	MoneyFB      string
	OWOBlack     string

	//

	// Rank & Leaderboard
	Rank1 string
	Rank2 string
	Rank3 string

	// Khoảng cách giữa các Animation
	SpaceAnimation string

	// ==========================================
	// 🎲 TÀI XỈU
	// ==========================================
	DiceRolling string
	Dice1       string
	Dice2       string
	Dice3       string
	Dice4       string
	Dice5       string
	Dice6       string

	TXClosed     string
	KingTX       string
	BidTX        string
	PeopleTX     string
	RoundTX      string
	RuleChoiceTX string
	StartWhiteL  string
	Warn         string
	FinishTX     string
	FinishTXFB   string

	TaixiuTai        string
	TaixiuTaiFB      string
	TaixiuXiu        string
	TaixiuXiuFB      string
	TaixiuChan       string
	TaixiuChanFB     string
	TaixiuLe         string
	TaixiuLeFB       string
	TaixiuHu         string
	TaixiuHuFB       string
	TaixiuCancel     string
	TaixiuCancelFB   string
	TaixiuContinue   string
	TaixiuContinueFB string

	// ==========================================
	// 🦀 BẦU CUA
	// ==========================================
	TitleBC     string
	HostBC      string
	BetBC       string
	PlayerBC    string
	TimerBC     string
	RewardBC    string
	StarBC      string
	WarningBC   string
	ListBC      string
	EmptyBC     string
	Line1BC     string
	Line2BC     string
	Line3BC     string
	LineArrowBC string

	AnimalRolling string
	Bau           string
	Cua           string
	Ca            string
	Tom           string
	Nai           string
	Ga            string

	BaucuaBau        string
	BaucuaBauFB      string
	BaucuaCua        string
	BaucuaCuaFB      string
	BaucuaTom        string
	BaucuaTomFB      string
	BaucuaCa         string
	BaucuaCaFB       string
	BaucuaNai        string
	BaucuaNaiFB      string
	BaucuaGa         string
	BaucuaGaFB       string
	BaucuaCancel     string
	BaucuaCancelFB   string
	BaucuaContinue   string
	BaucuaContinueFB string

	// ==========================================
	// 🃏 XÌ DÁCH (BLACKJACK)
	// ==========================================
	CardGame string
	Play     string
	Hand     string

	BlackjackJoin       string
	BlackjackJoinFB     string
	BlackjackStart      string
	BlackjackStartFB    string
	BlackjackCancel     string
	BlackjackCancelFB   string
	BlackjackView       string
	BlackjackViewFB     string
	BlackjackHit        string
	BlackjackHitFB      string
	BlackjackStand      string
	BlackjackStandFB    string
	BlackjackContinue   string
	BlackjackContinueFB string

	// ==========================================
	// 🎴 BÀI CÀO
	// ==========================================
	BaicaoHeart        string
	BaicaoKing         string
	BaicaoBet          string
	BaicaoReq          string
	BaicaoHost         string
	BaicaoPlayers      string
	BaicaoPlayerTitle  string
	BaicaoJoin         string
	BaicaoHiddenCard   string
	BaicaoBlink        string
	BaicaoViewTitle    string
	BaicaoResultTitle  string
	BaicaoPlayer       string
	BaicaoEndgameTitle string

	BaicaoSkull    string
	BaicaoCatRed   string
	BaicaoCatBlack string

	BaicaoJoinBtn       string
	BaicaoJoinBtnFB     string
	BaicaoStartBtn      string
	BaicaoStartBtnFB    string
	BaicaoCancelBtn     string
	BaicaoCancelBtnFB   string
	BaicaoViewBtn       string
	BaicaoViewBtnFB     string
	BaicaoRevealBtn     string
	BaicaoRevealBtnFB   string
	BaicaoShowBtn       string
	BaicaoShowBtnFB     string
	BaicaoFoldBtn       string
	BaicaoFoldBtnFB     string
	BaicaoContinueBtn   string
	BaicaoContinueBtnFB string

	// ==========================================
	// 🗴 OTHER / MẶC ĐỊNH BUTTON EMOJIS
	// ==========================================
	Join        string
	Start       string
	Hit         string
	Stand       string
	View        string
	Tai         string
	Xiu         string
	SuitHeart   string
	SuitDiamond string
	SuitClub    string
	SuitSpade   string
	CashOut     string
}

// Emojis là instance chứa các giá trị biểu tượng hiện tại.
var Emojis = &UIEmoji{
	Header:          "<a:canhtraiden:1503965032080343080><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:txc_line1:1505525862680756254><a:canhphaiden:1503965061604180018>", // Done
	Tick:            "<a:txc_350_tick3:1505510796908560405>",                                                                                                                                                                                                                                                                                                          //Done
	Heart:           "<a:txc_Bunny:1505522926793789620>",                                                                                                                                                                                                                                                                                                              //Done
	HeartHost:       "<a:heart_p:1510295576007409815>",                                                                                                                                                                                                                                                                                                                //Done
	Star:            "<a:txc_sao:1505523752946106440>",
	StarPurple:      "<a:bl_dcr_purple:1510513069242187917>", // Done
	WhiteStar:       "<:white_star:1504493475629105343>",     // Done
	BlackWingLeft:   "<a:canhtraiden:1505514095875457136>",   // Done
	BlackWingRight:  "<a:canhphaiden:1505514116813557853>",   // Done
	WingLeft:        "<:bl_canhtrai:1510295704722210918>",    // Done
	WingRight:       "<:bl_canhphai:1510295827317264395>",    // Done
	Flower:          "<:txc_hoatihon:1505525326380404847>",   // Done
	Dice:            "<:bl_dice:1510511305621241948>",        // Done
	Gift:            "<a:txc_hopqua:1505511360895647825>",    // Done
	Exclaim:         "<:txc_chamthan:1505511146206269531>",   // Done
	Book:            "<a:txc_sach:1505525412003053631>",      // Done
	Clock:           "<a:txc_line1:1505525862680756254>",     // Done
	Closed:          "<:txc_closed:1505525242964213902>",     // Done
	CardBack:        "<a:txc_sao:1505523752946106440>",       // Done
	CardBackside:    "<:emoji_20:1505513381182836767>",       // Done
	Fire:            "<:txc_chatIV:1505511172554883132>",     // Done
	FireFB:          "🔥",
	ArrowBlack:      "<a:muiten:1505512750527545484>",           // Done
	OWOBlack:        "<:owo:1505527080014577744>",               // Done
	BlueStar:        "<a:Bluestar_21g:1505514590308536340>",     // Done
	ArrowXoan:       "<a:bl_white_decor:1513417918476718140>",   // Done
	WhiteEnd:        "<a:kt_white_decor:1508780808872132709>",   // Done
	KingPurple:      "<a:9_tt_vuongmien1:1503697412798283806>",  // Done
	ButterflyPurple: "<a:purplebutterfly:1513419702880305243> ", // Done
	WhiteStarCircle: "<:whitesao:1503806459497025650> ",         // Done
	LoadingWhite:    "<a:bl_loading:1513417773458522244>",       // Done
	ArrowWhite:      "<a:muiten:1505512715999907911>",           // Done
	// Trạng thái & Hệ thống

	Success:   "<a:txc_350_tick3:1505510796908560405>", //Done
	SuccessFB: "✅",
	Error:     "<:txc_chamthan:1505511146206269531>", // Done
	ErrorFB:   "❌",
	Warning:   "<a:txc_chamthan:1505511118456754226>", // Done
	WarningFB: "⚠️",
	Loading:   "<a:txc_sao:1505523752946106440>", //Done
	LoadingFB: "⏳",
	Lock:      "<a:txc_line4:1505527618211156100>", // Done
	LockFB:    "🔒",
	Shield:    "<a:z145:1505511850639495269>", // Done
	ShieldFB:  "🛡️",
	Block:     "<:txc_066:1505510738863718462>", // Done
	BlockFB:   "⛔",
	CandyPink: "<a:txc_keohong:1505528087541190686>", // Done

	Bank:         "<a:txc_blue:1505510948306292797>", // Done
	BankFB:       "🏦",
	Coin:         "<:bl_uwu:1509821793790070875>", // Done
	CoinFB:       "🪙",
	History:      "<a:txc_blue:1505510948306292797>", // Done
	HistoryFB:    "📊",
	AdminPanel:   "<:txc_talkIV:1505511454831411252>", // Done
	AdminPanelFB: "🎛️",
	Give:         "<:txc_054:1505510988785516625>", // Done
	GiveFB:       "💰",
	Take:         "<:txc_054:1505510988785516625>", // Done
	TakeFB:       "💸",
	Trash:        "<a:txc_muiten:1505511636340051988>", // Done
	TrashFB:      "🗑️",
	Broom:        "<a:FLY_f2:1505514519248764978>", // Done
	BroomFB:      "🧹",
	Restart:      "<a:bluefiree:1505514169657589781>", // Done
	RestartFB:    "🔄",
	Help:         "<:rsl_white:1505512434738397284>", // Done
	HelpFB:       "❓",
	Settings:     "<a:blink_blue:1505514215971225633>", // Done
	SettingsFB:   "⚙️",
	Tag:          "<:rsl_white:1505512462521340044>", // Done
	TagFB:        "🏷️",
	User:         "<:txc_penguin:1505511684423553094>", // Done
	UserFB:       "👤",
	Crown:        "<a:tim:1505512062028353689>", // Done
	CrownFB:      "👑",
	Party:        "<a:txc_blue:1505510948306292797>", // Done
	PartyFB:      "🎉",
	Empty:        "<a:txc_guralachong:1505538717090189402>", // Done
	EmptyFB:      "⬛",
	Dot:          "•",
	DotFB:        "•",
	ListDot:      "<a:txc_blue:1505510948306292797>", // Done
	ListDotFB:    "🔹",
	Money:        "<:txc_054:1505510988785516625>", // Done
	MoneyFB:      "💵",
	Hentai:       "<a:hentaivip:1323298884625367091>",

	// Rank & Leaderboard
	Rank1: "<a:first_place_trophy:1505513291655417937>",  // Done
	Rank2: "<a:second_place_trophy:1505512154625999020>", // Done
	Rank3: "<a:third_place_trophy:1505512113160978453>",  // Done

	// Tài Xỉu và Xúc Xắc
	SpaceAnimation: "<a:muiten:1505512776830025859>", // Done

	DiceRolling: "<a:xucxac:1505511943639924939>", // Done
	Dice1:       "<:mat1:1505513014105870416>",    // Done
	Dice2:       "<:mat2:1505512983973855292>",    // Done
	Dice3:       "<:mat3:1505512958414032967>",    // Done
	Dice4:       "<:mat4:1505512930425438329>",    // Done
	Dice5:       "<:mat5:1505512905951547423>",    // Done
	Dice6:       "<:mat6:1505512868869705878>",    // Done

	KingTX:       "<a:KC_blackcrown:1505514481546170518>",      // Done
	TXClosed:     "<a:FLY_f1:1505514541306482738>",             // Done
	BidTX:        "<a:diamondsilver:1505513474438987788>",      // Done
	PeopleTX:     "<a:WingsWhite:1505514417528504412>",         // Done
	RoundTX:      "<a:dm3:1505513448040169574>",                // Done
	RuleChoiceTX: "<:rsl_white:1505512488572158104>",           // Done
	StartWhiteL:  "<:rsl_white:1505512434738397284>",           // Done
	Warn:         "<a:Keai_Sv_GhostBlack:1505530308106391673>", // Done
	FinishTX:     "<a:muiten:1505512776830025859>",             // Done
	FinishTXFB:   "🔙",

	TaixiuTai:        "<a:black_gemheart:1505514285097549824>", // Done
	TaixiuTaiFB:      "🔴",
	TaixiuXiu:        "<a:VermillionHeart:1505514439967772692>", // Done
	TaixiuXiuFB:      "🔵",
	TaixiuChan:       "<a:Keai_Sv_GhostBlack:1505530308106391673>", // Done
	TaixiuChanFB:     "⚪",
	TaixiuLe:         "<a:tim:1505512062028353689>", // Done
	TaixiuLeFB:       "⚫",
	TaixiuHu:         "<a:dotrang:1505513403509248002>", // Done
	TaixiuHuFB:       "🎁",
	TaixiuCancel:     "<a:4419blueheartbroken:1505514673284317265>", // Done
	TaixiuCancelFB:   "🗑️",
	TaixiuContinue:   "<a:muiten:1505512776830025859>", // Done
	TaixiuContinueFB: "▶️",

	// Bầu Cua
	TitleBC:     "<a:blink:1505514241480724547>",                // Done
	HostBC:      "<:crown:1505513511726612521>",                 // Done
	BetBC:       "<a:txc_Bbunny:1505512020471185418>",           // Done
	PlayerBC:    "<a:rsl_sea15:1505512544138170428>",            // Done
	TimerBC:     "<:sweetcloud45:1505512128742949016>",          // Done
	RewardBC:    "<a:txc_Bbunny:1505512020471185418>",           // Done
	StarBC:      "<a:ceri_dc_mfnwhitestar:1505513535705321542>", // Done
	WarningBC:   "<:white_star:1505511965533929654>",            // Done
	ListBC:      "<a:bluediaryDNS:1504460401553707198>",         // Done
	LineArrowBC: "<:rsl_decor:1505512572919746571>",             // Done
	EmptyBC:     "<:white_star:1505511965533929654>",            // Done
	Line1BC:     "<:xx_:1505511923930628277>",                   // Done
	Line2BC:     "<:xx_:1505511908109848667>",                   // Done
	Line3BC:     "<:xx_:1505511885687099422>",                   // Done

	AnimalRolling: "<a:baucua:1505514308472279080>", // Done
	Bau:           "<:bau:1505514328013668367>",     // Done
	Cua:           "<:cua:1505513491551752233>",     // Done
	Ca:            "<:ca:1505526952151486476>",      // Done
	Tom:           "<:tom:1505512038942773370>",     // Done
	Nai:           "<:nai:1505512685339672616>",     // Done
	Ga:            "<:ga:1505513182335074304>",      // Done

	BaucuaBau:        "<:bau:1505514328013668367>", //Done
	BaucuaBauFB:      "🍐",
	BaucuaCua:        "<:cua:1505513491551752233>", // Done
	BaucuaCuaFB:      "🦀",
	BaucuaTom:        "<:tom:1505512038942773370>", // Done
	BaucuaTomFB:      "🦐",
	BaucuaCa:         "<:ca:1505526952151486476>", // Done
	BaucuaCaFB:       "🐟",
	BaucuaNai:        "<:nai:1505512685339672616>", // Done
	BaucuaNaiFB:      "🦌",
	BaucuaGa:         "<:ga:1505513182335074304>", // Done
	BaucuaGaFB:       "🐓",
	BaucuaCancel:     "<a:bluefiree:1505514169657589781>", //Done
	BaucuaCancelFB:   "🗑️",
	BaucuaContinue:   "<a:txc_Bbunny:1505512020471185418>", // Done
	BaucuaContinueFB: "▶️",

	// Xì Dách (Blackjack)

	CardGame: "<:txc_tt:1505511771358625833>",         // Done
	Play:     "<a:txc_tim1:1505511535471100065>",      // Done
	Hand:     "<a:txc_DuckDance:1505510874620625109>", // Done

	BlackjackJoin:       "<:bl_book:1514556585211465811>", //Done
	BlackjackJoinFB:     "🤝",
	BlackjackStart:      "<a:bl_bttrfl:1514557673981481144>", // Done
	BlackjackStartFB:    "▶️",
	BlackjackCancel:     "<a:bl_cherry:1514556875792846870>", // Done
	BlackjackCancelFB:   "🗑️",
	BlackjackView:       "<:bl_book:1514556585211465811>", // Done
	BlackjackViewFB:     "👀",
	BlackjackHit:        "<a:purplebutterfly:1513419702880305243>", // Done
	BlackjackHitFB:      "➕",
	BlackjackStand:      "<a:blink:1514558176425676954>", // Done
	BlackjackStandFB:    "✋",
	BlackjackContinue:   "<a:txc_DuckDance:1505510874620625109>", // Done
	BlackjackContinueFB: "▶️",

	// Bài Cào

	BaicaoHeart:        "<a:KC_blackcrown:1505514481546170518>",      // Done
	BaicaoKing:         "<a:ad_X_3crown5:1505514356132020354>",       // Done
	BaicaoBet:          "<a:z145:1505511850639495269>",               // Done
	BaicaoReq:          "<:white_star:1505511965533929654>",          // Done
	BaicaoHost:         "<a:6417redcrown:1505514635694968852>",       // Done
	BaicaoPlayers:      "<a:txc_tim:1505511480676581498>",            // Done
	BaicaoPlayerTitle:  "<a:black_gemheart:1505514285097549824>",     // Done
	BaicaoJoin:         "<a:blink:1505514241480724547>",              // Done
	BaicaoHiddenCard:   "<:emoji_20:1505513381182836767>",            // Done
	BaicaoBlink:        "<a:blink:1505514241480724547>",              // Done
	BaicaoViewTitle:    "<a:kitty_1_rheart:1505513153860079728>",     // Done
	BaicaoResultTitle:  "<a:Keai_Sv_GhostBlack:1505530308106391673>", // Done
	BaicaoPlayer:       "<a:VermillionHeart:1505514439967772692>",    // Done
	BaicaoEndgameTitle: "<a:tim:1505512062028353689>",                // Done

	BaicaoSkull:    "<a:631562skull:1505514656976863403>", // Done
	BaicaoCatRed:   "<a:meodo:1505512810359033928>",       // Done
	BaicaoCatBlack: "<a:meoden:1505512837764616285>",      // Done

	BaicaoJoinBtn:       "<a:9_tt_vuongmien1:1505514615209984070>", // Done
	BaicaoJoinBtnFB:     "🤝",
	BaicaoStartBtn:      "<a:dotrang:1505513403509248002>", // Done
	BaicaoStartBtnFB:    "▶️",
	BaicaoCancelBtn:     "<:rsl_white:1505512434738397284>", // Done
	BaicaoCancelBtnFB:   "🗑️",
	BaicaoViewBtn:       "<a:Yb_dia6:1505514376860401695>", // Done
	BaicaoViewBtnFB:     "👀",
	BaicaoRevealBtn:     "<:thuoc:1505512096014667947>", // Done
	BaicaoRevealBtnFB:   "🤲",
	BaicaoShowBtn:       "<a:Keai_Sv_GhostBlack:1505530308106391673>", // Done
	BaicaoShowBtnFB:     "📢",
	BaicaoFoldBtn:       "<a:txc_tim:1505511480676581498>", // Done
	BaicaoFoldBtnFB:     "🏳️",
	BaicaoContinueBtn:   "<a:z145:1505511850639495269>", // Done
	BaicaoContinueBtnFB: "▶️",

	// Nút Mặc Định

	Join:        "<a:txc_gautrang:1505511275638161680>",  // Done
	Start:       "<a:txc_350_tick3:1505510796908560405>", // Done
	Hit:         "<a:txc_muiten:1505511417397383298>",    // Done
	Stand:       "<:txc_350_tick:1505510849606062152>",   // Done
	View:        "<a:txc_muiten:1505511417397383298>",    // Done
	Tai:         "🔴",
	Xiu:         "🔵",
	SuitHeart:   "♥️",
	SuitDiamond: "♦️",
	SuitClub:    "♣️",
	SuitSpade:   "♠️",
	CashOut:     "<:txc_054:1505510988785516625>", // Done
}

// DiceFace trả về emoji mặt xúc xắc tương ứng.
func (e *UIEmoji) DiceFace(n int) string {
	switch n {
	case 1:
		return e.Dice1
	case 2:
		return e.Dice2
	case 3:
		return e.Dice3
	case 4:
		return e.Dice4
	case 5:
		return e.Dice5
	case 6:
		return e.Dice6
	}
	return e.Dice
}

// ParseEmoji giúp chuyển đổi chuỗi emoji an toàn (hỗ trợ cả custom emoji động/tĩnh) để gán cho Nút Bấm.
// Nếu bot không có quyền dùng custom emoji này, nó sẽ tự động thay thế bằng fallback (emoji mặc định).
func ParseEmoji(s *discordgo.Session, emojiStr string, fallback string) *discordgo.ComponentEmoji {
	// Dọn dẹp tất cả các khoảng trắng vô tình bị chèn vào để Discord không bắt lỗi 50035
	emojiStr = strings.TrimSpace(emojiStr)
	fallback = strings.TrimSpace(fallback)
	if emojiStr == "" && fallback == "" {
		return nil
	}
	if emojiStr == "" && fallback != "" {
		return &discordgo.ComponentEmoji{Name: fallback}
	}
	if strings.HasPrefix(emojiStr, "<") && strings.HasSuffix(emojiStr, ">") {
		// Loại bỏ 2 dấu ngoặc nhọn < >
		core := emojiStr[1 : len(emojiStr)-1]
		parts := strings.Split(core, ":")

		if len(parts) == 3 {
			isAnimated := parts[0] == "a"
			name := parts[1]
			id := parts[2]

			// Kiểm tra xem bot có tồn tại emoji này không
			hasAccess := false
			if s != nil && s.State != nil {
				s.State.RLock()
				for _, g := range s.State.Guilds {
					for _, e := range g.Emojis {
						if e.ID == id {
							hasAccess = true
							break
						}
					}
					if hasAccess {
						break
					}
				}
				s.State.RUnlock()
			}

			if hasAccess {
				return &discordgo.ComponentEmoji{Animated: isAnimated, Name: name, ID: id}
			}
			// Nếu không có quyền, dùng fallback mặc định
			if fallback != "" {
				return &discordgo.ComponentEmoji{Name: fallback}
			}
			return nil
		}
	}
	return &discordgo.ComponentEmoji{Name: emojiStr}
}

// ─── ĐỊNH DẠNG SỐ TIỀN ───────────────────────────────────────────────────────

// FormatNumber định dạng số nguyên với dấu chấm: 1234567 → "1.234.567".
func FormatNumber(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return sign + s
	}
	var sb strings.Builder
	sb.WriteString(sign)
	mod := len(s) % 3
	for i, ch := range s {
		if i > 0 && (i-mod)%3 == 0 && mod != 0 || (i > 0 && mod == 0 && i%3 == 0) {
			sb.WriteRune('.')
		}
		sb.WriteRune(ch)
	}
	return sb.String()
}

// ─── QUẢN LÝ BÀI TÂY (CARDS EMOJI) ───────────────────────────────────────────

func cardKey(suit, rank int) int { return suit*100 + rank }

var cardEmojiMap = map[int]string{
	// ─── ♥ CƠ (Heart) ───
	cardKey(int(games.SuitHeart), 1):  "<:atco:1505516797988835420>",
	cardKey(int(games.SuitHeart), 2):  "<:co:1505517001010057296>",
	cardKey(int(games.SuitHeart), 3):  "<:Beige_Pink_Creative_Illustration:1505508785085485167>",
	cardKey(int(games.SuitHeart), 4):  "<:Beige_Pink_Creative_Illustration:1505508901209243770>",
	cardKey(int(games.SuitHeart), 5):  "<:Beige_Pink_Creative_Illustration:1505508996570677353>",
	cardKey(int(games.SuitHeart), 6):  "<:Beige_Pink_Creative_Illustration:1505509068679413760>",
	cardKey(int(games.SuitHeart), 7):  "<:Beige_Pink_Creative_Illustration:1505509156482711593>",
	cardKey(int(games.SuitHeart), 8):  "<:Beige_Pink_Creative_Illustration:1505509232017936444>",
	cardKey(int(games.SuitHeart), 9):  "<:Beige_Pink_Creative_Illustration:1505509320534786048>",
	cardKey(int(games.SuitHeart), 10): "<:Beige_Pink_Creative_Illustration:1505509399785902080>",
	cardKey(int(games.SuitHeart), 11): "<:Beige_Pink_Creative_Illustration:1505509499409006613>",
	cardKey(int(games.SuitHeart), 12): "<:qb:1505510246758748311>",
	cardKey(int(games.SuitHeart), 13): "<:kc:1505509683413254285>",

	// ─── ♦ RÔ (Diamond) ───
	cardKey(int(games.SuitDiamond), 1):  "<:atro:1505516820994592768>", // Át Rô
	cardKey(int(games.SuitDiamond), 2):  "<:ro:1505517038314197075>",   // 2 Rô
	cardKey(int(games.SuitDiamond), 3):  "<:Beige_Pink_Creative_Illustration:1505508809978675242>",
	cardKey(int(games.SuitDiamond), 4):  "<:Beige_Pink_Creative_Illustration:1505508924089045032>",
	cardKey(int(games.SuitDiamond), 5):  "<:Beige_Pink_Creative_Illustration:1505509014568439868>",
	cardKey(int(games.SuitDiamond), 6):  "<:Beige_Pink_Creative_Illustration:1505509088551763999>",
	cardKey(int(games.SuitDiamond), 7):  "<:Beige_Pink_Creative_Illustration:1505509171045601420>",
	cardKey(int(games.SuitDiamond), 8):  "<:Beige_Pink_Creative_Illustration:1505509254050877531>",
	cardKey(int(games.SuitDiamond), 9):  "<:Beige_Pink_Creative_Illustration:1505509339429863505>",
	cardKey(int(games.SuitDiamond), 10): "<:Beige_Pink_Creative_Illustration:1505509419662708886>",
	cardKey(int(games.SuitDiamond), 11): "<:Beige_Pink_Creative_Illustration:1505509542203756676>",
	cardKey(int(games.SuitDiamond), 12): "<:qr:1505510202168971264>",
	cardKey(int(games.SuitDiamond), 13): "<:kr:1505509706259501108>",

	// ─── ♣ NHÉP (Club) ───
	cardKey(int(games.SuitClub), 1):  "<:attep:1505516845338460221>",
	cardKey(int(games.SuitClub), 2):  "<:tep:1505517089753137153>",
	cardKey(int(games.SuitClub), 3):  "<:Beige_Pink_Creative_Illustration:1505508831998906379>",
	cardKey(int(games.SuitClub), 4):  "<:Beige_Pink_Creative_Illustration:1505508946457395371>",
	cardKey(int(games.SuitClub), 5):  "<:Beige_Pink_Creative_Illustration:1505509033476362421>",
	cardKey(int(games.SuitClub), 6):  "<:Beige_Pink_Creative_Illustration:1505509114548195348>",
	cardKey(int(games.SuitClub), 7):  "<:Beige_Pink_Creative_Illustration:1505509190188404837>",
	cardKey(int(games.SuitClub), 8):  "<:Beige_Pink_Creative_Illustration:1505509278239166524>",
	cardKey(int(games.SuitClub), 9):  "<:Beige_Pink_Creative_Illustration:1505509366973857892>",
	cardKey(int(games.SuitClub), 10): "<:Beige_Pink_Creative_Illustration:1505509434141704264>",
	cardKey(int(games.SuitClub), 11): "<:Beige_Pink_Creative_Illustration:1505509559794532354>",
	cardKey(int(games.SuitClub), 12): "<:qchuon:1505510154769006715>",
	cardKey(int(games.SuitClub), 13): "<:kchuon:1505510022531121264>",

	// ─── ♠ PÍCH (Spade) ───
	cardKey(int(games.SuitSpade), 1):  "<:atbich:1505516874513776692>",
	cardKey(int(games.SuitSpade), 2):  "<:bich:1505517118324740210>",
	cardKey(int(games.SuitSpade), 3):  "<:Beige_Pink_Creative_Illustration:1505508859836633108>",
	cardKey(int(games.SuitSpade), 4):  "<:Beige_Pink_Creative_Illustration:1505508968670298142>",
	cardKey(int(games.SuitSpade), 5):  "<:Beige_Pink_Creative_Illustration:1505509049397940344>",
	cardKey(int(games.SuitSpade), 6):  "<:Beige_Pink_Creative_Illustration:1505509132315267224>",
	cardKey(int(games.SuitSpade), 7):  "<:Beige_Pink_Creative_Illustration:1505509206579613717>",
	cardKey(int(games.SuitSpade), 8):  "<:Beige_Pink_Creative_Illustration:1505509297302536262>",
	cardKey(int(games.SuitSpade), 9):  "<:Beige_Pink_Creative_Illustration:1505509389916967092>",
	cardKey(int(games.SuitSpade), 10): "<:Beige_Pink_Creative_Illustration:1505509475136700446>",
	cardKey(int(games.SuitSpade), 11): "<:jb:1505523888736440330>",
	cardKey(int(games.SuitSpade), 12): "<:qb:1505510113920811168>",
	cardKey(int(games.SuitSpade), 13): "<:kb:1505509755119206410>",
}

var rankNames = [14]string{"", "A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
var suitSymbols = [4]string{"♥", "♦", "♣", "♠"}

// CardEmoji trả về emoji Discord cho lá bài.
func CardEmoji(suit, rank int) string {
	if e := cardEmojiMap[cardKey(suit, rank)]; e != "" {
		return e
	}
	if rank < 1 || rank > 13 || suit < 0 || suit > 3 {
		return "??"
	}
	return rankNames[rank] + suitSymbols[suit]
}

// CardString hiển thị lá bài.
func CardString(c games.Card) string {
	return CardEmoji(int(c.Suit), c.Rank)
}

// HandString ghép các lá bài thành chuỗi hiển thị.
func HandString(hand []games.Card) string {
	var s strings.Builder
	for _, c := range hand {
		s.WriteString(CardString(c))
		s.WriteByte(' ')
	}
	return strings.TrimSpace(s.String())
}
