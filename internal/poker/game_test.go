package poker

import "testing"

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
