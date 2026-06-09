package bot

import (
	"strings"
	"testing"

	"texas-holdem/internal/poker"
)

func TestSettlementTextShowsEveryPlayerHoleCards(t *testing.T) {
	game := &poker.Game{
		Board: []poker.Card{
			{Rank: 2, Suit: 0},
			{Rank: 7, Suit: 1},
			{Rank: 9, Suit: 2},
			{Rank: 11, Suit: 3},
			{Rank: 14, Suit: 0},
		},
		Players: []poker.Player{
			{UserID: 1, Display: "Alice", Status: poker.PlayerActive, Hole: []poker.Card{{Rank: 14, Suit: 3}, {Rank: 13, Suit: 2}}},
			{UserID: 2, Display: "Bob", Status: poker.PlayerFolded, Hole: []poker.Card{{Rank: 10, Suit: 1}, {Rank: 10, Suit: 0}}},
		},
		Awards: []poker.Award{
			{UserID: 1, Gross: 1000, Fee: 30, Net: 970, HandCategory: "一对"},
		},
	}
	text := settlementText(game)
	for _, want := range []string{
		"玩家手牌：",
		"Alice：♠️A ♥️K（在局）",
		"Bob：♦️10 ♣️10（弃牌）",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("settlement text missing %q:\n%s", want, text)
		}
	}
}
