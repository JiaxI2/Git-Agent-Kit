package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/issue"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/workflow"
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
		{"help", "pr", "request"},
		{"pr", "request", "--help"},
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

func TestExecuteEmitsOneStructuredFailureDocument(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".gia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(filepath.Join(repo, ".gia", "config.json"), config.Default()); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() error {
		if code := execute(context.Background(), []string{"validate", "--repo", repo}); code != 1 {
			t.Fatalf("exit code=%d, want 1", code)
		}
		return nil
	})
	decoder := json.NewDecoder(strings.NewReader(out))
	var result output
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode failure output: %v\n%s", err, out)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("failure output contains more than one JSON document: %v\n%s", err, out)
	}
	if result.OK || result.Command != "validate" || result.Error == "" || result.Data == nil {
		t.Fatalf("failure output=%+v", result)
	}
}

func TestExecuteAddsCommandToUnreportedFailure(t *testing.T) {
	out := captureStdout(t, func() error {
		if code := execute(context.Background(), []string{"pr", "missing"}); code != 1 {
			t.Fatalf("exit code=%d, want 1", code)
		}
		return nil
	})
	var result output
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode failure output: %v\n%s", err, out)
	}
	if result.Command != "pr missing" || result.Error == "" {
		t.Fatalf("failure output=%+v", result)
	}
}

func TestEmitDoesNotEscapeHTML(t *testing.T) {
	out := captureStdout(t, func() error {
		emit(output{OK: false, Command: "test", Error: "<unsafe>&"})
		return nil
	})
	if strings.Contains(out, `\u003c`) || strings.Contains(out, `\u003e`) || strings.Contains(out, `\u0026`) {
		t.Fatalf("HTML characters were escaped: %s", out)
	}
	var result output
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != "<unsafe>&" {
		t.Fatalf("error=%q", result.Error)
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

func TestInitSupportsJSONYAMLAndYML(t *testing.T) {
	for _, format := range []string{"json", "yaml", "yml"} {
		t.Run(format, func(t *testing.T) {
			repo := t.TempDir()
			out := captureStdout(t, func() error {
				return cmdInit([]string{"--repo", repo, "--format", format})
			})
			var result struct {
				Data initData `json:"data"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(repo, ".gia", "config."+format)
			if result.Data.Config != want {
				t.Fatalf("config=%q want=%q", result.Data.Config, want)
			}
			if _, err := config.LoadFromRepo(repo); err != nil {
				t.Fatalf("load initialized config: %v", err)
			}
		})
	}
}

func TestIssueCreateUsesExplicitConfigAndExecutorPolicy(t *testing.T) {
	repo := t.TempDir()
	cfg := config.Default()
	cfg.Issue.ReadyLabel = "review:ready"
	cfg.Permissions.AllowedExecutors = []string{"local-agent"}
	path := filepath.Join(repo, "review.yml")
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() error {
		return cmdIssue(context.Background(), []string{
			"create", "--repo", repo, "--config", "review.yml",
			"--direction", "prepare review", "--executor", "local-agent", "--dry-run",
		})
	})
	var result struct {
		Data issue.Spec `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Executor != "local-agent" || !containsString(result.Data.Labels, "review:ready") {
		t.Fatalf("spec=%+v", result.Data)
	}
	err := cmdIssue(context.Background(), []string{
		"create", "--repo", repo, "--config", "review.yml",
		"--direction", "prepare review", "--executor", "web-agent", "--dry-run",
	})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("executor policy error=%v", err)
	}
}

func TestRequiredCommandsAcceptExplicitConfigFlag(t *testing.T) {
	repo := t.TempDir()
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "issue", run: func() error {
			return cmdIssue(context.Background(), []string{"create", "--repo", repo, "--config", "missing.yml", "--direction", "task", "--dry-run"})
		}},
		{name: "scan", run: func() error {
			return cmdScan(context.Background(), []string{"--repo", repo, "--config", "missing.yml"})
		}},
		{name: "claim", run: func() error {
			return cmdClaim(context.Background(), []string{"--repo", repo, "--config", "missing.yml", "--issue", "1"})
		}},
		{name: "validate", run: func() error {
			return cmdValidate(context.Background(), []string{"--repo", repo, "--config", "missing.yml"})
		}},
		{name: "notify", run: func() error {
			return cmdNotify(context.Background(), []string{"--repo", repo, "--config", "missing.yml"})
		}},
		{name: "pr request", run: func() error {
			return cmdPR(context.Background(), []string{"request", "--repo", repo, "--config", "missing.yml", "--issue", "1"})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "read ") {
				t.Fatalf("explicit config error=%v", err)
			}
			if strings.Contains(err.Error(), "flag provided but not defined") {
				t.Fatalf("--config was not registered: %v", err)
			}
		})
	}
}

func TestNotifyKeepsStdoutMachineReadable(t *testing.T) {
	repo := t.TempDir()
	cfg := config.Default()
	cfg.Notifications.Console = true
	if err := os.MkdirAll(filepath.Join(repo, ".gia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(filepath.Join(repo, ".gia", "config.json"), cfg); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() error {
		return cmdNotify(context.Background(), []string{
			"--repo", repo,
			"--event", "ready",
			"--message", "work now",
		})
	})
	var result output
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("notify stdout is not one JSON document: %v\n%s", err, out)
	}
	if !result.OK || result.Command != "notify" {
		t.Fatalf("notify output=%+v", result)
	}
}

func TestPRRequestUsesClaimedIssueAndRemoteBranchTip(t *testing.T) {
	repo, cfg := newDraftPRRepository(t)
	oldGet := getIssueDetail
	oldRequest := requestDraftPullRequest
	t.Cleanup(func() {
		getIssueDetail = oldGet
		requestDraftPullRequest = oldRequest
	})
	getIssueDetail = func(_ context.Context, gotRepo string, number int) (issue.Detail, error) {
		if gotRepo != repo || number != 7 {
			t.Fatalf("issue lookup repo=%q number=%d", gotRepo, number)
		}
		return issue.Detail{
			Number: 7,
			Title:  "Issue title",
			Body:   "Issue body\nsecond line",
			Labels: []string{cfg.Issue.ClaimedLabel, "executor:web-agent"},
		}, nil
	}
	var request workflow.DraftPullRequestRequest
	requestDraftPullRequest = func(_ context.Context, got workflow.DraftPullRequestRequest) (workflow.DraftPullRequestResult, error) {
		request = got
		return workflow.DraftPullRequestResult{
			Number: 8, URL: "https://github.com/owner/repo/pull/8",
			State: "PENDING_USER_APPROVAL", Base: "main",
			Head: got.Head, Title: got.Title, Executor: got.Executor,
		}, nil
	}

	out := captureStdout(t, func() error {
		return cmdPR(context.Background(), []string{"request", "--repo", repo, "--issue", "7", "--executor", "web-agent"})
	})
	var result struct {
		Data prRequestData `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.Draft || result.Data.State != "PENDING_USER_APPROVAL" || result.Data.HeadSHA == "" {
		t.Fatalf("result=%+v", result.Data)
	}
	if request.Title != "Issue title" || request.Body != "Issue body\nsecond line" ||
		request.Base != "main" || request.Head != "agent/web-agent/feat/7-task" {
		t.Fatalf("request=%+v", request)
	}
}

func TestPRRequestRejectsUnpushedHeadBeforeIssueLookup(t *testing.T) {
	repo, cfg := newDraftPRRepository(t)
	if err := os.WriteFile(filepath.Join(repo, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "local.txt")
	runGitTest(t, repo, "commit", "-m", "local only")

	oldGet := getIssueDetail
	t.Cleanup(func() { getIssueDetail = oldGet })
	getIssueDetail = func(context.Context, string, int) (issue.Detail, error) {
		t.Fatal("Issue must not be read before remote SHA validation")
		return issue.Detail{}, nil
	}
	_, err := prepareDraftPullRequest(context.Background(), repo, 7, "", "", "web-agent", cfg)
	if err == nil || !strings.Contains(err.Error(), "does not match remote branch tip") {
		t.Fatalf("remote SHA error=%v", err)
	}
}

func TestClaimedIssueAndBodyFileSecurity(t *testing.T) {
	cfg := config.Default()
	good := issue.Detail{Number: 7, Labels: []string{cfg.Issue.ClaimedLabel, "executor:web-agent"}}
	if err := requireClaimedIssue(good, "web-agent", cfg); err != nil {
		t.Fatal(err)
	}
	missing := issue.Detail{Number: 7, Labels: []string{"executor:web-agent"}}
	if err := requireClaimedIssue(missing, "web-agent", cfg); err == nil || !strings.Contains(err.Error(), "not claimed") {
		t.Fatalf("missing claim error=%v", err)
	}
	mismatch := issue.Detail{Number: 7, Labels: []string{cfg.Issue.ClaimedLabel, "executor:local-agent"}}
	if err := requireClaimedIssue(mismatch, "web-agent", cfg); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("executor mismatch error=%v", err)
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "body.md")
	outside := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(inside, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readRepositoryFile(repo, inside); err != nil || string(got) != "body" {
		t.Fatalf("inside file=%q err=%v", got, err)
	}
	if _, err := readRepositoryFile(repo, outside); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("outside file error=%v", err)
	}
}

func TestReadRepositoryFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repo, "linked-secret.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic link creation is unavailable: %v", err)
	}
	if _, err := readRepositoryFile(repo, link); err == nil || !strings.Contains(err.Error(), "outside repository") {
		t.Fatalf("symlink escape error=%v", err)
	}
}

func newDraftPRRepository(t *testing.T) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	runGitTest(t, root, "init", "--bare", remote)
	runGitTest(t, root, "init", "-b", "main", repo)
	runGitTest(t, repo, "config", "user.name", "GIA Test")
	runGitTest(t, repo, "config", "user.email", "gia@example.invalid")
	cfg := config.Default()
	if err := os.MkdirAll(filepath.Join(repo, ".gia"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(filepath.Join(repo, ".gia", "config.json"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("draft test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", ".gia/config.json", "README.md")
	runGitTest(t, repo, "commit", "-m", "initial")
	runGitTest(t, repo, "remote", "add", "origin", remote)
	runGitTest(t, repo, "push", "-u", "origin", "main")
	runGitTest(t, repo, "checkout", "-b", "agent/web-agent/feat/7-task")
	if err := os.WriteFile(filepath.Join(repo, "change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "change.txt")
	runGitTest(t, repo, "commit", "-m", "change")
	runGitTest(t, repo, "push", "-u", "origin", "agent/web-agent/feat/7-task")
	return repo, cfg
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
