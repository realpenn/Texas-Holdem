package blackjack

import (
	"errors"
	"fmt"

	"texas-holdem/internal/poker"
)

const (
	StatusWaiting  = "waiting"
	StatusRunning  = "running"
	StatusFinished = "finished"
	StatusCanceled = "canceled"

	PlayerWaiting   = "waiting"
	PlayerActive    = "active"
	PlayerStand     = "stand"
	PlayerBust      = "bust"
	PlayerBlackjack = "blackjack"

	// DeckCount 副牌洗在一起，避免多人局把单副牌抽空。
	DeckCount = 4
)

type Settings struct {
	ID            string `json:"id"`
	ChatID        int64  `json:"chat_id"`
	CreatorID     int64  `json:"creator_id"`
	Bet           int64  `json:"bet"`
	WaitSeconds   int    `json:"wait_seconds"`
	ActionSeconds int    `json:"action_seconds"`
}

type Hand struct {
	Cards   []poker.Card `json:"cards"`
	Bet     int64        `json:"bet"`
	Status  string       `json:"status"`
	Doubled bool         `json:"doubled"`
}

type Player struct {
	UserID  int64  `json:"user_id"`
	Display string `json:"display"`
	Seat    int    `json:"seat"`
	Hands   []Hand `json:"hands"`
}

func (p *Player) TotalBet() int64 {
	var total int64
	for _, h := range p.Hands {
		total += h.Bet
	}
	return total
}

func (p *Player) SummaryStatus() string {
	if len(p.Hands) == 1 {
		return p.Hands[0].Status
	}
	out := ""
	for i, h := range p.Hands {
		if i > 0 {
			out += "/"
		}
		out += h.Status
	}
	return out
}

type Game struct {
	Settings
	Status      string       `json:"status"`
	Players     []Player     `json:"players"`
	Deck        []poker.Card `json:"deck"`
	Dealer      []poker.Card `json:"dealer"`
	CurrentTurn int          `json:"current_turn"`
	CurrentHand int          `json:"current_hand"`
	Awards      []Award      `json:"awards"`
}

type Award struct {
	UserID int64  `json:"user_id"`
	Bet    int64  `json:"bet"`
	Payout int64  `json:"payout"`
	Net    int64  `json:"net"`
	Reason string `json:"reason"`
}

type ActionKind string

const (
	ActionHit    ActionKind = "hit"
	ActionStand  ActionKind = "stand"
	ActionDouble ActionKind = "double"
	ActionSplit  ActionKind = "split"
)

type ActionResult struct {
	Messages []string
	Finished bool
}

func NewGame(settings Settings) *Game {
	return &Game{
		Settings:    settings,
		Status:      StatusWaiting,
		CurrentTurn: -1,
	}
}

func (g *Game) AddPlayer(userID int64, display string) error {
	if g.Status != StatusWaiting {
		return errors.New("牌局已经开始")
	}
	if len(g.Players) >= 7 {
		return errors.New("21点最多 7 人")
	}
	if g.findPlayer(userID) >= 0 {
		return errors.New("你已经加入本局")
	}
	g.Players = append(g.Players, Player{
		UserID:  userID,
		Display: display,
		Seat:    len(g.Players),
		Hands:   []Hand{{Bet: g.Bet, Status: PlayerWaiting}},
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
	if len(g.Players) < 1 {
		return errors.New("至少需要 1 人开局")
	}
	deck := make([]poker.Card, 0, 52*DeckCount)
	for i := 0; i < DeckCount; i++ {
		deck = append(deck, poker.NewDeck()...)
	}
	if err := poker.Shuffle(deck); err != nil {
		return err
	}
	g.Deck = deck
	g.Status = StatusRunning
	g.Dealer = nil
	for i := range g.Players {
		g.Players[i].Hands = []Hand{{Bet: g.Bet, Status: PlayerActive}}
	}
	for cardNo := 0; cardNo < 2; cardNo++ {
		for i := range g.Players {
			card, err := g.draw()
			if err != nil {
				return err
			}
			g.Players[i].Hands[0].Cards = append(g.Players[i].Hands[0].Cards, card)
		}
		card, err := g.draw()
		if err != nil {
			return err
		}
		g.Dealer = append(g.Dealer, card)
	}
	for i := range g.Players {
		if HandValue(g.Players[i].Hands[0].Cards) == 21 {
			g.Players[i].Hands[0].Status = PlayerBlackjack
		}
	}
	if DealerValue(g.Dealer) == 21 || g.actionableCount() == 0 {
		g.finish()
		return nil
	}
	g.CurrentTurn, g.CurrentHand = g.nextActionableAfter(-1, -1)
	return nil
}

// ExtraBetCost 返回该玩家执行 kind 需要额外投入的下注额（要牌/停牌为 0）。
// 调用方在执行前需确认玩家余额足够并扣款。
func (g *Game) ExtraBetCost(userID int64, kind ActionKind) int64 {
	if kind != ActionDouble && kind != ActionSplit {
		return 0
	}
	idx := g.findPlayer(userID)
	if idx < 0 || idx != g.CurrentTurn || g.CurrentHand < 0 || g.CurrentHand >= len(g.Players[idx].Hands) {
		return 0
	}
	return g.Players[idx].Hands[g.CurrentHand].Bet
}

func (g *Game) ApplyAction(userID int64, kind ActionKind) (ActionResult, error) {
	if g.Status != StatusRunning {
		return ActionResult{}, errors.New("当前没有进行中的21点牌局")
	}
	idx := g.findPlayer(userID)
	if idx < 0 {
		return ActionResult{}, errors.New("你不在本局中")
	}
	if idx != g.CurrentTurn {
		return ActionResult{}, fmt.Errorf("还没轮到你行动，当前行动玩家是 %s", g.Players[g.CurrentTurn].Display)
	}
	p := &g.Players[idx]
	if g.CurrentHand < 0 || g.CurrentHand >= len(p.Hands) {
		return ActionResult{}, errors.New("你当前不能行动")
	}
	hand := &p.Hands[g.CurrentHand]
	if hand.Status != PlayerActive {
		return ActionResult{}, errors.New("你当前不能行动")
	}
	label := handLabel(p, g.CurrentHand)
	var messages []string
	switch kind {
	case ActionHit:
		card, err := g.draw()
		if err != nil {
			return ActionResult{}, err
		}
		hand.Cards = append(hand.Cards, card)
		value := HandValue(hand.Cards)
		if value > 21 {
			hand.Status = PlayerBust
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，点数 %d，爆牌", label, card.String(), value))
		} else if value == 21 {
			hand.Status = PlayerStand
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，点数 21，自动停牌", label, card.String()))
		} else {
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，当前点数 %d", label, card.String(), value))
		}
	case ActionStand:
		hand.Status = PlayerStand
		messages = append(messages, fmt.Sprintf("%s 停牌，点数 %d", label, HandValue(hand.Cards)))
	case ActionDouble:
		if !canDouble(hand) {
			return ActionResult{}, errors.New("只能在两张牌时加倍")
		}
		card, err := g.draw()
		if err != nil {
			return ActionResult{}, err
		}
		hand.Bet *= 2
		hand.Doubled = true
		hand.Cards = append(hand.Cards, card)
		value := HandValue(hand.Cards)
		if value > 21 {
			hand.Status = PlayerBust
			messages = append(messages, fmt.Sprintf("%s 加倍至 %d，要牌 %s，点数 %d，爆牌", label, hand.Bet, card.String(), value))
		} else {
			hand.Status = PlayerStand
			messages = append(messages, fmt.Sprintf("%s 加倍至 %d，要牌 %s，点数 %d，停牌", label, hand.Bet, card.String(), value))
		}
	case ActionSplit:
		if !canSplit(p, hand) {
			return ActionResult{}, errors.New("只有两张同点数的牌且未分过牌时才能分牌")
		}
		isAces := cardPoint(hand.Cards[0]) == 11
		second := Hand{Cards: []poker.Card{hand.Cards[1]}, Bet: hand.Bet, Status: PlayerActive}
		hand.Cards = hand.Cards[:1]
		p.Hands = append(p.Hands, second)
		for i := range p.Hands {
			card, err := g.draw()
			if err != nil {
				return ActionResult{}, err
			}
			p.Hands[i].Cards = append(p.Hands[i].Cards, card)
			// 分出的 A 每手只补一张牌后自动停牌（常见规则）。
			if isAces || HandValue(p.Hands[i].Cards) == 21 {
				p.Hands[i].Status = PlayerStand
			}
		}
		messages = append(messages, fmt.Sprintf("%s 分牌：手牌1 %s（%d点），手牌2 %s（%d点）",
			p.Display,
			poker.CardsString(p.Hands[0].Cards), HandValue(p.Hands[0].Cards),
			poker.CardsString(p.Hands[1].Cards), HandValue(p.Hands[1].Cards)))
	default:
		return ActionResult{}, errors.New("未知行动")
	}
	if g.actionableCount() == 0 {
		g.finish()
		return ActionResult{Messages: append(messages, "庄家补牌并结算"), Finished: true}, nil
	}
	// 分牌后留在原玩家身上继续行动；其它行动转给下一手。
	if kind == ActionSplit && p.Hands[g.CurrentHand].Status == PlayerActive {
		return ActionResult{Messages: messages}, nil
	}
	g.CurrentTurn, g.CurrentHand = g.nextActionableAfter(idx, g.CurrentHand)
	return ActionResult{Messages: messages}, nil
}

func (g *Game) AutoAction() (ActionResult, error) {
	if g.Status != StatusRunning || g.CurrentTurn < 0 || g.CurrentTurn >= len(g.Players) {
		return ActionResult{}, errors.New("没有可超时处理的行动")
	}
	p := g.Players[g.CurrentTurn]
	return g.ApplyAction(p.UserID, ActionStand)
}

func (g *Game) CurrentPlayer() *Player {
	if g.Status != StatusRunning || g.CurrentTurn < 0 || g.CurrentTurn >= len(g.Players) {
		return nil
	}
	return &g.Players[g.CurrentTurn]
}

// CurrentActiveHand 返回当前行动的手牌，没有则返回 nil。
func (g *Game) CurrentActiveHand() *Hand {
	p := g.CurrentPlayer()
	if p == nil || g.CurrentHand < 0 || g.CurrentHand >= len(p.Hands) {
		return nil
	}
	return &p.Hands[g.CurrentHand]
}

func (g *Game) CanDoubleCurrent() bool {
	hand := g.CurrentActiveHand()
	return hand != nil && canDouble(hand)
}

func (g *Game) CanSplitCurrent() bool {
	p := g.CurrentPlayer()
	hand := g.CurrentActiveHand()
	return p != nil && hand != nil && canSplit(p, hand)
}

func (g *Game) finish() {
	dealerNatural := len(g.Dealer) == 2 && DealerValue(g.Dealer) == 21
	for DealerValue(g.Dealer) < 17 && hasStandingHand(g.Players) {
		card, err := g.draw()
		if err != nil {
			break
		}
		g.Dealer = append(g.Dealer, card)
	}
	dealerValue := DealerValue(g.Dealer)
	dealerBust := dealerValue > 21
	awards := make([]Award, 0, len(g.Players))
	for _, p := range g.Players {
		for _, hand := range p.Hands {
			value := HandValue(hand.Cards)
			award := Award{UserID: p.UserID, Bet: hand.Bet}
			switch {
			case hand.Status == PlayerBust || value > 21:
				award.Payout = 0
				award.Net = -hand.Bet
				award.Reason = "玩家爆牌"
			case hand.Status == PlayerBlackjack && dealerNatural:
				award.Payout = hand.Bet
				award.Net = 0
				award.Reason = "双方 Blackjack 平局退回"
			case hand.Status == PlayerBlackjack:
				award.Payout = hand.Bet + hand.Bet*3/2
				award.Net = award.Payout - hand.Bet
				award.Reason = "Blackjack"
			case dealerNatural:
				award.Payout = 0
				award.Net = -hand.Bet
				award.Reason = "庄家 Blackjack"
			case dealerBust:
				award.Payout = hand.Bet * 2
				award.Net = hand.Bet
				award.Reason = "庄家爆牌"
			case value > dealerValue:
				award.Payout = hand.Bet * 2
				award.Net = hand.Bet
				award.Reason = "点数大于庄家"
			case value == dealerValue:
				award.Payout = hand.Bet
				award.Net = 0
				award.Reason = "平局退回"
			default:
				award.Payout = 0
				award.Net = -hand.Bet
				award.Reason = "点数小于庄家"
			}
			awards = append(awards, award)
		}
	}
	g.Awards = awards
	g.Status = StatusFinished
	g.CurrentTurn = -1
	g.CurrentHand = -1
}

func HandValue(cards []poker.Card) int {
	total := 0
	aces := 0
	for _, card := range cards {
		point := cardPoint(card)
		total += point
		if point == 11 {
			aces++
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total
}

func cardPoint(card poker.Card) int {
	switch {
	case card.Rank == 14:
		return 11
	case card.Rank >= 10:
		return 10
	default:
		return card.Rank
	}
}

func DealerValue(cards []poker.Card) int {
	return HandValue(cards)
}

func (g *Game) DealerVisibleCards() []poker.Card {
	if g.Status == StatusRunning && len(g.Dealer) > 0 {
		return append([]poker.Card(nil), g.Dealer[:1]...)
	}
	return append([]poker.Card(nil), g.Dealer...)
}

func (g *Game) findPlayer(userID int64) int {
	for i, p := range g.Players {
		if p.UserID == userID {
			return i
		}
	}
	return -1
}

// nextActionableAfter 从 (playerIdx, handIdx) 之后找下一个待行动的手牌，
// 先看同一玩家的后续手牌，再轮转到其他玩家。
func (g *Game) nextActionableAfter(playerIdx, handIdx int) (int, int) {
	if len(g.Players) == 0 {
		return -1, -1
	}
	if playerIdx >= 0 && playerIdx < len(g.Players) {
		for h := handIdx + 1; h < len(g.Players[playerIdx].Hands); h++ {
			if g.Players[playerIdx].Hands[h].Status == PlayerActive {
				return playerIdx, h
			}
		}
	}
	for offset := 1; offset <= len(g.Players); offset++ {
		next := (playerIdx + offset) % len(g.Players)
		if next < 0 {
			next += len(g.Players)
		}
		for h, hand := range g.Players[next].Hands {
			if hand.Status == PlayerActive {
				return next, h
			}
		}
	}
	return -1, -1
}

func (g *Game) actionableCount() int {
	count := 0
	for _, p := range g.Players {
		for _, hand := range p.Hands {
			if hand.Status == PlayerActive {
				count++
			}
		}
	}
	return count
}

func (g *Game) draw() (poker.Card, error) {
	if len(g.Deck) == 0 {
		return poker.Card{}, errors.New("牌堆为空")
	}
	card := g.Deck[0]
	g.Deck = g.Deck[1:]
	return card, nil
}

func canDouble(hand *Hand) bool {
	return hand.Status == PlayerActive && len(hand.Cards) == 2 && !hand.Doubled
}

func canSplit(p *Player, hand *Hand) bool {
	return hand.Status == PlayerActive && len(p.Hands) == 1 && len(hand.Cards) == 2 &&
		cardPoint(hand.Cards[0]) == cardPoint(hand.Cards[1])
}

func handLabel(p *Player, handIdx int) string {
	if len(p.Hands) <= 1 {
		return p.Display
	}
	return fmt.Sprintf("%s 手牌%d", p.Display, handIdx+1)
}

func hasStandingHand(players []Player) bool {
	for _, p := range players {
		for _, hand := range p.Hands {
			if hand.Status == PlayerStand {
				return true
			}
		}
	}
	return false
}
