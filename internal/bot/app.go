package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"

	"texas-holdem/internal/blackjack"
	"texas-holdem/internal/config"
	"texas-holdem/internal/poker"
	"texas-holdem/internal/store"
)

type App struct {
	api   *tgbotapi.BotAPI
	cfg   config.Config
	store *store.Store
	log   *slog.Logger

	mu       sync.Mutex
	chatLock map[int64]*sync.Mutex
}

func New(api *tgbotapi.BotAPI, cfg config.Config, st *store.Store, logger *slog.Logger) *App {
	return &App{api: api, cfg: cfg, store: st, log: logger, chatLock: map[int64]*sync.Mutex{}}
}

func (a *App) Run(ctx context.Context) error {
	a.setCommands()
	go a.deadlineLoop(ctx)

	updateCfg := tgbotapi.NewUpdate(0)
	updateCfg.Timeout = 30
	updates := a.api.GetUpdatesChan(updateCfg)
	for {
		select {
		case <-ctx.Done():
			a.api.StopReceivingUpdates()
			return ctx.Err()
		case update := <-updates:
			if update.UpdateID == 0 && update.Message == nil && update.CallbackQuery == nil {
				continue
			}
			go a.handleUpdate(ctx, update)
		}
	}
}

func (a *App) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			a.log.Error("panic handling update", "panic", r)
		}
	}()
	if update.Message != nil {
		a.handleMessage(ctx, update.Message)
		return
	}
	if update.CallbackQuery != nil {
		a.handleCallback(ctx, update.CallbackQuery)
	}
}

func (a *App) handleMessage(ctx context.Context, m *tgbotapi.Message) {
	if m.From == nil || m.Chat == nil {
		return
	}
	_ = a.store.UpsertUser(ctx, m.From.ID, m.From.UserName, m.From.FirstName, m.From.LastName)
	if m.Chat.IsGroup() || m.Chat.IsSuperGroup() {
		_ = a.store.UpsertGroup(ctx, m.Chat.ID, m.Chat.Title)
		if !a.cfg.IsAllowedChat(m.Chat.ID) {
			a.reply(m.Chat.ID, m.MessageID, "本群未在 ALLOWED_CHAT_IDS 中配置，bot 不会在这里开局。")
			return
		}
	}
	if !m.IsCommand() {
		return
	}
	cmd := strings.ToLower(m.Command())
	args := strings.Fields(m.CommandArguments())
	if m.Chat.IsPrivate() {
		a.handlePrivateCommand(ctx, m, cmd, args)
		return
	}
	lock := a.lockFor(m.Chat.ID)
	lock.Lock()
	defer lock.Unlock()
	switch cmd {
	case "newgame":
		a.cmdNewGame(ctx, m, args)
	case "join":
		a.cmdJoin(ctx, m)
	case "leave":
		a.cmdLeave(ctx, m)
	case "begin":
		a.cmdBegin(ctx, m)
	case "cancel":
		a.cmdCancel(ctx, m)
	case "balance":
		a.cmdBalance(ctx, m)
	case "checkin":
		a.cmdCheckin(ctx, m)
	case "redeem":
		a.cmdRedeem(ctx, m, args)
	}
}

func (a *App) handlePrivateCommand(ctx context.Context, m *tgbotapi.Message, cmd string, args []string) {
	if !a.cfg.IsAdmin(m.From.ID) {
		a.reply(m.Chat.ID, m.MessageID, "私聊管理命令仅限全局管理员使用。")
		return
	}
	switch cmd {
	case "code":
		if len(args) < 3 {
			a.reply(m.Chat.ID, m.MessageID, "用法：/code CODE AMOUNT MAX_USES [YYYY-MM-DD]")
			return
		}
		amount, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || amount <= 0 {
			a.reply(m.Chat.ID, m.MessageID, "AMOUNT 必须是正整数。")
			return
		}
		maxUses64, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || maxUses64 <= 0 {
			a.reply(m.Chat.ID, m.MessageID, "MAX_USES 必须是正整数。")
			return
		}
		var expires *time.Time
		if len(args) >= 4 {
			t, err := time.ParseInLocation("2006-01-02 15:04:05", args[3]+" 23:59:59", a.cfg.Location)
			if err != nil {
				a.reply(m.Chat.ID, m.MessageID, "日期格式应为 YYYY-MM-DD。")
				return
			}
			expires = &t
		}
		if err := a.store.CreateRechargeCode(ctx, strings.ToUpper(args[0]), amount, int(maxUses64), expires, m.From.ID); err != nil {
			a.reply(m.Chat.ID, m.MessageID, "创建失败："+err.Error())
			return
		}
		a.reply(m.Chat.ID, m.MessageID, "充值码已创建。")
	case "voidcode":
		if len(args) < 1 {
			a.reply(m.Chat.ID, m.MessageID, "用法：/voidcode CODE")
			return
		}
		if err := a.store.VoidRechargeCode(ctx, strings.ToUpper(args[0])); err != nil {
			a.reply(m.Chat.ID, m.MessageID, "作废失败："+err.Error())
			return
		}
		a.reply(m.Chat.ID, m.MessageID, "充值码已作废。")
	case "codeinfo":
		if len(args) < 1 {
			a.reply(m.Chat.ID, m.MessageID, "用法：/codeinfo CODE")
			return
		}
		code, err := a.store.RechargeCode(ctx, strings.ToUpper(args[0]))
		if err != nil {
			a.reply(m.Chat.ID, m.MessageID, "查询失败："+err.Error())
			return
		}
		expires := "永不过期"
		if code.ExpiresAt != nil {
			expires = code.ExpiresAt.In(a.cfg.Location).Format("2006-01-02")
		}
		a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("代码：%s\n金额：%d\n次数：%d/%d\n状态：%s\n过期：%s", code.Code, code.Amount, code.UsedCount, code.MaxUses, code.Status, expires))
	}
}

func (a *App) cmdNewGame(ctx context.Context, m *tgbotapi.Message, args []string) {
	active, err := a.store.ActiveGame(ctx, m.Chat.ID)
	if err == nil && active.Type != "" {
		a.reply(m.Chat.ID, m.MessageID, "本群已有等待中或进行中的牌局。")
		return
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.reply(m.Chat.ID, m.MessageID, "读取牌局失败："+err.Error())
		return
	}
	if len(args) == 0 {
		msg := tgbotapi.NewMessage(m.Chat.ID, "请选择要创建的游戏：")
		msg.ReplyToMessageID = m.MessageID
		msg.ReplyMarkup = gameSelectKeyboard()
		if _, err := a.api.Send(msg); err != nil {
			a.log.Error("send game select", "error", err)
		}
		return
	}
	gameType, rest := parseGameType(args)
	switch gameType {
	case store.GameTypeHoldem:
		a.createHoldemGame(ctx, m, rest)
	case store.GameTypeBlackjack:
		a.createBlackjackGame(ctx, m, rest)
	default:
		a.reply(m.Chat.ID, m.MessageID, "未知游戏类型。用法：/newgame holdem ... 或 /newgame blackjack ...")
	}
}

func (a *App) createHoldemGame(ctx context.Context, m *tgbotapi.Message, args []string) {
	sb, bb, buyIn, waitSeconds := a.cfg.SmallBlind, a.cfg.BigBlind, a.cfg.BuyIn, a.cfg.WaitSeconds
	if len(args) >= 1 {
		sb = parsePositiveOr(args[0], sb)
	}
	if len(args) >= 2 {
		bb = parsePositiveOr(args[1], bb)
	}
	if len(args) >= 3 {
		buyIn = parsePositiveOr(args[2], buyIn)
	}
	if len(args) >= 4 {
		waitSeconds = int(parsePositiveOr(args[3], int64(waitSeconds)))
	}
	if sb <= 0 || bb <= sb || buyIn < bb || waitSeconds < 5 {
		a.reply(m.Chat.ID, m.MessageID, "参数需满足：0 < 小盲 < 大盲，买入不少于大盲，等待至少 5 秒。")
		return
	}
	bal, err := a.store.Balance(ctx, m.Chat.ID, m.From.ID)
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "读取余额失败："+err.Error())
		return
	}
	if bal < buyIn {
		a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("余额不足，买入需要 %d，当前 %d。", buyIn, bal))
		return
	}
	game := poker.NewGame(poker.Settings{
		ID:            uuid.NewString(),
		ChatID:        m.Chat.ID,
		CreatorID:     m.From.ID,
		SmallBlind:    sb,
		BigBlind:      bb,
		BuyIn:         buyIn,
		WaitSeconds:   waitSeconds,
		ActionSeconds: a.cfg.ActionSeconds,
		RakePercent:   a.cfg.RakePercent,
		RakeCap:       bb * a.cfg.RakeCapBigBlinds,
	})
	if err := game.AddPlayer(m.From.ID, displayName(m.From)); err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	if err := a.store.CreateWaitingGame(ctx, game, deadline); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "创建牌局失败："+err.Error())
		return
	}
	msg := tgbotapi.NewMessage(m.Chat.ID, waitingText(game, deadline.In(a.cfg.Location)))
	msg.ReplyMarkup = waitingKeyboard(game.ID)
	sent, err := a.api.Send(msg)
	if err == nil {
		_ = a.store.SaveWaitingGame(ctx, game, sent.MessageID, deadline)
	}
}

func (a *App) createBlackjackGame(ctx context.Context, m *tgbotapi.Message, args []string) {
	bet, waitSeconds := a.cfg.BigBlind, a.cfg.WaitSeconds
	if len(args) >= 1 {
		bet = parsePositiveOr(args[0], bet)
	}
	if len(args) >= 2 {
		waitSeconds = int(parsePositiveOr(args[1], int64(waitSeconds)))
	}
	if bet <= 0 || waitSeconds < 5 {
		a.reply(m.Chat.ID, m.MessageID, "参数需满足：下注为正整数，等待至少 5 秒。")
		return
	}
	bal, err := a.store.Balance(ctx, m.Chat.ID, m.From.ID)
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "读取余额失败："+err.Error())
		return
	}
	if bal < bet {
		a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("余额不足，21点下注需要 %d，当前 %d。", bet, bal))
		return
	}
	game := blackjack.NewGame(blackjack.Settings{
		ID:            uuid.NewString(),
		ChatID:        m.Chat.ID,
		CreatorID:     m.From.ID,
		Bet:           bet,
		WaitSeconds:   waitSeconds,
		ActionSeconds: a.cfg.ActionSeconds,
	})
	if err := game.AddPlayer(m.From.ID, displayName(m.From)); err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	if err := a.store.CreateWaitingBlackjack(ctx, game, deadline); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "创建21点失败："+err.Error())
		return
	}
	msg := tgbotapi.NewMessage(m.Chat.ID, blackjackWaitingText(game, deadline.In(a.cfg.Location)))
	msg.ReplyMarkup = waitingKeyboard(game.ID)
	sent, err := a.api.Send(msg)
	if err == nil {
		_ = a.store.SaveWaitingBlackjack(ctx, game, sent.MessageID, deadline)
	}
}

func (a *App) cmdJoin(ctx context.Context, m *tgbotapi.Message) {
	active, err := a.store.ActiveGame(ctx, m.Chat.ID)
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "当前没有等待中的牌局。")
		return
	}
	if active.Type == store.GameTypeBlackjack {
		a.joinBlackjack(ctx, m, active)
		return
	}
	game := active.Game
	if game == nil || game.Status != poker.StatusWaiting {
		a.reply(m.Chat.ID, m.MessageID, "牌局已经开始，不能加入。")
		return
	}
	bal, err := a.store.Balance(ctx, m.Chat.ID, m.From.ID)
	if err != nil || bal < game.BuyIn {
		a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("余额不足，买入需要 %d。", game.BuyIn))
		return
	}
	if err := game.AddPlayer(m.From.ID, displayName(m.From)); err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	if err := a.store.SaveWaitingGame(ctx, game, active.WaitingMessageID, active.ActionDeadline); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "加入失败："+err.Error())
		return
	}
	a.send(m.Chat.ID, waitingText(game, active.ActionDeadline.In(a.cfg.Location)), waitingKeyboard(game.ID))
}

func (a *App) joinBlackjack(ctx context.Context, m *tgbotapi.Message, active store.ActiveGame) {
	game := active.Blackjack
	if game == nil || game.Status != blackjack.StatusWaiting {
		a.reply(m.Chat.ID, m.MessageID, "牌局已经开始，不能加入。")
		return
	}
	bal, err := a.store.Balance(ctx, m.Chat.ID, m.From.ID)
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "读取余额失败："+err.Error())
		return
	}
	if bal < game.Bet {
		a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("余额不足，21点下注需要 %d，当前 %d。", game.Bet, bal))
		return
	}
	if err := game.AddPlayer(m.From.ID, displayName(m.From)); err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	if err := a.store.SaveWaitingBlackjack(ctx, game, active.WaitingMessageID, active.ActionDeadline); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "加入失败："+err.Error())
		return
	}
	a.send(m.Chat.ID, blackjackWaitingText(game, active.ActionDeadline.In(a.cfg.Location)), waitingKeyboard(game.ID))
}

func (a *App) cmdLeave(ctx context.Context, m *tgbotapi.Message) {
	active, err := a.store.ActiveGame(ctx, m.Chat.ID)
	if err != nil || activeStatus(active) != poker.StatusWaiting {
		a.reply(m.Chat.ID, m.MessageID, "当前没有等待中的牌局。")
		return
	}
	if active.Type == store.GameTypeBlackjack {
		if err := active.Blackjack.RemovePlayer(m.From.ID); err != nil {
			a.reply(m.Chat.ID, m.MessageID, err.Error())
			return
		}
		if err := a.store.SaveWaitingBlackjack(ctx, active.Blackjack, active.WaitingMessageID, active.ActionDeadline); err != nil {
			a.reply(m.Chat.ID, m.MessageID, "离开失败："+err.Error())
			return
		}
		a.send(m.Chat.ID, blackjackWaitingText(active.Blackjack, active.ActionDeadline.In(a.cfg.Location)), waitingKeyboard(active.Blackjack.ID))
		return
	}
	if err := active.Game.RemovePlayer(m.From.ID); err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	if err := a.store.SaveWaitingGame(ctx, active.Game, active.WaitingMessageID, active.ActionDeadline); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "离开失败："+err.Error())
		return
	}
	a.send(m.Chat.ID, waitingText(active.Game, active.ActionDeadline.In(a.cfg.Location)), waitingKeyboard(active.Game.ID))
}

func (a *App) cmdBegin(ctx context.Context, m *tgbotapi.Message) {
	active, err := a.store.ActiveGame(ctx, m.Chat.ID)
	if err != nil || activeStatus(active) != poker.StatusWaiting {
		a.reply(m.Chat.ID, m.MessageID, "当前没有等待中的牌局。")
		return
	}
	if activeCreatorID(active) != m.From.ID && !a.cfg.IsAdmin(m.From.ID) {
		a.reply(m.Chat.ID, m.MessageID, "只有发起人或管理员可以提前开局。")
		return
	}
	a.startWaitingGame(ctx, active, false)
}

func (a *App) cmdCancel(ctx context.Context, m *tgbotapi.Message) {
	active, err := a.store.ActiveGame(ctx, m.Chat.ID)
	if err != nil || activeStatus(active) != poker.StatusWaiting {
		a.reply(m.Chat.ID, m.MessageID, "当前没有可取消的等待局。")
		return
	}
	if activeCreatorID(active) != m.From.ID && !a.cfg.IsAdmin(m.From.ID) {
		a.reply(m.Chat.ID, m.MessageID, "只有发起人或管理员可以取消。")
		return
	}
	if active.Type == store.GameTypeBlackjack {
		if err := a.store.CancelWaitingBlackjack(ctx, active.Blackjack, m.From.ID); err != nil {
			a.reply(m.Chat.ID, m.MessageID, "取消失败："+err.Error())
			return
		}
		a.send(m.Chat.ID, "21点牌局已取消。", nil)
		return
	}
	if err := a.store.CancelWaitingGame(ctx, active.Game, m.From.ID); err != nil {
		a.reply(m.Chat.ID, m.MessageID, "取消失败："+err.Error())
		return
	}
	a.send(m.Chat.ID, "牌局已取消。", nil)
}

func (a *App) cmdBalance(ctx context.Context, m *tgbotapi.Message) {
	bal, err := a.store.Balance(ctx, m.Chat.ID, m.From.ID)
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "查询失败："+err.Error())
		return
	}
	a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("%s 当前余额：%d", displayName(m.From), bal))
}

func (a *App) cmdCheckin(ctx context.Context, m *tgbotapi.Message) {
	bal, amount, err := a.store.Checkin(ctx, m.Chat.ID, m.From.ID, time.Now().In(a.cfg.Location))
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, err.Error())
		return
	}
	a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("签到成功，获得 %d，当前余额 %d。", amount, bal))
}

func (a *App) cmdRedeem(ctx context.Context, m *tgbotapi.Message, args []string) {
	if len(args) < 1 {
		a.reply(m.Chat.ID, m.MessageID, "用法：/redeem CODE")
		return
	}
	bal, amount, err := a.store.Redeem(ctx, m.Chat.ID, m.From.ID, strings.ToUpper(args[0]), time.Now())
	if err != nil {
		a.reply(m.Chat.ID, m.MessageID, "兑换失败："+err.Error())
		return
	}
	a.reply(m.Chat.ID, m.MessageID, fmt.Sprintf("兑换成功，获得 %d，当前余额 %d。", amount, bal))
}

func (a *App) handleCallback(ctx context.Context, q *tgbotapi.CallbackQuery) {
	if q.Message == nil || q.From == nil {
		return
	}
	chatID := q.Message.Chat.ID
	if !a.cfg.IsAllowedChat(chatID) {
		a.answerCallback(q.ID, "本群未启用", true)
		return
	}
	parts := strings.Split(q.Data, ":")
	if len(parts) < 2 {
		a.answerCallback(q.ID, "无效按钮", true)
		return
	}
	lock := a.lockFor(chatID)
	lock.Lock()
	defer lock.Unlock()
	if parts[0] == "select" {
		a.handleGameSelect(ctx, q, parts)
		return
	}
	active, err := a.store.ActiveGame(ctx, chatID)
	if err != nil {
		a.answerCallback(q.ID, "牌局已结束", true)
		return
	}
	if parts[1] != activeID(active) {
		a.answerCallback(q.ID, "这是旧牌局的按钮", true)
		return
	}
	switch parts[0] {
	case "join":
		a.answerCallback(q.ID, "已收到", false)
		msg := &tgbotapi.Message{Chat: q.Message.Chat, From: q.From, MessageID: q.Message.MessageID}
		a.cmdJoin(ctx, msg)
	case "begin":
		if activeCreatorID(active) != q.From.ID && !a.cfg.IsAdmin(q.From.ID) {
			a.answerCallback(q.ID, "只有发起人或管理员可以开局", true)
			return
		}
		a.answerCallback(q.ID, "准备开局", false)
		a.startWaitingGame(ctx, active, false)
	case "hole":
		game := active.Game
		if game == nil {
			a.answerCallback(q.ID, "这不是德州牌局", true)
			return
		}
		hole, ok := game.PlayerHole(q.From.ID)
		if !ok {
			a.answerCallback(q.ID, "你不在本局，或尚未发牌", true)
			return
		}
		a.answerCallback(q.ID, "你的手牌："+poker.CardsString(hole), true)
	case "act":
		game := active.Game
		if game == nil {
			a.answerCallback(q.ID, "这不是德州牌局", true)
			return
		}
		if len(parts) < 3 {
			a.answerCallback(q.ID, "无效行动", true)
			return
		}
		kind := poker.ActionKind(parts[2])
		amount := int64(0)
		if kind == poker.ActionRaise {
			amount = game.MinRaiseTo()
		}
		a.answerCallback(q.ID, "行动已提交", false)
		a.applyAction(ctx, game, q.From.ID, kind, amount)
	case "bjhand":
		game := active.Blackjack
		if game == nil {
			a.answerCallback(q.ID, "这不是21点牌局", true)
			return
		}
		hand, ok := blackjackPlayerHand(game, q.From.ID)
		if !ok {
			a.answerCallback(q.ID, "你不在本局，或尚未发牌", true)
			return
		}
		a.answerCallback(q.ID, fmt.Sprintf("你的手牌：%s（%d点）", poker.CardsString(hand), blackjack.HandValue(hand)), true)
	case "bjact":
		game := active.Blackjack
		if game == nil {
			a.answerCallback(q.ID, "这不是21点牌局", true)
			return
		}
		if len(parts) < 3 {
			a.answerCallback(q.ID, "无效行动", true)
			return
		}
		a.answerCallback(q.ID, "行动已提交", false)
		a.applyBlackjackAction(ctx, game, q.From.ID, blackjack.ActionKind(parts[2]))
	}
}

func (a *App) handleGameSelect(ctx context.Context, q *tgbotapi.CallbackQuery, parts []string) {
	if len(parts) < 2 {
		a.answerCallback(q.ID, "无效游戏选择", true)
		return
	}
	if _, err := a.store.ActiveGame(ctx, q.Message.Chat.ID); err == nil {
		a.answerCallback(q.ID, "本群已有等待中或进行中的牌局", true)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		a.answerCallback(q.ID, "读取牌局失败", true)
		return
	}
	msg := &tgbotapi.Message{
		Chat:      q.Message.Chat,
		From:      q.From,
		MessageID: q.Message.MessageID,
	}
	switch parts[1] {
	case store.GameTypeHoldem:
		a.answerCallback(q.ID, "创建德州牌局", false)
		a.createHoldemGame(ctx, msg, nil)
	case store.GameTypeBlackjack:
		a.answerCallback(q.ID, "创建21点牌局", false)
		a.createBlackjackGame(ctx, msg, nil)
	default:
		a.answerCallback(q.ID, "未知游戏类型", true)
	}
}

func (a *App) startWaitingGame(ctx context.Context, active store.ActiveGame, auto bool) {
	if active.Type == store.GameTypeBlackjack {
		a.startWaitingBlackjack(ctx, active, auto)
		return
	}
	game := active.Game
	if game.Status != poker.StatusWaiting {
		return
	}
	if len(game.Players) < 2 {
		if auto {
			_ = a.store.CancelWaitingGame(ctx, game, 0)
			a.send(game.ChatID, "等待时间结束，人数不足 2 人，牌局已自动取消。", nil)
		} else {
			a.send(game.ChatID, "至少需要 2 人开局。", nil)
		}
		return
	}
	if err := game.Start(); err != nil {
		a.send(game.ChatID, "开局失败："+err.Error(), nil)
		return
	}
	deadline := time.Now().Add(time.Duration(game.ActionSeconds) * time.Second)
	if err := a.store.BeginGame(ctx, game, deadline); err != nil {
		a.send(game.ChatID, "开局失败："+err.Error(), nil)
		return
	}
	a.send(game.ChatID, "牌局开始！\n"+gameText(game, deadline.In(a.cfg.Location)), actionKeyboard(game))
}

func (a *App) startWaitingBlackjack(ctx context.Context, active store.ActiveGame, auto bool) {
	game := active.Blackjack
	if game.Status != blackjack.StatusWaiting {
		return
	}
	if len(game.Players) < 1 {
		if auto {
			_ = a.store.CancelWaitingBlackjack(ctx, game, 0)
			a.send(game.ChatID, "等待时间结束，人数不足，21点牌局已自动取消。", nil)
		} else {
			a.send(game.ChatID, "至少需要 1 人开局。", nil)
		}
		return
	}
	if err := game.Start(); err != nil {
		a.send(game.ChatID, "21点开局失败："+err.Error(), nil)
		return
	}
	if game.Status == blackjack.StatusFinished {
		if err := a.store.BeginBlackjack(ctx, game, time.Time{}); err != nil {
			a.send(game.ChatID, "21点开局失败："+err.Error(), nil)
			return
		}
		if err := a.store.FinishBlackjack(ctx, game); err != nil {
			a.send(game.ChatID, "21点结算失败："+err.Error(), nil)
			return
		}
		a.send(game.ChatID, blackjackSettlementText(game), nil)
		return
	}
	deadline := time.Now().Add(time.Duration(game.ActionSeconds) * time.Second)
	if err := a.store.BeginBlackjack(ctx, game, deadline); err != nil {
		a.send(game.ChatID, "21点开局失败："+err.Error(), nil)
		return
	}
	a.send(game.ChatID, "21点开始！\n"+blackjackGameText(game, deadline.In(a.cfg.Location)), blackjackActionKeyboard(game))
}

func (a *App) applyAction(ctx context.Context, game *poker.Game, userID int64, kind poker.ActionKind, amount int64) {
	result, err := game.ApplyAction(userID, kind, amount)
	if err != nil {
		a.send(game.ChatID, err.Error(), nil)
		return
	}
	text := strings.Join(result.Messages, "\n")
	if result.Finished {
		if err := a.store.FinishGame(ctx, game); err != nil {
			a.send(game.ChatID, "结算失败："+err.Error(), nil)
			return
		}
		if text != "" {
			text += "\n"
		}
		text += settlementText(game)
		a.send(game.ChatID, text, nil)
		return
	}
	deadline := time.Now().Add(time.Duration(game.ActionSeconds) * time.Second)
	if err := a.store.SaveRunningGame(ctx, game, deadline, userID, string(kind), "{}"); err != nil {
		a.send(game.ChatID, "保存牌局失败："+err.Error(), nil)
		return
	}
	if text != "" {
		text += "\n"
	}
	text += gameText(game, deadline.In(a.cfg.Location))
	a.send(game.ChatID, text, actionKeyboard(game))
}

func (a *App) applyBlackjackAction(ctx context.Context, game *blackjack.Game, userID int64, kind blackjack.ActionKind) {
	result, err := game.ApplyAction(userID, kind)
	if err != nil {
		a.send(game.ChatID, err.Error(), nil)
		return
	}
	text := strings.Join(result.Messages, "\n")
	if result.Finished {
		if err := a.store.FinishBlackjack(ctx, game); err != nil {
			a.send(game.ChatID, "21点结算失败："+err.Error(), nil)
			return
		}
		if text != "" {
			text += "\n"
		}
		text += blackjackSettlementText(game)
		a.send(game.ChatID, text, nil)
		return
	}
	deadline := time.Now().Add(time.Duration(game.ActionSeconds) * time.Second)
	if err := a.store.SaveRunningBlackjack(ctx, game, deadline, userID, string(kind), "{}"); err != nil {
		a.send(game.ChatID, "保存21点失败："+err.Error(), nil)
		return
	}
	if text != "" {
		text += "\n"
	}
	text += blackjackGameText(game, deadline.In(a.cfg.Location))
	a.send(game.ChatID, text, blackjackActionKeyboard(game))
}

func (a *App) deadlineLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.handleDeadlines(ctx)
		}
	}
}

func (a *App) handleDeadlines(ctx context.Context) {
	now := time.Now()
	waiting, err := a.store.WaitingGamesDue(ctx, now)
	if err == nil {
		for _, active := range waiting {
			lock := a.lockFor(activeChatID(active))
			lock.Lock()
			fresh, err := a.store.ActiveGame(ctx, activeChatID(active))
			if err == nil && activeStatus(fresh) == poker.StatusWaiting && !fresh.ActionDeadline.After(now) {
				a.startWaitingGame(ctx, fresh, true)
			}
			lock.Unlock()
		}
	}
	running, err := a.store.RunningGames(ctx)
	if err != nil {
		a.log.Error("load running games", "error", err)
		return
	}
	for _, active := range running {
		if active.ActionDeadline.IsZero() || active.ActionDeadline.After(now) {
			continue
		}
		lock := a.lockFor(activeChatID(active))
		lock.Lock()
		fresh, err := a.store.ActiveGame(ctx, activeChatID(active))
		if err == nil && activeStatus(fresh) == poker.StatusRunning && !fresh.ActionDeadline.After(now) {
			if fresh.Type == store.GameTypeBlackjack {
				a.handleBlackjackTimeout(ctx, fresh)
				lock.Unlock()
				continue
			}
			result, err := fresh.Game.AutoAction()
			if err != nil {
				a.log.Error("auto action", "error", err)
			} else if result.Finished {
				if err := a.store.FinishGame(ctx, fresh.Game); err == nil {
					a.send(fresh.Game.ChatID, strings.Join(result.Messages, "\n")+"\n"+settlementText(fresh.Game), nil)
				}
			} else {
				deadline := time.Now().Add(time.Duration(fresh.Game.ActionSeconds) * time.Second)
				if err := a.store.SaveRunningGame(ctx, fresh.Game, deadline, 0, "timeout", "{}"); err == nil {
					a.send(fresh.Game.ChatID, strings.Join(result.Messages, "\n")+"\n"+gameText(fresh.Game, deadline.In(a.cfg.Location)), actionKeyboard(fresh.Game))
				}
			}
		}
		lock.Unlock()
	}
}

func (a *App) handleBlackjackTimeout(ctx context.Context, fresh store.ActiveGame) {
	result, err := fresh.Blackjack.AutoAction()
	if err != nil {
		a.log.Error("blackjack auto action", "error", err)
		return
	}
	if result.Finished {
		if err := a.store.FinishBlackjack(ctx, fresh.Blackjack); err == nil {
			a.send(fresh.Blackjack.ChatID, strings.Join(result.Messages, "\n")+"\n"+blackjackSettlementText(fresh.Blackjack), nil)
		}
		return
	}
	deadline := time.Now().Add(time.Duration(fresh.Blackjack.ActionSeconds) * time.Second)
	if err := a.store.SaveRunningBlackjack(ctx, fresh.Blackjack, deadline, 0, "timeout", "{}"); err == nil {
		a.send(fresh.Blackjack.ChatID, strings.Join(result.Messages, "\n")+"\n"+blackjackGameText(fresh.Blackjack, deadline.In(a.cfg.Location)), blackjackActionKeyboard(fresh.Blackjack))
	}
}

func (a *App) lockFor(chatID int64) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	lock := a.chatLock[chatID]
	if lock == nil {
		lock = &sync.Mutex{}
		a.chatLock[chatID] = lock
	}
	return lock
}

func (a *App) send(chatID int64, text string, markup any) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	if _, err := a.api.Send(msg); err != nil {
		a.log.Error("send message", "error", err)
	}
}

func (a *App) reply(chatID int64, replyTo int, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyTo
	if _, err := a.api.Send(msg); err != nil {
		a.log.Error("reply message", "error", err)
	}
}

func (a *App) answerCallback(id, text string, alert bool) {
	callback := tgbotapi.NewCallback(id, text)
	callback.ShowAlert = alert
	if _, err := a.api.Request(callback); err != nil {
		a.log.Error("answer callback", "error", err)
	}
}

func (a *App) setCommands() {
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "newgame", Description: "创建牌局"},
		tgbotapi.BotCommand{Command: "join", Description: "加入等待局"},
		tgbotapi.BotCommand{Command: "begin", Description: "提前开局"},
		tgbotapi.BotCommand{Command: "balance", Description: "查询余额"},
		tgbotapi.BotCommand{Command: "checkin", Description: "每日签到"},
		tgbotapi.BotCommand{Command: "redeem", Description: "兑换充值码"},
	)
	if _, err := a.api.Request(commands); err != nil {
		a.log.Warn("set commands failed", "error", err)
	}
}
