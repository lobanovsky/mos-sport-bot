package telegram

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testURL = "https://sport.mos.ru/sections/11716"

func TestSubscribeReplyText(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     []byte
		err      error
		contains string
	}{
		{"network error", 0, nil, errors.New("boom"), "не удалось"},
		{"ok", http.StatusOK, nil, nil, testURL},
		{"bad url", http.StatusBadRequest, nil, nil, "sport.mos.ru"},
		{"rate limited with message", http.StatusTooManyRequests, []byte(`{"error":"слишком много подписок у этого чата"}`), nil, "слишком много подписок у этого чата"},
		{"rate limited without message", http.StatusTooManyRequests, []byte(`not json`), nil, "лимит"},
		{"server error", http.StatusInternalServerError, nil, nil, "не удалось"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subscribeReplyText(tt.status, tt.body, tt.err, testURL)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.contains)) {
				t.Fatalf("subscribeReplyText(%d, %q, %v) = %q, want substring %q", tt.status, tt.body, tt.err, got, tt.contains)
			}
		})
	}
}

func TestUnsubscribeReplyText(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		err      error
		contains string
	}{
		{"network error", 0, errors.New("boom"), "не удалось"},
		{"ok", http.StatusOK, nil, testURL},
		{"not found", http.StatusNotFound, nil, "не были подписаны"},
		{"bad url", http.StatusBadRequest, nil, "sport.mos.ru"},
		{"server error", http.StatusInternalServerError, nil, "не удалось"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unsubscribeReplyText(tt.status, tt.err, testURL)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.contains)) {
				t.Fatalf("unsubscribeReplyText(%d, %v) = %q, want substring %q", tt.status, tt.err, got, tt.contains)
			}
		})
	}
}

func TestListReplyText(t *testing.T) {
	empty := listReplyText(http.StatusOK, []byte(`{"chat_id":1,"urls":[]}`), nil)
	if !strings.Contains(empty, "нет активных подписок") {
		t.Fatalf("empty list reply = %q", empty)
	}

	nonEmpty := listReplyText(http.StatusOK, []byte(`{"chat_id":1,"urls":["`+testURL+`"]}`), nil)
	if !strings.Contains(nonEmpty, testURL) {
		t.Fatalf("non-empty list reply = %q, want it to contain %q", nonEmpty, testURL)
	}

	errReply := listReplyText(0, nil, errors.New("boom"))
	if !strings.Contains(strings.ToLower(errReply), "не удалось") {
		t.Fatalf("error list reply = %q", errReply)
	}
}

func TestStatusReplyText(t *testing.T) {
	empty := statusReplyText(http.StatusOK, []byte(`{"chat_id":1,"statuses":[]}`), nil)
	if !strings.Contains(empty, "нет активных подписок") {
		t.Fatalf("empty status reply = %q", empty)
	}

	body := `{"chat_id":1,"statuses":[{"state":"available","url":"` + testURL + `","last_check":"2026-08-25T00:00:00Z"}]}`
	nonEmpty := statusReplyText(http.StatusOK, []byte(body), nil)
	if !strings.Contains(nonEmpty, testURL) || !strings.Contains(nonEmpty, "available") {
		t.Fatalf("status reply = %q", nonEmpty)
	}
}

func TestFormatOneStatus(t *testing.T) {
	s := backendStatus{
		State:      "available",
		URL:        testURL,
		LastCheck:  time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		LastChange: time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC),
		LastError:  "boom",
	}
	got := formatOneStatus(s)
	for _, want := range []string{testURL, "available", "Последнее изменение", "boom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatOneStatus() = %q, missing %q", got, want)
		}
	}
}

func TestErrorMessage(t *testing.T) {
	if got := errorMessage([]byte(`{"error":"oops"}`)); got != "oops" {
		t.Fatalf("errorMessage = %q, want oops", got)
	}
	if got := errorMessage([]byte(`not json`)); got != "" {
		t.Fatalf("errorMessage(malformed) = %q, want empty", got)
	}
}
