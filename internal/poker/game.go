package poker

import (
	"errors"
	"fmt"
	"sort"
)

const (
	StatusWaiting  = "waiting"
	StatusRunning  = "running"
	StatusFinished = "finished"
	StatusCanceled = "canceled"

	PlayerWaiting = "waiting"
	PlayerActive  = "active"
	PlayerFolded  = "folded"
	PlayerAllIn   = "allin"

	StreetWaiting = "waiting"
	StreetPreflop = "preflop"
	StreetFlop    = "flop"
	StreetTurn    = "turn"
	StreetRiver   = "river"
	StreetDone    = "done"
)

type Settings struct {
	ID            string `json:"id"`
	ChatID        int64  `json:"chat_id"`
	CreatorID     int64  `json:"creator_id"`
	SmallBlind    int64  `json:"small_blind"`
	BigBlind      int64  `json:"big_blind"`
	BuyIn         int64  `json:"buy_in"`
	WaitSeconds   int    `json:"wait_seconds"`
	ActionSeconds int    `json:"action_seconds"`
	RakePercent   int64  `json:"rake_percent"`
	RakeCap       int64  `json:"rake_cap"`
}

type Player struct {
	UserID     int64  `json:"user_id"`
	Display    string `json:"display"`
	Seat       int    `json:"seat"`
	Stack      int64  `json:"stack"`
	Status     string `json:"status"`
	Hole       []Card `json:"hole"`
	CurrentBet int64  `json:"current_bet"`
	TotalBet   int64  `json:"total_bet"`
	HasActed   bool   `json:"has_acted"`
}

type Game struct {
	Settings
	Status      string   `json:"status"`
	Street      string   `json:"street"`
	Players     []Player `json:"players"`
	Deck        []Card   `json:"deck"`
	Board       []Card   `json:"board"`
	Dealer      int      `json:"dealer"`
	CurrentTurn int      `json:"current_turn"`
	CurrentBet  int64    `json:"current_bet"`
	MinRaise    int64    `json:"min_raise"`
	Pot         int64    `json:"pot"`
	Awards      []Award  `json:"awards"`
}

type ActionKind string

const (
	ActionFold  ActionKind = "fold"
	ActionCheck ActionKind = "check"
	ActionCall  ActionKind = "call"
	ActionRaise ActionKind = "raise"
	ActionAllIn ActionKind = "allin"
)

type ActionResult struct {
	Messages []string
	Awards   []Award
	Finished bool
}

type Award struct {
	UserID       int64  `json:"user_id"`
	Gross        int64  `json:"gross"`
	Fee          int64  `json:"fee"`
	Net          int64  `json:"net"`
	Reason       string `json:"reason"`
	HandCategory string `json:"hand_category"`
}

func NewGame(settings Settings) *Game {
	return &Game{
		Settings: settings,
		Status:   StatusWaiting,
		Street:   StreetWaiting,
		MinRaise: settings.BigBlind,
	}
}

func (g *Game) AddPlayer(userID int64, display string) error {
	if g.Status != StatusWaiting {
		return errors.New("牌局已经开始")
	}
	if len(g.Players) >= 9 {
		return errors.New("本局最多 9 人")
	}
	if g.findPlayer(userID) >= 0 {
		return errors.New("你已经加入本局")
	}
	g.Players = append(g.Players, Player{
		UserID:  userID,
		Display: display,
		Seat:    len(g.Players),
		Stack:   g.BuyIn,
		Status:  PlayerWaiting,
	})
	return nil
}

func (g *Game) RemovePlayer(userID int64) error {
	if g.Status != StatusWaiting {
		return errors.New("牌局已经开始")
	}
	idx := g.findPlayer(userID)
	if idx < 0 {
		return errors.New("你不在本局中")
	}
	g.Players = append(g.Players[:idx], g.Players[idx+1:]...)
	for i := range g.Players {
		g.Players[i].Seat = i
	}
	return nil
}

func (g *Game) Start() error {
	if g.Status != StatusWaiting {
		return errors.New("牌局不是等待状态")
	}
	if len(g.Players) < 2 {
		return errors.New("至少需要 2 人开局")
	}
	deck := NewDeck()
	if err := Shuffle(deck); err != nil {
		return err
	}
	g.Deck = deck
	g.Status = StatusRunning
	g.Street = StreetPreflop
	g.Dealer = 0
	g.CurrentBet = 0
	g.MinRaise = g.BigBlind
	for i := range g.Players {
		g.Players[i].Status = PlayerActive
		g.Players[i].Hole = nil
		g.Players[i].CurrentBet = 0
		g.Players[i].TotalBet = 0
		g.Players[i].HasActed = false
	}
	for cardNo := 0; cardNo < 2; cardNo++ {
		for i := range g.Players {
			card, err := g.draw()
			if err != nil {
				return err
			}
			g.Players[i].Hole = append(g.Players[i].Hole, card)
		}
	}
	if len(g.Players) == 2 {
		g.postBlind(g.Dealer, g.SmallBlind)
		g.postBlind(g.nextSeat(g.Dealer), g.BigBlind)
		g.CurrentTurn = g.Dealer
		return nil
	}
	sb := g.nextSeat(g.Dealer)
	bb := g.nextSeat(sb)
	g.postBlind(sb, g.SmallBlind)
	g.postBlind(bb, g.BigBlind)
	g.CurrentTurn = g.nextActionableAfter(bb)
	return nil
}

func (g *Game) ApplyAction(userID int64, kind ActionKind, amount int64) (ActionResult, error) {
	if g.Status != StatusRunning {
		return ActionResult{}, errors.New("当前没有进行中的牌局")
	}
	idx := g.findPlayer(userID)
	if idx < 0 {
		return ActionResult{}, errors.New("你不在本局中")
	}
	if idx != g.CurrentTurn {
		return ActionResult{}, fmt.Errorf("还没轮到你行动，当前行动玩家是 %s", g.Players[g.CurrentTurn].Display)
	}
	p := &g.Players[idx]
	if !canAct(*p) {
		return ActionResult{}, errors.New("你当前不能行动")
	}
	var messages []string
	toCall := g.CurrentBet - p.CurrentBet
	switch kind {
	case ActionFold:
		p.Status = PlayerFolded
		p.HasActed = true
		messages = append(messages, fmt.Sprintf("%s 弃牌", p.Display))
	case ActionCheck:
		if toCall > 0 {
			return ActionResult{}, fmt.Errorf("需要跟注 %d，不能过牌", toCall)
		}
		p.HasActed = true
		messages = append(messages, fmt.Sprintf("%s 过牌", p.Display))
	case ActionCall:
		if toCall <= 0 {
			p.HasActed = true
			messages = append(messages, fmt.Sprintf("%s 过牌", p.Display))
			break
		}
		paid := g.pay(idx, min(toCall, p.Stack))
		p.HasActed = true
		if p.Stack == 0 {
			p.Status = PlayerAllIn
			messages = append(messages, fmt.Sprintf("%s 跟注 %d 并 all-in", p.Display, paid))
		} else {
			messages = append(messages, fmt.Sprintf("%s 跟注 %d", p.Display, paid))
		}
	case ActionRaise:
		if amount <= g.CurrentBet {
			return ActionResult{}, fmt.Errorf("加注目标必须大于当前下注 %d", g.CurrentBet)
		}
		needed := amount - p.CurrentBet
		if needed > p.Stack {
			return ActionResult{}, errors.New("筹码不足；可以点击 All-in")
		}
		raiseBy := amount - g.CurrentBet
		if raiseBy < g.MinRaise && needed != p.Stack {
			return ActionResult{}, fmt.Errorf("最小加注到 %d", g.CurrentBet+g.MinRaise)
		}
		paid := g.pay(idx, needed)
		g.CurrentBet = p.CurrentBet
		if raiseBy >= g.MinRaise {
			g.MinRaise = raiseBy
		}
		g.resetActedExcept(idx)
		p.HasActed = true
		if p.Stack == 0 {
			p.Status = PlayerAllIn
			messages = append(messages, fmt.Sprintf("%s 加注到 %d 并 all-in", p.Display, p.CurrentBet))
		} else {
			messages = append(messages, fmt.Sprintf("%s 加注到 %d（投入 %d）", p.Display, p.CurrentBet, paid))
		}
	case ActionAllIn:
		if p.Stack <= 0 {
			return ActionResult{}, errors.New("没有可 all-in 的筹码")
		}
		oldCurrent := g.CurrentBet
		paid := g.pay(idx, p.Stack)
		p.Status = PlayerAllIn
		p.HasActed = true
		if p.CurrentBet > g.CurrentBet {
			raiseBy := p.CurrentBet - oldCurrent
			g.CurrentBet = p.CurrentBet
			if raiseBy >= g.MinRaise {
				g.MinRaise = raiseBy
				g.resetActedExcept(idx)
				p.HasActed = true
			}
		}
		messages = append(messages, fmt.Sprintf("%s all-in %d", p.Display, paid))
	default:
		return ActionResult{}, errors.New("未知行动")
	}

	if g.livePlayerCount() == 1 {
		awards := g.finishUncontested()
		return ActionResult{Messages: messages, Awards: awards, Finished: true}, nil
	}
	if g.roundComplete() {
		nextMessages, awards, finished, err := g.advanceStreet()
		if err != nil {
			return ActionResult{}, err
		}
		messages = append(messages, nextMessages...)
		return ActionResult{Messages: messages, Awards: awards, Finished: finished}, nil
	}
	g.CurrentTurn = g.nextActionableAfter(idx)
	return ActionResult{Messages: messages}, nil
}

func (g *Game) AutoAction() (ActionResult, error) {
	if g.Status != StatusRunning || g.CurrentTurn < 0 || g.CurrentTurn >= len(g.Players) {
		return ActionResult{}, errors.New("没有可超时处理的行动")
	}
	p := g.Players[g.CurrentTurn]
	if g.CurrentBet > p.CurrentBet {
		return g.ApplyAction(p.UserID, ActionFold, 0)
	}
	return g.ApplyAction(p.UserID, ActionCheck, 0)
}

func (g *Game) CurrentPlayer() *Player {
	if g.Status != StatusRunning || g.CurrentTurn < 0 || g.CurrentTurn >= len(g.Players) {
		return nil
	}
	return &g.Players[g.CurrentTurn]
}

func (g *Game) ToCall() int64 {
	p := g.CurrentPlayer()
	if p == nil {
		return 0
	}
	return max(0, g.CurrentBet-p.CurrentBet)
}

func (g *Game) MinRaiseTo() int64 {
	return g.CurrentBet + g.MinRaise
}

func (g *Game) PlayerHole(userID int64) ([]Card, bool) {
	idx := g.findPlayer(userID)
	if idx < 0 || len(g.Players[idx].Hole) != 2 {
		return nil, false
	}
	return append([]Card(nil), g.Players[idx].Hole...), true
}

func (g *Game) postBlind(idx int, amount int64) {
	g.pay(idx, min(amount, g.Players[idx].Stack))
	if g.Players[idx].Stack == 0 {
		g.Players[idx].Status = PlayerAllIn
	}
	if g.Players[idx].CurrentBet > g.CurrentBet {
		g.CurrentBet = g.Players[idx].CurrentBet
	}
}

func (g *Game) pay(idx int, amount int64) int64 {
	p := &g.Players[idx]
	paid := min(amount, p.Stack)
	p.Stack -= paid
	p.CurrentBet += paid
	p.TotalBet += paid
	g.Pot += paid
	return paid
}

func (g *Game) advanceStreet() ([]string, []Award, bool, error) {
	if g.actionableCount() <= 1 {
		for len(g.Board) < 5 {
			if err := g.dealNextBoard(); err != nil {
				return nil, nil, false, err
			}
		}
		awards, err := g.finishShowdown()
		return []string{"所有可行动玩家已 all-in，直接发完公共牌并摊牌"}, awards, true, err
	}
	if g.Street == StreetRiver {
		awards, err := g.finishShowdown()
		return []string{"进入摊牌"}, awards, true, err
	}
	if err := g.dealNextBoard(); err != nil {
		return nil, nil, false, err
	}
	for i := range g.Players {
		g.Players[i].CurrentBet = 0
		g.Players[i].HasActed = false
	}
	g.CurrentBet = 0
	g.MinRaise = g.BigBlind
	g.CurrentTurn = g.firstPostflopAction()
	return []string{fmt.Sprintf("进入 %s，公共牌：%s", streetName(g.Street), CardsString(g.Board))}, nil, false, nil
}

func (g *Game) dealNextBoard() error {
	count := 1
	switch g.Street {
	case StreetPreflop:
		g.Street = StreetFlop
		count = 3
	case StreetFlop:
		g.Street = StreetTurn
	case StreetTurn:
		g.Street = StreetRiver
	default:
		return errors.New("无法继续发公共牌")
	}
	for i := 0; i < count; i++ {
		card, err := g.draw()
		if err != nil {
			return err
		}
		g.Board = append(g.Board, card)
	}
	return nil
}

func (g *Game) finishUncontested() []Award {
	winner := -1
	for i, p := range g.Players {
		if p.Status != PlayerFolded {
			winner = i
			break
		}
	}
	if winner < 0 {
		return nil
	}
	award := g.makeAward(g.Players[winner].UserID, g.Pot, "其他玩家均已弃牌", "")
	g.Players[winner].Stack += award.Net
	g.Status = StatusFinished
	g.Street = StreetDone
	g.Awards = []Award{award}
	return g.Awards
}

func (g *Game) finishShowdown() ([]Award, error) {
	pots := buildSidePots(g.Players)
	awards := make([]Award, 0)
	feeUsed := int64(0)
	for _, pot := range pots {
		if pot.Uncalled {
			idx := pot.Eligible[0]
			award := Award{
				UserID: g.Players[idx].UserID,
				Gross:  pot.Amount,
				Fee:    0,
				Net:    pot.Amount,
				Reason: "未被跟注退回",
			}
			g.Players[idx].Stack += award.Net
			awards = append(awards, award)
			continue
		}
		winners, category, err := g.potWinners(pot.Eligible)
		if err != nil {
			return nil, err
		}
		if len(winners) == 0 {
			continue
		}
		share := pot.Amount / int64(len(winners))
		remainder := pot.Amount % int64(len(winners))
		for i, idx := range winners {
			gross := share
			if int64(i) < remainder {
				gross++
			}
			fee := rake(gross, g.RakePercent, max(0, g.RakeCap-feeUsed))
			feeUsed += fee
			award := Award{
				UserID:       g.Players[idx].UserID,
				Gross:        gross,
				Fee:          fee,
				Net:          gross - fee,
				Reason:       "摊牌胜出",
				HandCategory: category,
			}
			g.Players[idx].Stack += award.Net
			awards = append(awards, award)
		}
	}
	g.Status = StatusFinished
	g.Street = StreetDone
	g.Awards = awards
	return awards, nil
}

func (g *Game) potWinners(eligible []int) ([]int, string, error) {
	bestValue := uint64(0)
	category := ""
	winners := make([]int, 0)
	for _, idx := range eligible {
		p := g.Players[idx]
		if p.Status == PlayerFolded {
			continue
		}
		cards := append([]Card(nil), p.Hole...)
		cards = append(cards, g.Board...)
		rank, err := Evaluate(cards)
		if err != nil {
			return nil, "", err
		}
		if rank.Value > bestValue {
			bestValue = rank.Value
			category = rank.Category
			winners = []int{idx}
		} else if rank.Value == bestValue {
			winners = append(winners, idx)
		}
	}
	sort.Ints(winners)
	return winners, category, nil
}

type sidePot struct {
	Amount   int64
	Eligible []int
	Uncalled bool
}

func buildSidePots(players []Player) []sidePot {
	levelsMap := map[int64]bool{}
	for _, p := range players {
		if p.TotalBet > 0 {
			levelsMap[p.TotalBet] = true
		}
	}
	levels := make([]int64, 0, len(levelsMap))
	for level := range levelsMap {
		levels = append(levels, level)
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	pots := make([]sidePot, 0, len(levels))
	prev := int64(0)
	for _, level := range levels {
		amount := int64(0)
		contributors := 0
		eligible := make([]int, 0)
		for i, p := range players {
			if p.TotalBet >= level {
				amount += level - prev
				contributors++
				if p.Status != PlayerFolded {
					eligible = append(eligible, i)
				}
			} else if p.TotalBet > prev {
				amount += p.TotalBet - prev
				contributors++
			}
		}
		if amount > 0 && len(eligible) > 0 {
			pots = append(pots, sidePot{Amount: amount, Eligible: eligible, Uncalled: contributors == 1 && len(eligible) == 1})
		}
		prev = level
	}
	return pots
}

func (g *Game) makeAward(userID, gross int64, reason, category string) Award {
	fee := rake(gross, g.RakePercent, g.RakeCap)
	return Award{UserID: userID, Gross: gross, Fee: fee, Net: gross - fee, Reason: reason, HandCategory: category}
}

func rake(gross, percent, cap int64) int64 {
	if gross <= 0 || percent <= 0 || cap <= 0 {
		return 0
	}
	fee := gross * percent / 100
	if fee > cap {
		return cap
	}
	return fee
}

func (g *Game) roundComplete() bool {
	for _, p := range g.Players {
		if !canAct(p) {
			continue
		}
		if !p.HasActed || p.CurrentBet != g.CurrentBet {
			return false
		}
	}
	return true
}

func (g *Game) resetActedExcept(idx int) {
	for i := range g.Players {
		if i == idx || !canAct(g.Players[i]) {
			continue
		}
		g.Players[i].HasActed = false
	}
}

func (g *Game) firstPostflopAction() int {
	idx := g.nextActionableAfter(g.Dealer)
	if idx < 0 {
		return 0
	}
	return idx
}

func (g *Game) nextActionableAfter(idx int) int {
	if len(g.Players) == 0 {
		return -1
	}
	for offset := 1; offset <= len(g.Players); offset++ {
		next := (idx + offset) % len(g.Players)
		if canAct(g.Players[next]) {
			return next
		}
	}
	return -1
}

func (g *Game) nextSeat(idx int) int {
	return (idx + 1) % len(g.Players)
}

func (g *Game) findPlayer(userID int64) int {
	for i, p := range g.Players {
		if p.UserID == userID {
			return i
		}
	}
	return -1
}

func canAct(p Player) bool {
	return p.Status == PlayerActive && p.Stack > 0
}

func (g *Game) livePlayerCount() int {
	count := 0
	for _, p := range g.Players {
		if p.Status != PlayerFolded {
			count++
		}
	}
	return count
}

func (g *Game) actionableCount() int {
	count := 0
	for _, p := range g.Players {
		if canAct(p) {
			count++
		}
	}
	return count
}

func (g *Game) draw() (Card, error) {
	if len(g.Deck) == 0 {
		return Card{}, errors.New("牌堆为空")
	}
	card := g.Deck[0]
	g.Deck = g.Deck[1:]
	return card, nil
}

func streetName(street string) string {
	switch street {
	case StreetPreflop:
		return "翻牌前"
	case StreetFlop:
		return "翻牌圈"
	case StreetTurn:
		return "转牌圈"
	case StreetRiver:
		return "河牌圈"
	default:
		return street
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
