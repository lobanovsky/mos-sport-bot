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
	if !strings.Contains(nonEmpty, testURL) || !strings.Contains(nonEmpty, "1. "+testURL) {
		t.Fatalf("non-empty list reply = %q, want a numbered entry for %q", nonEmpty, testURL)
	}

	errReply := listReplyText(0, nil, errors.New("boom"))
	if !strings.Contains(strings.ToLower(errReply), "не удалось") {
		t.Fatalf("error list reply = %q", errReply)
	}
}

func TestURLByIndex(t *testing.T) {
	urls := []string{testURL, "https://sport.mos.ru/sections/99"}

	got, err := urlByIndex(urls, 1)
	if err != nil || got != urls[0] {
		t.Fatalf("urlByIndex(1) = %q, %v, want %q, nil", got, err, urls[0])
	}

	got, err = urlByIndex(urls, 2)
	if err != nil || got != urls[1] {
		t.Fatalf("urlByIndex(2) = %q, %v, want %q, nil", got, err, urls[1])
	}

	if _, err := urlByIndex(urls, 0); err == nil {
		t.Fatal("urlByIndex(0) should error")
	}
	if _, err := urlByIndex(urls, 3); err == nil {
		t.Fatal("urlByIndex(3) should error (out of range)")
	}
}

func TestStatusReplyText(t *testing.T) {
	empty := statusReplyText(http.StatusOK, []byte(`{"chat_id":1,"statuses":[]}`), nil)
	if !strings.Contains(empty, "нет активных подписок") {
		t.Fatalf("empty status reply = %q", empty)
	}

	body := `{"chat_id":1,"statuses":[{"state":"available","url":"` + testURL + `","last_check":"2026-08-25T00:00:00Z"}]}`
	nonEmpty := statusReplyText(http.StatusOK, []byte(body), nil)
	if !strings.Contains(nonEmpty, testURL) || !strings.Contains(nonEmpty, "Запись доступна") {
		t.Fatalf("status reply = %q", nonEmpty)
	}
}

func TestFormatOneStatus(t *testing.T) {
	available := backendStatus{
		State:      "available",
		URL:        testURL,
		LastCheck:  time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		LastChange: time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC),
		LastError:  "boom",
	}
	got := formatOneStatus(available)
	for _, want := range []string{testURL, "✅", "Запись доступна", "Последнее изменение"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatOneStatus(available) = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "boom") {
		t.Fatalf("formatOneStatus(available) = %q, should not include the last error", got)
	}

	unavailable := backendStatus{State: "unavailable", URL: testURL, LastCheck: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)}
	got = formatOneStatus(unavailable)
	for _, want := range []string{"❌", "Запись недоступна"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatOneStatus(unavailable) = %q, missing %q", got, want)
		}
	}
}

func TestSubscribersReplyText(t *testing.T) {
	empty := subscribersReplyText(http.StatusOK, []byte(`{"total":0,"subscribers":[]}`), nil)
	if !strings.Contains(empty, "нет") {
		t.Fatalf("empty subscribers reply = %q", empty)
	}

	body := `{"total":2,"subscribers":[
		{"chat_id":1,"username":"evgeny","first_name":"Evgeny","last_name":"Lobanovsky","url_count":2},
		{"chat_id":2,"url_count":1}
	]}`
	got := subscribersReplyText(http.StatusOK, []byte(body), nil)
	for _, want := range []string{"Подписчиков: 2", "@evgeny", "Evgeny Lobanovsky", "chat 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("subscribers reply = %q, missing %q", got, want)
		}
	}

	errReply := subscribersReplyText(0, nil, errors.New("boom"))
	if !strings.Contains(strings.ToLower(errReply), "не удалось") {
		t.Fatalf("error subscribers reply = %q", errReply)
	}
}

func TestDescribeSubscriber(t *testing.T) {
	tests := []struct {
		name string
		sub  subscriberSummary
		want string
	}{
		{"username and name", subscriberSummary{Username: "evgeny", FirstName: "Evgeny", LastName: "Lobanovsky"}, "@evgeny (Evgeny Lobanovsky)"},
		{"username only", subscriberSummary{Username: "evgeny"}, "@evgeny"},
		{"name only", subscriberSummary{FirstName: "Evgeny"}, "Evgeny"},
		{"nothing known", subscriberSummary{ChatID: 42}, "chat 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeSubscriber(tt.sub); got != tt.want {
				t.Fatalf("describeSubscriber(%+v) = %q, want %q", tt.sub, got, tt.want)
			}
		})
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
