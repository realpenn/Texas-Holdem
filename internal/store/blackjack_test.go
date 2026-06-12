package store

import (
	"context"
	"testing"
	"time"

	"texas-holdem/internal/blackjack"
)

func TestBlackjackGamePersistsAndSettles(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	game := blackjack.NewGame(blackjack.Settings{
		ID:            "bj1",
		ChatID:        -100,
		CreatorID:     1,
		Bet:           1000,
		WaitSeconds:   300,
		ActionSeconds: 60,
	})
	if err := game.AddPlayer(1, "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateWaitingBlackjack(ctx, game, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, err := st.ActiveGame(ctx, -100)
	if err != nil {
		t.Fatal(err)
	}
	if active.Type != GameTypeBlackjack || active.Blackjack == nil || active.Blackjack.ID != "bj1" {
		t.Fatalf("unexpected active game: %#v", active)
	}

	game.Awards = []blackjack.Award{{UserID: 1, Bet: 1000, Payout: 2000, Net: 1000, Reason: "测试获胜"}}
	game.Status = blackjack.StatusFinished
	if err := st.BeginBlackjack(ctx, game, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishBlackjack(ctx, game, 0, 0); err != nil {
		t.Fatal(err)
	}
	balance, err := st.Balance(ctx, -100, 1)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 11000 {
		t.Fatalf("balance = %d, want 11000", balance)
	}
}
