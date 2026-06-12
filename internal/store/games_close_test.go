package store

import (
	"context"
	"testing"
	"time"

	"texas-holdem/internal/poker"
)

func TestCloseGameRefundsStacks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	game := poker.NewGame(poker.Settings{
		ID: "g1", ChatID: -200, CreatorID: 1,
		SmallBlind: 50, BigBlind: 100, BuyIn: 1000,
		WaitSeconds: 300, ActionSeconds: 60,
	})
	if err := game.AddPlayer(1, "A"); err != nil {
		t.Fatal(err)
	}
	if err := game.AddPlayer(2, "B"); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateWaitingGame(ctx, game, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginGame(ctx, game, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	// 买入后余额各减 1000。
	for _, userID := range []int64{1, 2} {
		bal, err := st.Balance(ctx, -200, userID)
		if err != nil {
			t.Fatal(err)
		}
		if bal != 9000 {
			t.Fatalf("user %d balance after buy-in = %d, want 9000", userID, bal)
		}
	}
	// 模拟一手结束：A 赢 300（抽水 9），B 剩 700。
	game.Players[0].Stack = 1291
	game.Players[1].Stack = 700
	game.Awards = []poker.Award{{UserID: 1, Gross: 300, Fee: 9, Net: 291, Reason: "摊牌胜出"}}
	game.Street = poker.StreetDone
	if err := st.SettleHand(ctx, game, time.Time{}); err != nil {
		t.Fatal(err)
	}
	game.Close()
	if err := st.CloseGame(ctx, game); err != nil {
		t.Fatal(err)
	}
	balA, _ := st.Balance(ctx, -200, 1)
	balB, _ := st.Balance(ctx, -200, 2)
	if balA != 9000+1291 {
		t.Fatalf("A balance = %d, want %d", balA, 9000+1291)
	}
	if balB != 9000+700 {
		t.Fatalf("B balance = %d, want %d", balB, 9000+700)
	}
}
