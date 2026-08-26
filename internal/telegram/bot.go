package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	telebot "gopkg.in/telebot.v3"
)

const helpText = "Слежу за доступностью записи на секциях sport.mos.ru.\n\n" +
	"/sub <url> — подписаться на URL\n" +
	"/unsub <url или номер из /list> — отписаться\n" +
	"/list — показать мои подписки (с номерами)\n" +
	"/status — статус по моим подпискам"

var botCommands = []telebot.Command{
	{Text: "sub", Description: "подписаться: /sub <url>"},
	{Text: "unsub", Description: "отписаться: /unsub <url или номер>"},
	{Text: "list", Description: "показать мои подписки с номерами"},
	{Text: "status", Description: "статус по моим подпискам"},
}

type BroadcastResult struct {
	Sent   int
	Failed int
}

type subscribeRequest struct {
	ChatID int64  `json:"chat_id"`
	URL    string `json:"url"`
}

type subscriptionsResponse struct {
	URLs []string `json:"urls"`
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

type statusResponse struct {
	Statuses []backendStatus `json:"statuses"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type Bot struct {
	bot        *telebot.Bot
	apiBaseURL string
	secret     string
	httpClient *http.Client
	logger     *slog.Logger
}

func New(token, apiBaseURL, secret string, requestTimeout time.Duration, logger *slog.Logger) (*Bot, error) {
	tb, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}

	b := &Bot{
		bot:        tb,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		secret:     secret,
		httpClient: &http.Client{Timeout: requestTimeout},
		logger:     logger,
	}

	tb.Handle("/start", b.handleStart)
	tb.Handle("/sub", b.handleSubscribe)
	tb.Handle("/unsub", b.handleUnsubscribe)
	tb.Handle("/list", b.handleList)
	tb.Handle("/status", b.handleStatus)

	if err := tb.SetCommands(botCommands); err != nil {
		logger.Warn("setting bot command menu failed", "error", err)
	}

	return b, nil
}

func (b *Bot) Run(ctx context.Context) {
	go b.bot.Start()
	<-ctx.Done()
	b.bot.Stop()
}

// SendTo messages exactly the given chat IDs, supplied by the caller (the
// webhook handler, on the backend's behalf) rather than any local state.
func (b *Bot) SendTo(ctx context.Context, chatIDs []int64, text string) BroadcastResult {
	var result BroadcastResult
	for _, id := range chatIDs {
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
	return c.Send(helpText)
}

func (b *Bot) handleSubscribe(c telebot.Context) error {
	args := c.Args()
	if len(args) != 1 {
		return c.Send("Укажите ссылку: /sub <url>")
	}
	url := args[0]
	status, body, err := b.callAPI(context.Background(), http.MethodPost, "/subscriptions", subscribeRequest{ChatID: c.Chat().ID, URL: url})
	if err != nil {
		b.logger.Error("subscribe call failed", "chat_id", c.Chat().ID, "url", url, "error", err)
	}
	return c.Send(subscribeReplyText(status, body, err, url))
}

func (b *Bot) handleUnsubscribe(c telebot.Context) error {
	args := c.Args()
	if len(args) != 1 {
		return c.Send("Укажите ссылку или номер из /list: /unsub <url|номер>")
	}

	url, resolveErr := b.resolveUnsubscribeTarget(context.Background(), c.Chat().ID, args[0])
	if resolveErr != nil {
		return c.Send(resolveErr.Error())
	}

	status, _, err := b.callAPI(context.Background(), http.MethodDelete, "/subscriptions", subscribeRequest{ChatID: c.Chat().ID, URL: url})
	if err != nil {
		b.logger.Error("unsubscribe call failed", "chat_id", c.Chat().ID, "url", url, "error", err)
	}
	return c.Send(unsubscribeReplyText(status, err, url))
}

// resolveUnsubscribeTarget turns arg into a URL to unsubscribe from. If arg
// isn't a number, it's already a URL. If it is a number, it's a 1-based
// index into the chat's current /list ordering (backend returns it sorted by
// URL, so the numbering matches what the user just saw in /list, as long as
// their subscriptions haven't changed in between).
func (b *Bot) resolveUnsubscribeTarget(ctx context.Context, chatID int64, arg string) (string, error) {
	n, convErr := strconv.Atoi(arg)
	if convErr != nil {
		return arg, nil
	}

	status, body, err := b.callAPI(ctx, http.MethodGet, fmt.Sprintf("/subscriptions?chat_id=%d", chatID), nil)
	if err != nil || status != http.StatusOK {
		return "", fmt.Errorf("⚠️ Не удалось получить список подписок, попробуйте позже.")
	}
	var resp subscriptionsResponse
	if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
		return "", fmt.Errorf("⚠️ Не удалось разобрать ответ бэкенда.")
	}
	return urlByIndex(resp.URLs, n)
}

func urlByIndex(urls []string, n int) (string, error) {
	if n < 1 || n > len(urls) {
		return "", fmt.Errorf("⚠️ Нет подписки под номером %d. Посмотрите номера в /list.", n)
	}
	return urls[n-1], nil
}

func (b *Bot) handleList(c telebot.Context) error {
	status, body, err := b.callAPI(context.Background(), http.MethodGet, fmt.Sprintf("/subscriptions?chat_id=%d", c.Chat().ID), nil)
	if err != nil {
		b.logger.Error("list call failed", "chat_id", c.Chat().ID, "error", err)
	}
	return c.Send(listReplyText(status, body, err))
}

func (b *Bot) handleStatus(c telebot.Context) error {
	status, body, err := b.callAPI(context.Background(), http.MethodGet, fmt.Sprintf("/status?chat_id=%d", c.Chat().ID), nil)
	if err != nil {
		b.logger.Error("status call failed", "chat_id", c.Chat().ID, "error", err)
	}
	return c.Send(statusReplyText(status, body, err))
}

// callAPI sends body (if non-nil) as JSON to path on the backend, with the
// shared secret header, and returns the response status code and raw body.
func (b *Bot) callAPI(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.apiBaseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Webhook-Secret", b.secret)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, data, nil
}

func subscribeReplyText(status int, body []byte, err error, url string) string {
	if err != nil {
		return "⚠️ Не удалось оформить подписку, попробуйте позже."
	}
	switch status {
	case http.StatusOK:
		return "Вы подписались на уведомления о доступности записи:\n" + url
	case http.StatusBadRequest:
		return "⚠️ Эта ссылка не поддерживается. Подписаться можно только на страницы sport.mos.ru."
	case http.StatusTooManyRequests:
		return "⚠️ " + fallback(errorMessage(body), "превышен лимит подписок, попробуйте позже")
	default:
		return "⚠️ Не удалось оформить подписку, попробуйте позже."
	}
}

func unsubscribeReplyText(status int, err error, url string) string {
	if err != nil {
		return "⚠️ Не удалось отменить подписку, попробуйте позже."
	}
	switch status {
	case http.StatusOK:
		return "Вы отписались от уведомлений:\n" + url
	case http.StatusNotFound:
		return "Вы не были подписаны на эту ссылку."
	case http.StatusBadRequest:
		return "⚠️ Эта ссылка не поддерживается. Подписаться можно только на страницы sport.mos.ru."
	default:
		return "⚠️ Не удалось отменить подписку, попробуйте позже."
	}
}

func listReplyText(status int, body []byte, err error) string {
	if err != nil || status != http.StatusOK {
		return "⚠️ Не удалось получить список подписок, попробуйте позже."
	}
	var resp subscriptionsResponse
	if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
		return "⚠️ Не удалось разобрать ответ бэкенда."
	}
	if len(resp.URLs) == 0 {
		return "У вас нет активных подписок. Используйте /sub <url>, чтобы подписаться."
	}
	lines := make([]string, len(resp.URLs))
	for i, url := range resp.URLs {
		lines[i] = fmt.Sprintf("%d. %s", i+1, url)
	}
	return "Ваши подписки (для отписки: /unsub <номер>):\n" + strings.Join(lines, "\n")
}

func statusReplyText(status int, body []byte, err error) string {
	if err != nil || status != http.StatusOK {
		return "⚠️ Не удалось получить статус, попробуйте позже."
	}
	var resp statusResponse
	if jsonErr := json.Unmarshal(body, &resp); jsonErr != nil {
		return "⚠️ Не удалось разобрать ответ бэкенда."
	}
	if len(resp.Statuses) == 0 {
		return "У вас нет активных подписок."
	}
	blocks := make([]string, 0, len(resp.Statuses))
	for _, s := range resp.Statuses {
		blocks = append(blocks, formatOneStatus(s))
	}
	return strings.Join(blocks, "\n\n")
}

func formatOneStatus(s backendStatus) string {
	icon, label := "❌", "Запись недоступна"
	if s.State == "available" {
		icon, label = "✅", "Запись доступна"
	}
	msg := fmt.Sprintf("%s\n%s %s\nПоследняя проверка: %s", s.URL, icon, label, formatTime(s.LastCheck))
	if !s.LastChange.IsZero() {
		msg += fmt.Sprintf("\nПоследнее изменение: %s", formatTime(s.LastChange))
	}
	return msg
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func errorMessage(body []byte) string {
	var resp errorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	return resp.Error
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
