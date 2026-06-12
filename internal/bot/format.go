package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"texas-holdem/internal/poker"
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
	fmt.Fprintf(&b, "第 %d 手 · 阶段：%s\n底池：%d\n公共牌：%s\n", game.HandNo, streetLabel(game.Street), game.Pot, boardText(game.Board))
	for i, p := range game.Players {
		marker := " "
		if current := game.CurrentPlayer(); current != nil && current.UserID == p.UserID {
			marker = ">"
		}
		button := ""
		if i == game.Dealer {
			button = "（庄）"
		}
		fmt.Fprintf(&b, "%s %s%s：%d（%s，本轮 %d）\n", marker, p.Display, button, p.Stack, playerStatus(p.Status), p.CurrentBet)
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
	b.WriteString("当前筹码：")
	for _, p := range game.Players {
		fmt.Fprintf(&b, "%s %d；", p.Display, p.Stack)
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
			tgbotapi.NewInlineKeyboardButtonData("All-in", "act:"+game.ID+":allin"),
		))
	} else {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("过牌", "act:"+game.ID+":check"),
			tgbotapi.NewInlineKeyboardButtonData("All-in", "act:"+game.ID+":allin"),
		))
	}
	if row := raiseRow(game, current); len(row) > 0 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardRow(row...))
	}
	return tgbotapi.NewInlineKeyboardMarkup(buttons...)
}

// raiseRow 给当前玩家生成加注档位按钮：最小加注、半池、满池。
// 金额是“加注到”的总额，写进回调数据；不低于 all-in 总额的档位省略。
func raiseRow(game *poker.Game, current *poker.Player) []tgbotapi.InlineKeyboardButton {
	toCall := game.ToCall()
	maxTo := current.CurrentBet + current.Stack
	minTo := game.MinRaiseTo()
	if minTo >= maxTo {
		return nil
	}
	row := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("最小加注 %d", minTo), fmt.Sprintf("act:%s:raise:%d", game.ID, minTo)),
	}
	potAfterCall := game.Pot + toCall
	for _, opt := range []struct {
		label string
		extra int64
	}{
		{"半池", potAfterCall / 2},
		{"满池", potAfterCall},
	} {
		target := game.CurrentBet + toCall + opt.extra
		if target <= minTo || target >= maxTo {
			continue
		}
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s %d", opt.label, target),
			fmt.Sprintf("act:%s:raise:%d", game.ID, target)))
	}
	return row
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

func parsePositiveOr(raw string, fallback int64) int64 {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
