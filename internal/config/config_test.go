package config

import (
	"os"
	"testing"
)

func TestLoadDefaultsMatchExample(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("BOT_TOKEN", "test-token")
	for _, key := range []string{
		"ADMIN_USER_IDS",
		"ALLOWED_CHAT_IDS",
		"SQLITE_PATH",
		"DEFAULT_SMALL_BLIND",
		"DEFAULT_BIG_BLIND",
		"DEFAULT_BUY_IN",
		"DEFAULT_INITIAL_BALANCE",
		"DEFAULT_DAILY_CHECKIN",
		"DEFAULT_WAIT_SECONDS",
		"DEFAULT_ACTION_SECONDS",
		"RAKE_PERCENT",
		"RAKE_CAP_BIG_BLINDS",
	} {
		_ = os.Unsetenv(key)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.SmallBlind != 50 ||
		cfg.BigBlind != 1000 ||
		cfg.BuyIn != 10000 ||
		cfg.InitialBalance != 10000 ||
		cfg.DailyCheckin != 1000 ||
		cfg.WaitSeconds != 300 ||
		cfg.ActionSeconds != 60 ||
		cfg.RakePercent != 3 ||
		cfg.RakeCapBigBlinds != 3 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}
