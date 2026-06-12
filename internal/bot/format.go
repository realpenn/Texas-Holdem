package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"texas-holdem/internal/blackjack"
	"texas-holdem/internal/poker"
	"texas-holdem/internal/store"
)

func waitingText(game *poker.Game, deadline time.Time) string {
	names := make([]string, 0, len(game.Players))
	for _, p := range game.Players {
		names = append(names, p.Display)
	}
	return fmt.Sprintf("新牌局等待中\n盲注：%d/%d\n买入：%d\n人数：%d/9\n玩家：%s\n自动开局：%s",
		game.SmallBlind, game.BigBlind, game.BuyIn, len(game.Players), strings.Join(names, "、"), deadline.Format("15:04:05"))
}

func gameText(game *poker.Game, deadline time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "阶段：%s\n底池：%d\n公共牌：%s\n", streetLabel(game.Street), game.Pot, boardText(game.Board))
	for _, p := range game.Players {
		marker := " "
		if current := game.CurrentPlayer(); current != nil && current.UserID == p.UserID {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %s：%d（%s，本轮 %d）\n", marker, p.Display, p.Stack, playerStatus(p.Status), p.CurrentBet)
	}
	if current := game.CurrentPlayer(); current != nil {
		toCall := game.ToCall()
		if toCall > 0 {
			fmt.Fprintf(&b, "轮到：%s，需要跟注 %d，截止 %s", current.Display, toCall, deadline.Format("15:04:05"))
		} else {
			fmt.Fprintf(&b, "轮到：%s，可过牌，截止 %s", current.Display, deadline.Format("15:04:05"))
		}
	}
	return b.String()
}

func settlementText(game *poker.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "本手结束\n公共牌：%s\n", boardText(game.Board))
	if len(game.Players) > 0 {
		b.WriteString("玩家手牌：\n")
		for _, p := range game.Players {
			hole := "未发牌"
			if len(p.Hole) > 0 {
				hole = poker.CardsString(p.Hole)
			}
			fmt.Fprintf(&b, "%s：%s（%s）\n", p.Display, hole, playerStatus(p.Status))
		}
	}
	for _, award := range game.Awards {
		name := playerName(game, award.UserID)
		if award.HandCategory != "" {
			fmt.Fprintf(&b, "%s 赢得 %d，服务费 %d，入账 %d（%s）\n", name, award.Gross, award.Fee, award.Net, award.HandCategory)
		} else {
			fmt.Fprintf(&b, "%s 赢得 %d，服务费 %d，入账 %d（%s）\n", name, award.Gross, award.Fee, award.Net, award.Reason)
		}
	}
	return strings.TrimSpace(b.String())
}

func waitingKeyboard(gameID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("加入", "join:"+gameID),
			tgbotapi.NewInlineKeyboardButtonData("开始", "begin:"+gameID),
		),
	)
}

func gameSelectKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("德州扑克", "select:"+store.GameTypeHoldem),
			tgbotapi.NewInlineKeyboardButtonData("21点", "select:"+store.GameTypeBlackjack),
		),
	)
}

func actionKeyboard(game *poker.Game) tgbotapi.InlineKeyboardMarkup {
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("看牌", "hole:"+game.ID),
		},
	}
	current := game.CurrentPlayer()
	if current == nil {
		return tgbotapi.NewInlineKeyboardMarkup(buttons...)
	}
	if game.ToCall() > 0 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("弃牌", "act:"+game.ID+":fold"),
			tgbotapi.NewInlineKeyboardButtonData("跟注", "act:"+game.ID+":call"),
			tgbotapi.NewInlineKeyboardButtonData("最小加注", "act:"+game.ID+":raise"),
			tgbotapi.NewInlineKeyboardButtonData("All-in", "act:"+game.ID+":allin"),
		))
	} else {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("过牌", "act:"+game.ID+":check"),
			tgbotapi.NewInlineKeyboardButtonData("最小加注", "act:"+game.ID+":raise"),
			tgbotapi.NewInlineKeyboardButtonData("All-in", "act:"+game.ID+":allin"),
		))
	}
	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

func blackjackWaitingText(game *blackjack.Game, deadline time.Time) string {
	names := make([]string, 0, len(game.Players))
	for _, p := range game.Players {
		names = append(names, p.Display)
	}
	return fmt.Sprintf("21点等待中\n下注：%d\n人数：%d/7\n玩家：%s\n自动开局：%s",
		game.Bet, len(game.Players), strings.Join(names, "、"), deadline.Format("15:04:05"))
}

func blackjackHandLine(p blackjack.Player, handIdx int) string {
	hand := p.Hands[handIdx]
	name := p.Display
	if len(p.Hands) > 1 {
		name = fmt.Sprintf("%s 手牌%d", p.Display, handIdx+1)
	}
	extra := ""
	if hand.Doubled {
		extra = "，已加倍"
	}
	return fmt.Sprintf("%s：%s（%d点，%s%s）", name, poker.CardsString(hand.Cards), blackjack.HandValue(hand.Cards), blackjackPlayerStatus(hand.Status), extra)
}

func blackjackGameText(game *blackjack.Game, deadline time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "庄家明牌：%s\n", boardText(game.DealerVisibleCards()))
	current := game.CurrentPlayer()
	for _, p := range game.Players {
		for h := range p.Hands {
			marker := " "
			if current != nil && current.UserID == p.UserID && h == game.CurrentHand {
				marker = ">"
			}
			fmt.Fprintf(&b, "%s %s\n", marker, blackjackHandLine(p, h))
		}
	}
	if current != nil {
		fmt.Fprintf(&b, "轮到：%s，截止 %s", current.Display, deadline.Format("15:04:05"))
	}
	return b.String()
}

func blackjackSettlementText(game *blackjack.Game) string {
	var b strings.Builder
	fmt.Fprintf(&b, "21点结束\n庄家：%s（%d点）\n玩家手牌：\n", poker.CardsString(game.Dealer), blackjack.HandValue(game.Dealer))
	for _, p := range game.Players {
		for h := range p.Hands {
			fmt.Fprintf(&b, "%s\n", blackjackHandLine(p, h))
		}
	}
	for _, award := range game.Awards {
		name := blackjackPlayerName(game, award.UserID)
		fmt.Fprintf(&b, "%s：下注 %d，返还 %d，净输赢 %+d（%s）\n", name, award.Bet, award.Payout, award.Net, award.Reason)
	}
	return strings.TrimSpace(b.String())
}

func blackjackActionKeyboard(game *blackjack.Game) tgbotapi.InlineKeyboardMarkup {
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData("看牌", "bjhand:"+game.ID),
		},
	}
	if game.CurrentPlayer() != nil {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("要牌", "bjact:"+game.ID+":hit"),
			tgbotapi.NewInlineKeyboardButtonData("停牌", "bjact:"+game.ID+":stand"),
		}
		if game.CanDoubleCurrent() {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("加倍", "bjact:"+game.ID+":double"))
		}
		if game.CanSplitCurrent() {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData("分牌", "bjact:"+game.ID+":split"))
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(row...))
	}
	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

func displayName(user *tgbotapi.User) string {
	if user == nil {
		return "未知玩家"
	}
	if user.UserName != "" {
		return "@" + user.UserName
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		return strconv.FormatInt(user.ID, 10)
	}
	return name
}

func boardText(cards []poker.Card) string {
	if len(cards) == 0 {
		return "尚无"
	}
	return poker.CardsString(cards)
}

func streetLabel(street string) string {
	switch street {
	case poker.StreetPreflop:
		return "翻牌前"
	case poker.StreetFlop:
		return "翻牌圈"
	case poker.StreetTurn:
		return "转牌圈"
	case poker.StreetRiver:
		return "河牌圈"
	case poker.StreetDone:
		return "结束"
	default:
		return street
	}
}

func playerStatus(status string) string {
	switch status {
	case poker.PlayerWaiting:
		return "等待"
	case poker.PlayerActive:
		return "在局"
	case poker.PlayerFolded:
		return "弃牌"
	case poker.PlayerAllIn:
		return "All-in"
	default:
		return status
	}
}

func playerName(game *poker.Game, userID int64) string {
	for _, p := range game.Players {
		if p.UserID == userID {
			return p.Display
		}
	}
	return strconv.FormatInt(userID, 10)
}

func blackjackPlayerName(game *blackjack.Game, userID int64) string {
	for _, p := range game.Players {
		if p.UserID == userID {
			return p.Display
		}
	}
	return strconv.FormatInt(userID, 10)
}

func blackjackPlayerHandText(game *blackjack.Game, userID int64) (string, bool) {
	for _, p := range game.Players {
		if p.UserID == userID {
			if len(p.Hands) == 0 || len(p.Hands[0].Cards) == 0 {
				return "", false
			}
			parts := make([]string, 0, len(p.Hands))
			for _, hand := range p.Hands {
				parts = append(parts, fmt.Sprintf("%s（%d点）", poker.CardsString(hand.Cards), blackjack.HandValue(hand.Cards)))
			}
			return strings.Join(parts, "；"), true
		}
	}
	return "", false
}

func blackjackActionLabel(kind blackjack.ActionKind) string {
	switch kind {
	case blackjack.ActionHit:
		return "要牌"
	case blackjack.ActionStand:
		return "停牌"
	case blackjack.ActionDouble:
		return "加倍"
	case blackjack.ActionSplit:
		return "分牌"
	default:
		return string(kind)
	}
}

func blackjackPlayerStatus(status string) string {
	switch status {
	case blackjack.PlayerWaiting:
		return "等待"
	case blackjack.PlayerActive:
		return "行动中"
	case blackjack.PlayerStand:
		return "停牌"
	case blackjack.PlayerBust:
		return "爆牌"
	case blackjack.PlayerBlackjack:
		return "Blackjack"
	default:
		return status
	}
}

func activeID(active store.ActiveGame) string {
	if active.Type == store.GameTypeBlackjack && active.Blackjack != nil {
		return active.Blackjack.ID
	}
	if active.Game != nil {
		return active.Game.ID
	}
	return ""
}

func activeChatID(active store.ActiveGame) int64 {
	if active.Type == store.GameTypeBlackjack && active.Blackjack != nil {
		return active.Blackjack.ChatID
	}
	if active.Game != nil {
		return active.Game.ChatID
	}
	return 0
}

func activeCreatorID(active store.ActiveGame) int64 {
	if active.Type == store.GameTypeBlackjack && active.Blackjack != nil {
		return active.Blackjack.CreatorID
	}
	if active.Game != nil {
		return active.Game.CreatorID
	}
	return 0
}

func activeStatus(active store.ActiveGame) string {
	if active.Type == store.GameTypeBlackjack && active.Blackjack != nil {
		return active.Blackjack.Status
	}
	if active.Game != nil {
		return active.Game.Status
	}
	return ""
}

func parseGameType(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	switch strings.ToLower(args[0]) {
	case "holdem", "texas", "poker", "德州", "德州扑克":
		return store.GameTypeHoldem, args[1:]
	case "blackjack", "bj", "21", "21点", "二十一点":
		return store.GameTypeBlackjack, args[1:]
	default:
		if _, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			return store.GameTypeHoldem, args
		}
		return "", args
	}
}

func parsePositiveOr(raw string, fallback int64) int64 {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseIntArg(args []string, idx int) (int64, error) {
	if len(args) <= idx {
		return 0, fmt.Errorf("missing argument")
	}
	v, err := strconv.ParseInt(args[idx], 10, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid integer")
	}
	return v, nil
}
