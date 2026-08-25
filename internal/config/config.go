package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken string
	WebhookSecret    string
	ListenAddr       string
	SubscribersFile  string
	BackendStatusURL string
	RequestTimeout   time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		TelegramBotToken: get("TELEGRAM_BOT_TOKEN", ""),
		WebhookSecret:    get("WEBHOOK_SECRET", ""),
		ListenAddr:       get("LISTEN_ADDR", ":8088"),
		SubscribersFile:  get("SUBSCRIBERS_FILE", "/data/subscribers.json"),
		BackendStatusURL: get("BACKEND_STATUS_URL", ""),
		RequestTimeout:   duration("REQUEST_TIMEOUT", 10*time.Second),
	}
	if cfg.TelegramBotToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.WebhookSecret == "" {
		return Config{}, fmt.Errorf("WEBHOOK_SECRET is required")
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
