package poker

import "testing"

func mustCards(t *testing.T, raw ...string) []Card {
	t.Helper()
	cards := make([]Card, 0, len(raw))
	for _, item := range raw {
		card, err := ParseCard(item)
		if err != nil {
			t.Fatal(err)
		}
		cards = append(cards, card)
	}
	return cards
}

func TestEvaluateStraightFlushBeatsQuads(t *testing.T) {
	sf, err := Evaluate(mustCards(t, "AS", "KS", "QS", "JS", "TS", "2D", "3C"))
	if err != nil {
		t.Fatal(err)
	}
	quads, err := Evaluate(mustCards(t, "AH", "AD", "AC", "AS", "KD", "2C", "3D"))
	if err != nil {
		t.Fatal(err)
	}
	if sf.Value <= quads.Value {
		t.Fatalf("straight flush should beat quads: sf=%v quads=%v", sf, quads)
	}
}

func TestEvaluateWheelStraight(t *testing.T) {
	rank, err := Evaluate(mustCards(t, "AS", "2D", "3H", "4C", "5S", "KD", "QC"))
	if err != nil {
		t.Fatal(err)
	}
	if rank.Category != "顺子" {
		t.Fatalf("expected wheel straight, got %s", rank.Category)
	}
}
