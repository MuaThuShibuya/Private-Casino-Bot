package games

// Các biến này được giữ lại như fallback strings để tránh lỗi build khi xóa package assets.
// Giao diện hiển thị thực tế đã được chuyển sang dùng ui.Emojis.

var (
	EmojiCoin     = "🪙"
	EmojiBank     = "🏦"
	EmojiWin      = "🎉"
	EmojiLose     = "💸"
	EmojiTransfer = "🔄"
	EmojiHistory  = "📋"
)

var (
	EmojiSuccess = "✅"
	EmojiError   = "❌"
	EmojiWarning = "⚠️"
	EmojiTimer   = "⏱️"
	EmojiLock    = "🔒"
	EmojiShield  = "🛡️"
	EmojiBlock   = "⛔"
)

var (
	EmojiCrown = "👑"
	EmojiSword = "⚔️"
	EmojiUser  = "👤"
)

var (
	EmojiTai    = "🔴"
	EmojiXiu    = "🔵"
	EmojiTriple = "💀"
	EmojiDice   = "🎲"
)

var DiceFaces = [7]string{
	"",
	"⚀",
	"⚁",
	"⚂",
	"⚃",
	"⚄",
	"⚅",
}

var (
	EmojiCard         = "🃏"
	EmojiBust         = "💥"
	EmojiCardBackside = "🂠"
	EmojiBlackjack    = "🎰"
	EmojiDraw         = "🤝"
	EmojiHit          = "➕"
	EmojiStand        = "✋"
	EmojiDouble       = "💎"
)

var (
	EmojiHeart   = "♥️"
	EmojiDiamond = "♦️"
	EmojiClub    = "♣️"
	EmojiSpade   = "♠️"
)

var SuitEmoji = [4]string{"♥️", "♦️", "♣️", "♠️"}
