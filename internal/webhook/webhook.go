package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"mos-sport-bot/internal/telegram"
)

const maxBodyBytes = 4096

type notification struct {
	Available     bool      `json:"available"`
	CheckedAt     time.Time `json:"checked_at"`
	URL           string    `json:"url"`
	ActiveButtons int       `json:"active_buttons"`
	Buttons       []string  `json:"buttons"`
}

type Broadcaster interface {
	Broadcast(ctx context.Context, text string) telegram.BroadcastResult
}

type Handler struct {
	broadcaster Broadcaster
	secret      string
	logger      *slog.Logger
}

func New(broadcaster Broadcaster, secret string, logger *slog.Logger) *Handler {
	return &Handler{broadcaster: broadcaster, secret: secret, logger: logger}
}

func (h *Handler) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /webhook/notify", h.notify)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h *Handler) notify(w http.ResponseWriter, r *http.Request) {
	got := []byte(r.Header.Get("X-Webhook-Secret"))
	want := []byte(h.secret)
	if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var n notification
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&n); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if !n.Available {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	text := fmt.Sprintf("🎾 Появилась запись!\nПроверено: %s\n%s", n.CheckedAt.Local().Format("2006-01-02 15:04:05"), n.URL)
	result := h.broadcaster.Broadcast(r.Context(), text)
	h.logger.Info("notification broadcast", "sent", result.Sent, "failed", result.Failed)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"sent": result.Sent, "failed": result.Failed})
}
