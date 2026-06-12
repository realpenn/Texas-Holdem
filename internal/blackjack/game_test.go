package blackjack

import (
	"testing"

	"texas-holdem/internal/poker"
)

func TestHandValueTreatsAcesAsOneWhenNeeded(t *testing.T) {
	cards := []poker.Card{{Rank: 14, Suit: 0}, {Rank: 9, Suit: 1}, {Rank: 5, Suit: 2}}
	if got := HandValue(cards); got != 15 {
		t.Fatalf("HandValue() = %d, want 15", got)
	}
}

func TestFinishSettlesPushWinAndBust(t *testing.T) {
	game := &Game{
		Settings: Settings{Bet: 1000},
		Status:   StatusRunning,
		Dealer:   []poker.Card{{Rank: 10, Suit: 0}, {Rank: 7, Suit: 1}},
		Players: []Player{
			{UserID: 1, Hands: []Hand{{Bet: 1000, Status: PlayerStand, Cards: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 8, Suit: 3}}}}},
			{UserID: 2, Hands: []Hand{{Bet: 1000, Status: PlayerStand, Cards: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 7, Suit: 3}}}}},
			{UserID: 3, Hands: []Hand{{Bet: 1000, Status: PlayerBust, Cards: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 8, Suit: 3}, {Rank: 9, Suit: 1}}}}},
		},
	}
	game.finish()
	if game.Awards[0].Payout != 2000 || game.Awards[0].Net != 1000 {
		t.Fatalf("expected player 1 win, got %#v", game.Awards[0])
	}
	if game.Awards[1].Payout != 1000 || game.Awards[1].Net != 0 {
		t.Fatalf("expected player 2 push, got %#v", game.Awards[1])
	}
	if game.Awards[2].Payout != 0 || game.Awards[2].Net != -1000 {
		t.Fatalf("expected player 3 loss, got %#v", game.Awards[2])
	}
}

func TestNaturalBlackjackBeatsMultiCardDealer21(t *testing.T) {
	game := &Game{
		Settings: Settings{Bet: 1000},
		Status:   StatusRunning,
		Dealer:   []poker.Card{{Rank: 5, Suit: 0}, {Rank: 6, Suit: 1}, {Rank: 10, Suit: 2}},
		Players: []Player{
			{UserID: 1, Hands: []Hand{{Bet: 1000, Status: PlayerBlackjack, Cards: []poker.Card{{Rank: 14, Suit: 2}, {Rank: 13, Suit: 3}}}}},
		},
	}
	game.finish()
	if game.Awards[0].Payout != 2500 || game.Awards[0].Net != 1500 {
		t.Fatalf("expected 3:2 blackjack payout, got %#v", game.Awards[0])
	}
}

func TestDoubleDoublesBetAndStands(t *testing.T) {
	game := &Game{
		Settings:    Settings{Bet: 1000},
		Status:      StatusRunning,
		CurrentTurn: 0,
		CurrentHand: 0,
		Deck:        []poker.Card{{Rank: 9, Suit: 0}, {Rank: 10, Suit: 1}, {Rank: 7, Suit: 2}},
		Dealer:      []poker.Card{{Rank: 10, Suit: 0}, {Rank: 8, Suit: 1}},
		Players: []Player{
			{UserID: 1, Hands: []Hand{{Bet: 1000, Status: PlayerActive, Cards: []poker.Card{{Rank: 5, Suit: 2}, {Rank: 6, Suit: 3}}}}},
		},
	}
	if cost := game.ExtraBetCost(1, ActionDouble); cost != 1000 {
		t.Fatalf("ExtraBetCost = %d, want 1000", cost)
	}
	result, err := game.ApplyAction(1, ActionDouble)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Finished {
		t.Fatal("expected game to finish after the only player doubles")
	}
	// 5+6+9=20 vs dealer 18: doubled bet wins 1:1.
	if game.Awards[0].Bet != 2000 || game.Awards[0].Payout != 4000 || game.Awards[0].Net != 2000 {
		t.Fatalf("unexpected double award: %#v", game.Awards[0])
	}
}

func TestSplitCreatesTwoHands(t *testing.T) {
	game := &Game{
		Settings:    Settings{Bet: 1000},
		Status:      StatusRunning,
		CurrentTurn: 0,
		CurrentHand: 0,
		Deck:        []poker.Card{{Rank: 5, Suit: 0}, {Rank: 6, Suit: 1}, {Rank: 10, Suit: 2}, {Rank: 10, Suit: 3}, {Rank: 9, Suit: 0}},
		Dealer:      []poker.Card{{Rank: 10, Suit: 0}, {Rank: 8, Suit: 1}},
		Players: []Player{
			{UserID: 1, Hands: []Hand{{Bet: 1000, Status: PlayerActive, Cards: []poker.Card{{Rank: 8, Suit: 2}, {Rank: 8, Suit: 3}}}}},
		},
	}
	if _, err := game.ApplyAction(1, ActionSplit); err != nil {
		t.Fatal(err)
	}
	p := game.Players[0]
	if len(p.Hands) != 2 {
		t.Fatalf("expected 2 hands, got %d", len(p.Hands))
	}
	if p.Hands[0].Bet != 1000 || p.Hands[1].Bet != 1000 {
		t.Fatalf("expected each hand to keep the original bet, got %d/%d", p.Hands[0].Bet, p.Hands[1].Bet)
	}
	if len(p.Hands[0].Cards) != 2 || len(p.Hands[1].Cards) != 2 {
		t.Fatalf("expected each hand to draw one card")
	}
	if game.CurrentTurn != 0 || game.CurrentHand != 0 {
		t.Fatalf("expected turn to stay on first split hand, got %d/%d", game.CurrentTurn, game.CurrentHand)
	}
}

func TestSplitAcesGetOneCardAndStand(t *testing.T) {
	game := &Game{
		Settings:    Settings{Bet: 1000},
		Status:      StatusRunning,
		CurrentTurn: 0,
		CurrentHand: 0,
		Deck:        []poker.Card{{Rank: 9, Suit: 0}, {Rank: 8, Suit: 1}, {Rank: 2, Suit: 2}},
		Dealer:      []poker.Card{{Rank: 10, Suit: 0}, {Rank: 9, Suit: 1}},
		Players: []Player{
			{UserID: 1, Hands: []Hand{{Bet: 1000, Status: PlayerActive, Cards: []poker.Card{{Rank: 14, Suit: 2}, {Rank: 14, Suit: 3}}}}},
		},
	}
	result, err := game.ApplyAction(1, ActionSplit)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Finished {
		t.Fatal("expected game to finish: both split-ace hands auto-stand")
	}
	for i, hand := range game.Players[0].Hands {
		if len(hand.Cards) != 2 || hand.Status != PlayerStand {
			t.Fatalf("hand %d: expected one extra card then stand, got %#v", i, hand)
		}
	}
}

func TestStartUsesMultipleDecks(t *testing.T) {
	game := NewGame(Settings{Bet: 1000})
	if err := game.AddPlayer(1, "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := game.Start(); err != nil {
		t.Fatal(err)
	}
	// 4 副牌 208 张，发出 4 张后牌堆应剩 204 张。
	if want := 52*DeckCount - 4; len(game.Deck) != want {
		t.Fatalf("deck size = %d, want %d", len(game.Deck), want)
	}
}
