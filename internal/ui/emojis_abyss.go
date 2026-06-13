package ui

import "strings"

type AIEmojiSet struct {
	Block   string
	Butler  string
	Cards   string
	Dice    string
	Wine    string
	Moon    string
	Sparkle string
	Gem     string
	Candle  string
	Coin    string
	Spade   string
	Heart   string
	Diamond string
	Club    string
	Galaxy  string
	Crystal string
	Eye     string
	Warning string
	Success string
	Error   string
	Loading string
}

var AIEmojis = AIEmojiSet{
	Block:   "<:thuoc:1503965001101086830>",
	Butler:  "<:rsl_white:1503806459497025650>",
	Cards:   "<a:z145:1503772149792509992>",
	Dice:    "<a:dotrang:1503962547538366636>",
	Wine:    "<a:Keai_Sv_GhostBlack:1503716107784163368>",
	Moon:    "<:rsl_white:1503806459497025650>",
	Sparkle: "<a:blink:1503824287042244821>",
	Gem:     "<a:dm3:1503783620580474920>",
	Candle:  "<a:tim:1503767770620493896>",
	Coin:    "<:owo:1503810343980372169>",
	Spade:   "<a:diamondsilver:1503791293728297043>",
	Heart:   "<a:VermillionHeart:1503768808622526654>",
	Diamond: "<a:phale:1503962615435493498>",
	Club:    "<a:txc_DuckDance:1312845520946860033>",
	Galaxy:  "<a:blink_blue:1503770405675077733>",
	Crystal: "<a:muiten:1503999272516063254>",
	Eye:     "<:rsl_white:1503806331507970049>",
	Warning: "<:txc_cat_ban:1341383954535551047>",
	Success: "<a:muiten:1503999272516063254>",
	Error:   "<:txc_066:1357374926016872498>",
	Loading: "<a:txc_cat:1312700579067334676>",
}

// ReplaceEmojiTokens duyệt và map toàn bộ Token dạng {Name} thành Emoji thật
func ReplaceEmojiTokens(text string) string {
	replacer := strings.NewReplacer(
		"{Block}", AIEmojis.Block, "{Butler}", AIEmojis.Butler, "{Cards}", AIEmojis.Cards,
		"{Dice}", AIEmojis.Dice, "{Wine}", AIEmojis.Wine, "{Moon}", AIEmojis.Moon,
		"{Sparkle}", AIEmojis.Sparkle, "{Gem}", AIEmojis.Gem, "{Candle}", AIEmojis.Candle,
		"{Coin}", AIEmojis.Coin, "{Spade}", AIEmojis.Spade, "{Heart}", AIEmojis.Heart,
		"{Diamond}", AIEmojis.Diamond, "{Club}", AIEmojis.Club, "{Galaxy}", AIEmojis.Galaxy,
		"{Crystal}", AIEmojis.Crystal, "{Eye}", AIEmojis.Eye, "{Warning}", AIEmojis.Warning,
		"{Success}", AIEmojis.Success, "{Error}", AIEmojis.Error, "{Loading}", AIEmojis.Loading,
	)
	return replacer.Replace(text)
}
