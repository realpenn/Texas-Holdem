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
