package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"mos-sport-bot/internal/store"

	telebot "gopkg.in/telebot.v3"
)

type BroadcastResult struct {
	Sent   int
	Failed int
}

type backendStatus struct {
	State       string    `json:"state"`
	URL         string    `json:"url"`
	LastCheck   time.Time `json:"last_check"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastChange  time.Time `json:"last_change,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	Checks      uint64    `json:"checks"`
}

type Bot struct {
	bot        *telebot.Bot
	store      *store.Store
	statusURL  string
	httpClient *http.Client
	logger     *slog.Logger
}

func New(token string, st *store.Store, statusURL string, requestTimeout time.Duration, logger *slog.Logger) (*Bot, error) {
	tb, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}

	b := &Bot{
		bot:        tb,
		store:      st,
		statusURL:  statusURL,
		httpClient: &http.Client{Timeout: requestTimeout},
		logger:     logger,
	}

	tb.Handle("/start", b.handleStart)
	tb.Handle("/stop", b.handleStop)
	tb.Handle("/status", b.handleStatus)

	return b, nil
}

func (b *Bot) Run(ctx context.Context) {
	go b.bot.Start()
	<-ctx.Done()
	b.bot.Stop()
}

func (b *Bot) Broadcast(ctx context.Context, text string) BroadcastResult {
	var result BroadcastResult
	for _, id := range b.store.List() {
		if ctx.Err() != nil {
			return result
		}
		if _, err := b.bot.Send(telebot.ChatID(id), text); err != nil {
			b.logger.Error("send notification failed", "chat_id", id, "error", err)
			result.Failed++
			continue
		}
		result.Sent++
	}
	return result
}

func (b *Bot) handleStart(c telebot.Context) error {
	added, err := b.store.Add(c.Chat().ID)
	if err != nil {
		b.logger.Error("subscribe failed", "chat_id", c.Chat().ID, "error", err)
		return c.Send("⚠️ Не удалось сохранить подписку, попробуйте ещё раз.")
	}
	if !added {
		return c.Send("Вы уже подписаны.")
	}
	return c.Send("Вы подписались на уведомления о доступности записи.")
}

func (b *Bot) handleStop(c telebot.Context) error {
	removed, err := b.store.Remove(c.Chat().ID)
	if err != nil {
		b.logger.Error("unsubscribe failed", "chat_id", c.Chat().ID, "error", err)
		return c.Send("⚠️ Не удалось отменить подписку, попробуйте ещё раз.")
	}
	if !removed {
		return c.Send("Вы не были подписаны.")
	}
	return c.Send("Вы отписались от уведомлений.")
}

func (b *Bot) handleStatus(c telebot.Context) error {
	if b.statusURL == "" {
		return c.Send("⚠️ Команда /status не настроена (не задан BACKEND_STATUS_URL).")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, b.statusURL, nil)
	if err != nil {
		return c.Send(fmt.Sprintf("⚠️ Не удалось получить статус: %s", err))
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return c.Send(fmt.Sprintf("⚠️ Не удалось получить статус: %s", err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return c.Send(fmt.Sprintf("⚠️ Не удалось получить статус: сервер бэкенда вернул %d", resp.StatusCode))
	}
	var s backendStatus
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return c.Send(fmt.Sprintf("⚠️ Не удалось разобрать статус: %s", err))
	}
	return c.Send(formatStatus(s))
}

func formatStatus(s backendStatus) string {
	msg := fmt.Sprintf("Состояние: %s\nПоследняя проверка: %s", s.State, formatTime(s.LastCheck))
	if !s.LastChange.IsZero() {
		msg += fmt.Sprintf("\nПоследнее изменение: %s", formatTime(s.LastChange))
	}
	if s.LastError != "" {
		msg += fmt.Sprintf("\nПоследняя ошибка: %s", s.LastError)
	}
	return msg
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
