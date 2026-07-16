package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/example/git-isolated-agent-kit/internal/config"
	"github.com/example/git-isolated-agent-kit/internal/gitx"
)

type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}
type DoctorReport struct {
	OK     bool          `json:"ok"`
	Checks []DoctorCheck `json:"checks"`
}
type ClaimResult struct {
	Issue    int    `json:"issue"`
	Branch   string `json:"branch"`
	Executor string `json:"executor"`
	State    string `json:"state"`
	Head     string `json:"head"`
}
type WorktreeResult struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Ref    string `json:"ref"`
	Head   string `json:"head"`
}
type HandoffResult struct {
	PR    int    `json:"pr"`
	To    string `json:"to"`
	State string `json:"state"`
	Head  string `json:"head"`
}
type StatusResult struct {
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Clean     bool   `json:"clean"`
	Worktrees string `json:"worktrees"`
}
type FeedbackResult struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

func Initialize(repo string, force bool) error {
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(cfgPath); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite", cfgPath)
	}
	if err := config.Save(cfgPath, config.Default()); err != nil {
		return err
	}
	templates := map[string]string{
		"issue.md":   "# Agent Task\n\nDescribe one optimization direction. GIA converts it into a controlled issue.\n",
		"handoff.md": "<!-- GIA:HANDOFF:START -->\nexecutor: web-agent\nstate: WEB_OWNED\nhead_sha: <sha>\nnext_executor: local-agent\n<!-- GIA:HANDOFF:END -->\n",
	}
	tdir := filepath.Join(dir, "templates")
	_ = os.MkdirAll(tdir, 0o755)
	for n, c := range templates {
		_ = os.WriteFile(filepath.Join(tdir, n), []byte(c), 0o644)
	}
	return nil
}

func Doctor(ctx context.Context, repo string) DoctorReport {
	checks := []DoctorCheck{}
	add := func(name string, err error, detail string) {
		checks = append(checks, DoctorCheck{name, err == nil, detail})
	}
	_, err := exec.LookPath("git")
	add("git", err, "Git CLI")
	_, err = exec.LookPath("gh")
	add("gh", err, "GitHub CLI; required for issue/PR automation")
	_, err = exec.LookPath("go")
	add("go", err, "Go toolchain")
	_, err = gitx.Run(ctx, repo, "rev-parse", "--is-inside-work-tree")
	add("repository", err, repo)
	_, err = config.LoadFromRepo(repo)
	add("config", err, filepath.Join(repo, ".gia", "config.json"))
	_, err = run(ctx, repo, "gh", "auth", "status")
	add("github-auth", err, "gh auth status")
	ok := true
	for _, c := range checks {
		if !c.OK {
			ok = false
		}
	}
	return DoctorReport{ok, checks}
}

func Claim(ctx context.Context, repo string, issue int, executor string, cfg config.Config) (ClaimResult, error) {
	if err := gitx.Fetch(ctx, repo); err != nil {
		return ClaimResult{}, err
	}
	base := "origin/" + cfg.DefaultBranch
	head, err := gitx.Run(ctx, repo, "rev-parse", base)
	if err != nil {
		return ClaimResult{}, err
	}
	slug := fmt.Sprintf("issue-%d", issue)
	branch := strings.NewReplacer("{executor}", sanitize(executor), "{type}", "feat", "{issue}", fmt.Sprint(issue), "{slug}", slug).Replace(cfg.Branches.Pattern)
	full, err := run(ctx, repo, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return ClaimResult{}, err
	}
	// GitHub branch creation through git is intentionally explicit and auditable.
	if _, err := gitx.Run(ctx, repo, "branch", branch, head); err != nil && !strings.Contains(err.Error(), "already exists") {
		return ClaimResult{}, err
	}
	if _, err := gitx.Run(ctx, repo, "push", "-u", "origin", branch); err != nil {
		return ClaimResult{}, err
	}
	_, _ = run(ctx, repo, "gh", "issue", "edit", fmt.Sprint(issue), "--repo", strings.TrimSpace(full), "--add-label", cfg.Issue.ClaimedLabel, "--remove-label", cfg.Issue.ReadyLabel, "--add-label", "executor:"+executor)
	_, _ = run(ctx, repo, "gh", "issue", "comment", fmt.Sprint(issue), "--repo", strings.TrimSpace(full), "--body", fmt.Sprintf("GIA claimed this task.\n\n- executor: `%s`\n- branch: `%s`\n- base head: `%s`\n- state: `CLAIMED`", executor, branch, head))
	return ClaimResult{issue, branch, executor, "CLAIMED", head}, nil
}

func CreateValidationWorktree(ctx context.Context, repo string, pr int, ref, root string) (WorktreeResult, error) {
	if err := gitx.Fetch(ctx, repo); err != nil {
		return WorktreeResult{}, err
	}
	if ref == "" {
		out, err := run(ctx, repo, "gh", "pr", "view", fmt.Sprint(pr), "--json", "headRefName", "--jq", ".headRefName")
		if err != nil {
			return WorktreeResult{}, err
		}
		ref = "origin/" + strings.TrimSpace(out)
	}
	if root == "" {
		cfg, err := config.LoadFromRepo(repo)
		if err != nil {
			return WorktreeResult{}, err
		}
		root = cfg.Worktrees.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(repo, root)
		}
	}
	name := fmt.Sprintf("pr-%d", pr)
	if pr == 0 {
		name = sanitize(strings.TrimPrefix(ref, "origin/"))
	}
	path := filepath.Clean(filepath.Join(root, name))
	branch := "verify/" + name
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WorktreeResult{}, err
	}
	if err := gitx.CreateWorktree(ctx, repo, path, branch, ref); err != nil {
		return WorktreeResult{}, err
	}
	head, err := gitx.Head(ctx, path)
	if err != nil {
		return WorktreeResult{}, err
	}
	return WorktreeResult{path, branch, ref, head}, nil
}

func Handoff(ctx context.Context, repo string, pr int, to, state, note string) (HandoffResult, error) {
	head, err := run(ctx, repo, "gh", "pr", "view", fmt.Sprint(pr), "--json", "headRefOid", "--jq", ".headRefOid")
	if err != nil {
		return HandoffResult{}, err
	}
	head = strings.TrimSpace(head)
	body := fmt.Sprintf("<!-- GIA:HANDOFF:START -->\nexecutor: %s\nstate: %s\nhead_sha: %s\nnext_executor: %s\nnote: %s\n<!-- GIA:HANDOFF:END -->", to, state, head, to, escapeLine(note))
	if _, err := run(ctx, repo, "gh", "pr", "comment", fmt.Sprint(pr), "--body", body); err != nil {
		return HandoffResult{}, err
	}
	return HandoffResult{pr, to, state, head}, nil
}

func Status(ctx context.Context, repo string) (StatusResult, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return StatusResult{}, err
	}
	b, err := gitx.Branch(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	h, err := gitx.Head(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	clean, err := gitx.IsClean(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	wt, err := gitx.Run(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{abs, b, h, clean, wt}, nil
}

func RecordFeedback(ctx context.Context, repo, category, message string, pr int) (FeedbackResult, error) {
	dir := filepath.Join(repo, ".gia", "feedback")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FeedbackResult{}, err
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(dir, name)
	data := map[string]interface{}{"time": time.Now().UTC().Format(time.RFC3339Nano), "category": category, "message": message, "pr": pr, "os": runtime.GOOS, "arch": runtime.GOARCH}
	b, _ := json.MarshalIndent(data, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{path, category}, nil
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
func sanitize(s string) string {
	r := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	return strings.Trim(r.ReplaceAllString(strings.ToLower(s), "-"), "-")
}
func escapeLine(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ") }
