package poker

import "testing"

func TestCardsStringUsesChineseDisplay(t *testing.T) {
	cards := mustCards(t, "AS", "TD", "3C", "QH")
	got := CardsString(cards)
	want := "♠️A ♦️10 ♣️3 ♥️Q"
	if got != want {
		t.Fatalf("CardsString() = %q, want %q", got, want)
	}
}
