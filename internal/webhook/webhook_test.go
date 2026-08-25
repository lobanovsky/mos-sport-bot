package webhook

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mos-sport-bot/internal/telegram"
)

type fakeBroadcaster struct {
	calls   []string
	chatIDs [][]int64
	result  telegram.BroadcastResult
}

func (f *fakeBroadcaster) SendTo(_ context.Context, chatIDs []int64, text string) telegram.BroadcastResult {
	f.calls = append(f.calls, text)
	f.chatIDs = append(f.chatIDs, chatIDs)
	return f.result
}

func newTestHandler(secret string) (*Handler, *fakeBroadcaster) {
	fb := &fakeBroadcaster{result: telegram.BroadcastResult{Sent: 2, Failed: 1}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(fb, secret, logger), fb
}

func TestNotifyRejectsBadSecret(t *testing.T) {
	h, fb := newTestHandler("correct-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook/notify", strings.NewReader(`{"available":true}`))
	req.Header.Set("X-Webhook-Secret", "wrong-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("broadcaster called %d times, want 0", len(fb.calls))
	}
}

func TestNotifyRejectsMalformedBody(t *testing.T) {
	h, fb := newTestHandler("correct-secret")
	req := httptest.NewRequest(http.MethodPost, "/webhook/notify", strings.NewReader(`not json`))
	req.Header.Set("X-Webhook-Secret", "correct-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("broadcaster called %d times, want 0", len(fb.calls))
	}
}

func TestNotifyBroadcastsOnAvailable(t *testing.T) {
	h, fb := newTestHandler("correct-secret")
	body := `{"available":true,"checked_at":"2026-08-25T00:00:00Z","url":"https://sport.mos.ru/sections/11716","chat_ids":[100,200]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/notify", strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", "correct-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(fb.calls) != 1 {
		t.Fatalf("broadcaster called %d times, want 1", len(fb.calls))
	}
	if !strings.Contains(fb.calls[0], "sport.mos.ru/sections/11716") {
		t.Fatalf("broadcast text = %q, want it to contain the URL", fb.calls[0])
	}
	if len(fb.chatIDs[0]) != 2 || fb.chatIDs[0][0] != 100 || fb.chatIDs[0][1] != 200 {
		t.Fatalf("chatIDs = %v, want [100 200]", fb.chatIDs[0])
	}
}

func TestNotifySkipsBroadcastWhenUnavailable(t *testing.T) {
	h, fb := newTestHandler("correct-secret")
	body := `{"available":false,"chat_ids":[100]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/notify", strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", "correct-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("broadcaster called %d times, want 0", len(fb.calls))
	}
}

func TestNotifySkipsBroadcastWhenNoChatIDs(t *testing.T) {
	h, fb := newTestHandler("correct-secret")
	body := `{"available":true,"checked_at":"2026-08-25T00:00:00Z","url":"https://sport.mos.ru/sections/11716","chat_ids":[]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/notify", strings.NewReader(body))
	req.Header.Set("X-Webhook-Secret", "correct-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(fb.calls) != 0 {
		t.Fatalf("broadcaster called %d times, want 0", len(fb.calls))
	}
}

func TestHealthz(t *testing.T) {
	h, _ := newTestHandler("correct-secret")
	rec := httptest.NewRecorder()
	h.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
