package poker

import (
	"errors"
	"fmt"
	"testing"
)

func TestShowdownReturnsUncalledBetWithoutRake(t *testing.T) {
	game := &Game{
		Settings: Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, RakePercent: 3, RakeCap: 300},
		Status:   StatusRunning,
		Street:   StreetRiver,
		Board:    mustCards(t, "2C", "7D", "9H", "JS", "QC"),
		Pot:      1500,
		Players: []Player{
			{UserID: 1, Display: "A", Status: PlayerActive, Hole: mustCards(t, "AS", "AD"), TotalBet: 1000},
			{UserID: 2, Display: "B", Status: PlayerAllIn, Hole: mustCards(t, "KS", "KD"), TotalBet: 500},
		},
	}
	awards, err := game.finishShowdown()
	if err != nil {
		t.Fatal(err)
	}
	if len(awards) != 2 {
		t.Fatalf("expected main pot award and uncalled return, got %#v", awards)
	}
	if awards[0].Gross != 1000 || awards[0].Fee != 30 || awards[0].Net != 970 {
		t.Fatalf("unexpected main pot award: %#v", awards[0])
	}
	if awards[1].Gross != 500 || awards[1].Fee != 0 || awards[1].Net != 500 || awards[1].Reason != "未被跟注退回" {
		t.Fatalf("unexpected uncalled return: %#v", awards[1])
	}
}

func TestSidePotWithFoldedContributionIsRaked(t *testing.T) {
	game := &Game{
		Settings: Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, RakePercent: 3, RakeCap: 300},
		Status:   StatusRunning,
		Street:   StreetRiver,
		Board:    mustCards(t, "2C", "7D", "9H", "JS", "QC"),
		Pot:      2500,
		Players: []Player{
			{UserID: 1, Display: "A", Status: PlayerActive, Hole: mustCards(t, "AS", "AD"), TotalBet: 1000},
			{UserID: 2, Display: "B", Status: PlayerAllIn, Hole: mustCards(t, "KS", "KD"), TotalBet: 500},
			{UserID: 3, Display: "C", Status: PlayerFolded, Hole: mustCards(t, "3S", "3D"), TotalBet: 1000},
		},
	}
	awards, err := game.finishShowdown()
	if err != nil {
		t.Fatal(err)
	}
	if len(awards) != 2 {
		t.Fatalf("expected main and side pot awards, got %#v", awards)
	}
	if awards[0].Gross != 1500 || awards[0].Fee != 45 {
		t.Fatalf("unexpected main pot: %#v", awards[0])
	}
	if awards[1].Gross != 1000 || awards[1].Fee != 30 || awards[1].Reason != "摊牌胜出" {
		t.Fatalf("folded contribution side pot should be won and raked: %#v", awards[1])
	}
}

func TestRakeCapAppliesAcrossAwards(t *testing.T) {
	game := &Game{
		Settings: Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, RakePercent: 10, RakeCap: 50},
		Status:   StatusRunning,
		Street:   StreetRiver,
		Board:    mustCards(t, "2C", "7D", "9H", "JS", "QC"),
		Pot:      2000,
		Players: []Player{
			{UserID: 1, Display: "A", Status: PlayerActive, Hole: mustCards(t, "AS", "AD"), TotalBet: 1000},
			{UserID: 2, Display: "B", Status: PlayerActive, Hole: mustCards(t, "KS", "KD"), TotalBet: 1000},
		},
	}
	awards, err := game.finishShowdown()
	if err != nil {
		t.Fatal(err)
	}
	if len(awards) != 1 {
		t.Fatalf("expected one winner, got %#v", awards)
	}
	if awards[0].Fee != 50 || awards[0].Net != 1950 {
		t.Fatalf("rake cap not applied: %#v", awards[0])
	}
}

func TestUncontestedWinDoesNotRakeUncalledBet(t *testing.T) {
	game := &Game{
		Settings: Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, RakePercent: 3, RakeCap: 300},
		Status:   StatusRunning,
		Street:   StreetPreflop,
		Pot:      5150,
		Players: []Player{
			{UserID: 1, Display: "A", Status: PlayerActive, TotalBet: 5000},
			{UserID: 2, Display: "B", Status: PlayerFolded, TotalBet: 50},
			{UserID: 3, Display: "C", Status: PlayerFolded, TotalBet: 100},
		},
	}
	awards := game.finishUncontested()
	if len(awards) != 1 {
		t.Fatalf("expected one award, got %#v", awards)
	}
	// 被跟到的部分只有 50+100+100=250，4900 未被跟注的部分不抽水。
	wantFee := int64(250 * 3 / 100)
	if awards[0].Gross != 5150 || awards[0].Fee != wantFee {
		t.Fatalf("uncalled portion should not be raked: %#v", awards[0])
	}
	if !game.BetweenHands() {
		t.Fatal("hand end should keep the table running between hands")
	}
}

func TestShortAllInRaiseDoesNotReopenBetting(t *testing.T) {
	game := &Game{
		Settings:    Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, RakePercent: 0},
		Status:      StatusRunning,
		Street:      StreetFlop,
		CurrentTurn: 1,
		CurrentBet:  1000,
		MinRaise:    1000,
		Players: []Player{
			{UserID: 1, Display: "A", Status: PlayerActive, Stack: 5000, CurrentBet: 1000, TotalBet: 1000, HasActed: true},
			{UserID: 2, Display: "B", Status: PlayerActive, Stack: 1400, CurrentBet: 0, TotalBet: 0},
			{UserID: 3, Display: "C", Status: PlayerActive, Stack: 5000, CurrentBet: 1000, TotalBet: 1000, HasActed: true},
		},
	}
	// B 用 raise 全押到 1400，不足最小加注 2000，不应重开 A/C 的加注权。
	if _, err := game.ApplyAction(2, ActionRaise, 1400); err != nil {
		t.Fatal(err)
	}
	if !game.Players[0].HasActed || !game.Players[2].HasActed {
		t.Fatal("short all-in raise must not reset HasActed for players who already acted")
	}
	if game.CurrentBet != 1400 || game.MinRaise != 1000 {
		t.Fatalf("unexpected bet state: currentBet=%d minRaise=%d", game.CurrentBet, game.MinRaise)
	}
}

func TestMultiHandRotationAndBrokeRemoval(t *testing.T) {
	game := NewGame(Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, BuyIn: 1000})
	for i := int64(1); i <= 3; i++ {
		if err := game.AddPlayer(i, fmt.Sprintf("P%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := game.Start(); err != nil {
		t.Fatal(err)
	}
	if game.HandNo != 1 {
		t.Fatalf("HandNo = %d, want 1", game.HandNo)
	}
	prevDealerUser := game.Players[game.Dealer].UserID
	// 人为结束本手：P3 输光。
	game.Street = StreetDone
	game.CurrentTurn = -1
	for i := range game.Players {
		game.Players[i].Stack = 1000
	}
	game.Players[2].Stack = 0
	removed, err := game.StartNextHand()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].UserID != 3 {
		t.Fatalf("expected P3 removed, got %#v", removed)
	}
	if game.HandNo != 2 || len(game.Players) != 2 {
		t.Fatalf("unexpected next hand state: handNo=%d players=%d", game.HandNo, len(game.Players))
	}
	if game.Players[game.Dealer].UserID == prevDealerUser && game.PlayersWithChips() > 1 {
		// 庄位必须移交给下一个有筹码的玩家（P3 输光时可能跳过）。
		next := (game.Dealer + 1) % len(game.Players)
		t.Fatalf("dealer did not rotate: dealer=%d next=%d prevUser=%d", game.Dealer, next, prevDealerUser)
	}
}

func TestStartNextHandClosesWhenOnePlayerLeft(t *testing.T) {
	game := NewGame(Settings{ID: "g1", ChatID: 1, SmallBlind: 50, BigBlind: 100, BuyIn: 1000})
	_ = game.AddPlayer(1, "A")
	_ = game.AddPlayer(2, "B")
	if err := game.Start(); err != nil {
		t.Fatal(err)
	}
	game.Street = StreetDone
	game.CurrentTurn = -1
	game.Players[0].Stack = 2000
	game.Players[1].Stack = 0
	if _, err := game.StartNextHand(); !errors.Is(err, ErrNotEnoughPlayers) {
		t.Fatalf("expected ErrNotEnoughPlayers, got %v", err)
	}
}
