package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/example/git-isolated-agent-kit/internal/config"
)

type Result struct {
	Console bool `json:"console"`
	Webhook bool `json:"webhook"`
	Command bool `json:"command"`
}

func Send(ctx context.Context, event, message string, cfg config.NotificationConfig) (Result, error) {
	r := Result{}
	if cfg.Console {
		fmt.Printf("[%s] %s\n", event, message)
		r.Console = true
	}
	if cfg.WebhookURL != "" {
		b, _ := json.Marshal(map[string]string{"event": event, "message": message, "time": time.Now().UTC().Format(time.RFC3339)})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(b))
		if err != nil {
			return r, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return r, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return r, fmt.Errorf("webhook returned %s", resp.Status)
		}
		r.Webhook = true
	}
	if len(cfg.Command) > 0 {
		args := make([]string, len(cfg.Command)-1)
		for i, a := range cfg.Command[1:] {
			args[i] = strings.NewReplacer("{event}", event, "{message}", message).Replace(a)
		}
		cmd := exec.CommandContext(ctx, cfg.Command[0], args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return r, fmt.Errorf("notification command: %w: %s", err, strings.TrimSpace(string(out)))
		}
		r.Command = true
	}
	return r, nil
}
