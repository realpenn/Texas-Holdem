package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken         string
	AdminUserIDs     map[int64]bool
	AllowedChatIDs   map[int64]bool
	SQLitePath       string
	SmallBlind       int64
	BigBlind         int64
	BuyIn            int64
	InitialBalance   int64
	DailyCheckin     int64
	WaitSeconds      int
	ActionSeconds    int
	RakePercent      int64
	RakeCapBigBlinds int64
	Location         *time.Location
}

func Load() (Config, error) {
	_ = loadDotEnv(".env")
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		BotToken:         strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		AdminUserIDs:     parseIntSet(os.Getenv("ADMIN_USER_IDS")),
		AllowedChatIDs:   parseIntSet(os.Getenv("ALLOWED_CHAT_IDS")),
		SQLitePath:       envString("SQLITE_PATH", "./texas_holdem.db"),
		SmallBlind:       envInt64("DEFAULT_SMALL_BLIND", 50),
		BigBlind:         envInt64("DEFAULT_BIG_BLIND", 1000),
		BuyIn:            envInt64("DEFAULT_BUY_IN", 10000),
		InitialBalance:   envInt64("DEFAULT_INITIAL_BALANCE", 10000),
		DailyCheckin:     envInt64("DEFAULT_DAILY_CHECKIN", 1000),
		WaitSeconds:      int(envInt64("DEFAULT_WAIT_SECONDS", 300)),
		ActionSeconds:    int(envInt64("DEFAULT_ACTION_SECONDS", 60)),
		RakePercent:      envInt64("RAKE_PERCENT", 3),
		RakeCapBigBlinds: envInt64("RAKE_CAP_BIG_BLINDS", 3),
		Location:         loc,
	}
	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.SmallBlind <= 0 || cfg.BigBlind <= cfg.SmallBlind {
		return Config{}, fmt.Errorf("blind defaults must satisfy 0 < small_blind < big_blind")
	}
	if cfg.BuyIn < cfg.BigBlind {
		return Config{}, fmt.Errorf("buy-in must be at least one big blind")
	}
	if cfg.DailyCheckin < 100 {
		return Config{}, fmt.Errorf("DEFAULT_DAILY_CHECKIN must be at least 100")
	}
	return cfg, nil
}

func (c Config) IsAdmin(userID int64) bool {
	return c.AdminUserIDs[userID]
}

func (c Config) IsAllowedChat(chatID int64) bool {
	if len(c.AllowedChatIDs) == 0 {
		return true
	}
	return c.AllowedChatIDs[chatID]
}

func (c Config) RakeCap() int64 {
	return c.BigBlind * c.RakeCapBigBlinds
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func parseIntSet(raw string) map[int64]bool {
	out := make(map[int64]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseInt(part, 10, 64)
		if err == nil {
			out[v] = true
		}
	}
	return out
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}
