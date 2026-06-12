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
			{UserID: 1, Bet: 1000, Status: PlayerStand, Hand: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 8, Suit: 3}}},
			{UserID: 2, Bet: 1000, Status: PlayerStand, Hand: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 7, Suit: 3}}},
			{UserID: 3, Bet: 1000, Status: PlayerBust, Hand: []poker.Card{{Rank: 10, Suit: 2}, {Rank: 8, Suit: 3}, {Rank: 9, Suit: 1}}},
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
