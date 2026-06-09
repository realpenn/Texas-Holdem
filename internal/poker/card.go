package poker

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

type Card struct {
	Rank int `json:"rank"`
	Suit int `json:"suit"`
}

var rankNames = map[int]string{
	2: "2", 3: "3", 4: "4", 5: "5", 6: "6", 7: "7", 8: "8", 9: "9",
	10: "10", 11: "J", 12: "Q", 13: "K", 14: "A",
}

var suitNames = map[int]string{
	0: "♣️",
	1: "♦️",
	2: "♥️",
	3: "♠️",
}

var parseSuitNames = map[byte]int{
	'C': 0,
	'D': 1,
	'H': 2,
	'S': 3,
}

func NewDeck() []Card {
	deck := make([]Card, 0, 52)
	for suit := 0; suit < 4; suit++ {
		for rank := 2; rank <= 14; rank++ {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return deck
}

func Shuffle(deck []Card) error {
	for i := len(deck) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(n.Int64())
		deck[i], deck[j] = deck[j], deck[i]
	}
	return nil
}

func (c Card) String() string {
	return suitNames[c.Suit] + rankNames[c.Rank]
}

func CardsString(cards []Card) string {
	parts := make([]string, 0, len(cards))
	for _, c := range cards {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, " ")
}

func ParseCard(raw string) (Card, error) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if len(raw) != 2 {
		return Card{}, fmt.Errorf("invalid card %q", raw)
	}
	var rank int
	switch raw[0] {
	case '2', '3', '4', '5', '6', '7', '8', '9':
		rank = int(raw[0] - '0')
	case 'T':
		rank = 10
	case 'J':
		rank = 11
	case 'Q':
		rank = 12
	case 'K':
		rank = 13
	case 'A':
		rank = 14
	default:
		return Card{}, fmt.Errorf("invalid rank %q", raw)
	}
	suit, ok := parseSuitNames[raw[1]]
	if !ok {
		return Card{}, fmt.Errorf("invalid suit %q", raw)
	}
	return Card{Rank: rank, Suit: suit}, nil
}
