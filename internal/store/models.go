package store

import (
	"errors"
	"time"

	"texas-holdem/internal/blackjack"
	"texas-holdem/internal/poker"
)

var ErrNotFound = errors.New("not found")

type ActiveGame struct {
	Type             string
	Game             *poker.Game
	Blackjack        *blackjack.Game
	WaitingMessageID int
	ActionDeadline   time.Time
}

type RechargeCode struct {
	Code      string
	Amount    int64
	MaxUses   int
	UsedCount int
	ExpiresAt *time.Time
	CreatedBy int64
	Status    string
}
