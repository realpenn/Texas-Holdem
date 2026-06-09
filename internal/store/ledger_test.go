package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), 10000, 500)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCheckinOncePerLocalDate(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	bal, amount, err := st.Checkin(ctx, -100, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if amount < 100 || amount > 500 {
		t.Fatalf("checkin amount out of range: %d", amount)
	}
	if bal != 10000+amount {
		t.Fatalf("unexpected checkin result balance=%d amount=%d", bal, amount)
	}
	if _, _, err = st.Checkin(ctx, -100, 1, now); err == nil {
		t.Fatal("expected duplicate checkin error")
	}
}

func TestRedeemCodeCannotBeRedeemedTwiceBySameUser(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.CreateRechargeCode(ctx, "ABC", 1200, 2, nil, 99); err != nil {
		t.Fatal(err)
	}
	bal, amount, err := st.Redeem(ctx, -100, 1, "ABC", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if amount != 1200 || bal != 11200 {
		t.Fatalf("unexpected redeem result balance=%d amount=%d", bal, amount)
	}
	if _, _, err = st.Redeem(ctx, -100, 1, "ABC", time.Now()); err == nil {
		t.Fatal("expected duplicate redemption error")
	}
	code, err := st.RechargeCode(ctx, "ABC")
	if err != nil {
		t.Fatal(err)
	}
	if code.UsedCount != 1 {
		t.Fatalf("used count should remain 1, got %d", code.UsedCount)
	}
}
