package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken string
	WebhookSecret    string
	ListenAddr       string
	BackendAPIURL    string
	RequestTimeout   time.Duration
	AdminChatIDs     []int64
}

func Load() (Config, error) {
	cfg := Config{
		TelegramBotToken: get("TELEGRAM_BOT_TOKEN", ""),
		WebhookSecret:    get("WEBHOOK_SECRET", ""),
		ListenAddr:       get("LISTEN_ADDR", ":8088"),
		BackendAPIURL:    get("BACKEND_API_URL", ""),
		RequestTimeout:   duration("REQUEST_TIMEOUT", 10*time.Second),
		AdminChatIDs:     chatIDList("ADMIN_CHAT_IDS"),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.WebhookSecret == "" {
		return Config{}, fmt.Errorf("WEBHOOK_SECRET is required")
	}
	if cfg.BackendAPIURL == "" {
		return Config{}, fmt.Errorf("BACKEND_API_URL is required")
	}
	return cfg, nil
}

func get(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(get(key, ""))
	if err == nil && v > 0 {
		return v
	}
	return fallback
}

// chatIDList parses a comma-separated list of chat IDs (e.g. "111,-222").
// Unparseable entries are skipped rather than failing startup.
func chatIDList(key string) []int64 {
	var ids []int64
	for _, part := range strings.Split(get(key, ""), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
