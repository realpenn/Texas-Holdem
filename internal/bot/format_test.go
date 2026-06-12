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

func TestRaiseRowUsesRaiseToPotSizing(t *testing.T) {
	game := &poker.Game{
		Settings:    poker.Settings{ID: "g1", SmallBlind: 50, BigBlind: 100},
		Status:      poker.StatusRunning,
		Street:      poker.StreetPreflop,
		Pot:         150,
		CurrentBet:  100,
		MinRaise:    100,
		CurrentTurn: 0,
		Players: []poker.Player{
			{UserID: 1, Display: "A", Status: poker.PlayerActive, Stack: 1000, CurrentBet: 0},
			{UserID: 2, Display: "B", Status: poker.PlayerActive, Stack: 900, CurrentBet: 100},
		},
	}
	row := raiseRow(game, game.CurrentPlayer())
	var got []string
	for _, button := range row {
		if button.CallbackData != nil {
			got = append(got, *button.CallbackData)
		}
	}
	want := []string{"act:g1:raise:200", "act:g1:raise:225", "act:g1:raise:350"}
	if len(got) != len(want) {
		t.Fatalf("raise callbacks = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("raise callbacks = %#v, want %#v", got, want)
		}
	}
}
