package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/git-isolated-agent-kit/internal/config"
	"github.com/example/git-isolated-agent-kit/internal/issue"
)

func TestHelpFormsReturnSuccess(t *testing.T) {
	tests := [][]string{
		{"help"},
		{"--help"},
		{"-h"},
		{"help", "scan"},
		{"scan", "--help"},
		{"help", "issue", "create"},
		{"issue", "create", "--help"},
		{"help", "issue", "list"},
		{"issue", "list", "--help"},
		{"worktree", "create", "--help"},
	}
	for _, args := range tests {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out := captureStdout(t, func() error {
				return run(context.Background(), args)
			})
			if strings.TrimSpace(out) == "" {
				t.Fatal("help output is empty")
			}
		})
	}
}

func TestUnknownHelpTopicFails(t *testing.T) {
	if err := run(context.Background(), []string{"help", "missing"}); err == nil {
		t.Fatal("expected unknown help topic error")
	}
}

func TestIssueListOutputPreservesItemsAndAddsEmptyGuidance(t *testing.T) {
	cfg := config.Default()
	items := []issue.Item{}
	got := issueListOutput("issue list", items, cfg, 7)
	if got.Command != "issue list" {
		t.Fatalf("command=%q", got.Command)
	}
	if _, ok := got.Data.([]issue.Item); !ok {
		t.Fatalf("data type=%T, want []issue.Item", got.Data)
	}
	if got.Guidance == nil {
		t.Fatal("empty result should include guidance")
	}
	if got.Guidance.Query["label"] != cfg.Issue.ReadyLabel {
		t.Fatalf("label=%v", got.Guidance.Query["label"])
	}
	if got.Guidance.Query["limit"] != 7 {
		t.Fatalf("limit=%v", got.Guidance.Query["limit"])
	}
}

func TestIssueListOutputWithItemsHasNoGuidance(t *testing.T) {
	cfg := config.Default()
	items := []issue.Item{{Number: 1, Title: "task"}}
	got := issueListOutput("scan", items, cfg, 20)
	if got.Guidance != nil {
		t.Fatal("non-empty result should not include empty-result guidance")
	}
}

func TestInitReportsRepositoryNextSteps(t *testing.T) {
	repo := t.TempDir()
	out := captureStdout(t, func() error {
		return cmdInit([]string{"--repo", repo})
	})
	var result struct {
		OK   bool     `json:"ok"`
		Data initData `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode init output: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatal("init output not OK")
	}
	if result.Data.Config != filepath.Join(repo, ".gia", "config.json") {
		t.Fatalf("config=%q", result.Data.Config)
	}
	joined := strings.Join(result.Data.NextSteps, "\n")
	if !strings.Contains(joined, "git add .gia") || !strings.Contains(joined, ".gitignore") {
		t.Fatalf("missing commit/ignore guidance: %s", joined)
	}
}

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	if closeErr := w.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = old
	b, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if runErr != nil {
		t.Fatalf("run failed: %v", runErr)
	}
	return string(b)
}
