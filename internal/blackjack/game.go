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
)

type Settings struct {
	ID            string `json:"id"`
	ChatID        int64  `json:"chat_id"`
	CreatorID     int64  `json:"creator_id"`
	Bet           int64  `json:"bet"`
	WaitSeconds   int    `json:"wait_seconds"`
	ActionSeconds int    `json:"action_seconds"`
}

type Player struct {
	UserID  int64        `json:"user_id"`
	Display string       `json:"display"`
	Seat    int          `json:"seat"`
	Bet     int64        `json:"bet"`
	Status  string       `json:"status"`
	Hand    []poker.Card `json:"hand"`
}

type Game struct {
	Settings
	Status      string       `json:"status"`
	Players     []Player     `json:"players"`
	Deck        []poker.Card `json:"deck"`
	Dealer      []poker.Card `json:"dealer"`
	CurrentTurn int          `json:"current_turn"`
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
	ActionHit   ActionKind = "hit"
	ActionStand ActionKind = "stand"
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
		Bet:     g.Bet,
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
	if len(g.Players) < 1 {
		return errors.New("至少需要 1 人开局")
	}
	deck := poker.NewDeck()
	if err := poker.Shuffle(deck); err != nil {
		return err
	}
	g.Deck = deck
	g.Status = StatusRunning
	g.Dealer = nil
	for i := range g.Players {
		g.Players[i].Status = PlayerActive
		g.Players[i].Hand = nil
	}
	for cardNo := 0; cardNo < 2; cardNo++ {
		for i := range g.Players {
			card, err := g.draw()
			if err != nil {
				return err
			}
			g.Players[i].Hand = append(g.Players[i].Hand, card)
		}
		card, err := g.draw()
		if err != nil {
			return err
		}
		g.Dealer = append(g.Dealer, card)
	}
	for i := range g.Players {
		if HandValue(g.Players[i].Hand) == 21 {
			g.Players[i].Status = PlayerBlackjack
		}
	}
	if DealerValue(g.Dealer) == 21 || g.actionableCount() == 0 {
		g.finish()
		return nil
	}
	g.CurrentTurn = g.nextActionableAfter(-1)
	return nil
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
	if p.Status != PlayerActive {
		return ActionResult{}, errors.New("你当前不能行动")
	}
	var messages []string
	switch kind {
	case ActionHit:
		card, err := g.draw()
		if err != nil {
			return ActionResult{}, err
		}
		p.Hand = append(p.Hand, card)
		value := HandValue(p.Hand)
		if value > 21 {
			p.Status = PlayerBust
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，点数 %d，爆牌", p.Display, card.String(), value))
		} else if value == 21 {
			p.Status = PlayerStand
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，点数 21，自动停牌", p.Display, card.String()))
		} else {
			messages = append(messages, fmt.Sprintf("%s 要牌 %s，当前点数 %d", p.Display, card.String(), value))
		}
	case ActionStand:
		p.Status = PlayerStand
		messages = append(messages, fmt.Sprintf("%s 停牌，点数 %d", p.Display, HandValue(p.Hand)))
	default:
		return ActionResult{}, errors.New("未知行动")
	}
	if g.actionableCount() == 0 {
		g.finish()
		return ActionResult{Messages: append(messages, "庄家补牌并结算"), Finished: true}, nil
	}
	g.CurrentTurn = g.nextActionableAfter(idx)
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

func (g *Game) finish() {
	dealerNatural := len(g.Dealer) == 2 && DealerValue(g.Dealer) == 21
	for DealerValue(g.Dealer) < 17 && hasStandingPlayer(g.Players) {
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
		value := HandValue(p.Hand)
		award := Award{UserID: p.UserID, Bet: p.Bet}
		switch {
		case p.Status == PlayerBust || value > 21:
			award.Payout = 0
			award.Net = -p.Bet
			award.Reason = "玩家爆牌"
		case p.Status == PlayerBlackjack && dealerNatural:
			award.Payout = p.Bet
			award.Net = 0
			award.Reason = "双方 Blackjack 平局退回"
		case p.Status == PlayerBlackjack:
			award.Payout = p.Bet + p.Bet*3/2
			award.Net = award.Payout - p.Bet
			award.Reason = "Blackjack"
		case dealerBust:
			award.Payout = p.Bet * 2
			award.Net = p.Bet
			award.Reason = "庄家爆牌"
		case value > dealerValue:
			award.Payout = p.Bet * 2
			award.Net = p.Bet
			award.Reason = "点数大于庄家"
		case value == dealerValue:
			award.Payout = p.Bet
			award.Net = 0
			award.Reason = "平局退回"
		default:
			award.Payout = 0
			award.Net = -p.Bet
			award.Reason = "点数小于庄家"
		}
		awards = append(awards, award)
	}
	g.Awards = awards
	g.Status = StatusFinished
	g.CurrentTurn = -1
}

func HandValue(cards []poker.Card) int {
	total := 0
	aces := 0
	for _, card := range cards {
		switch {
		case card.Rank == 14:
			total += 11
			aces++
		case card.Rank >= 10:
			total += 10
		default:
			total += card.Rank
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total
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

func (g *Game) nextActionableAfter(idx int) int {
	if len(g.Players) == 0 {
		return -1
	}
	for offset := 1; offset <= len(g.Players); offset++ {
		next := (idx + offset) % len(g.Players)
		if g.Players[next].Status == PlayerActive {
			return next
		}
	}
	return -1
}

func (g *Game) actionableCount() int {
	count := 0
	for _, p := range g.Players {
		if p.Status == PlayerActive {
			count++
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

func hasStandingPlayer(players []Player) bool {
	for _, p := range players {
		if p.Status == PlayerStand {
			return true
		}
	}
	return false
}
