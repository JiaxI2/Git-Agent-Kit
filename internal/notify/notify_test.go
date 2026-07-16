package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/example/git-isolated-agent-kit/internal/config"
)

func TestSendWithNoChannels(t *testing.T) {
	got, err := Send(context.Background(), "test", "message", config.NotificationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Console || got.Webhook || got.Command {
		t.Fatalf("unexpected delivery result: %+v", got)
	}
}

func TestSendWebhook(t *testing.T) {
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type=%q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	got, err := Send(context.Background(), "complete", "done", config.NotificationConfig{WebhookURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Webhook {
		t.Fatal("webhook delivery not reported")
	}
	if payload["event"] != "complete" || payload["message"] != "done" || payload["time"] == "" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestSendWebhookRejectsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := Send(context.Background(), "test", "message", config.NotificationConfig{WebhookURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestSendCommandSubstitutesEventAndMessage(t *testing.T) {
	cfg := config.NotificationConfig{
		Command: []string{os.Args[0], "-test.run=TestNotifyHelperProcess", "--", "{event}", "{message}"},
	}
	got, err := Send(context.Background(), "ready", "work now", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Command {
		t.Fatal("command delivery not reported")
	}
}

func TestNotifyHelperProcess(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	got := os.Args[separator+1:]
	if len(got) != 2 || got[0] != "ready" || got[1] != "work now" {
		os.Exit(2)
	}
}
