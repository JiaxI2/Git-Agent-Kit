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
	"github.com/example/git-isolated-agent-kit/internal/githubx"
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
type DraftPullRequestRequest struct {
	Repo     string `json:"repo"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Executor string `json:"executor"`
}
type DraftPullRequestResult struct {
	Number   int    `json:"number"`
	URL      string `json:"url"`
	State    string `json:"state"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Title    string `json:"title"`
	Executor string `json:"executor"`
}
type draftPullRequestRecord struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	Body        string `json:"body"`
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

var runGitHub = githubx.Run
var runGitHubWithBodyFile = githubx.RunWithBodyFile

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
	comment := fmt.Sprintf("GIA claimed this task.\n\n- executor: `%s`\n- branch: `%s`\n- base head: `%s`\n- state: `CLAIMED`", executor, branch, head)
	if _, err := runGitHubWithBodyFile(ctx, repo, comment, "issue", "comment", fmt.Sprint(issue), "--repo", strings.TrimSpace(full)); err != nil {
		return ClaimResult{}, err
	}
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
	head, err := runGitHub(ctx, repo, "pr", "view", fmt.Sprint(pr), "--json", "headRefOid", "--jq", ".headRefOid")
	if err != nil {
		return HandoffResult{}, err
	}
	head = strings.TrimSpace(head)
	if head == "" {
		return HandoffResult{}, fmt.Errorf("gh pr view returned empty headRefOid for PR #%d", pr)
	}
	body := fmt.Sprintf("<!-- GIA:HANDOFF:START -->\nexecutor: %s\nstate: %s\nhead_sha: %s\nnext_executor: %s\nnote: %s\n<!-- GIA:HANDOFF:END -->", to, state, head, to, escapeLine(note))
	if _, err := runGitHubWithBodyFile(ctx, repo, body, "pr", "comment", fmt.Sprint(pr)); err != nil {
		return HandoffResult{}, err
	}
	return HandoffResult{pr, to, state, head}, nil
}

func RequestDraftPullRequest(ctx context.Context, request DraftPullRequestRequest) (DraftPullRequestResult, error) {
	repo := strings.TrimSpace(request.Repo)
	base := strings.TrimSpace(request.Base)
	head := strings.TrimSpace(request.Head)
	title := strings.TrimSpace(request.Title)
	executor := strings.TrimSpace(request.Executor)
	fields := []struct {
		name  string
		value string
	}{
		{name: "repo", value: repo},
		{name: "base", value: base},
		{name: "head", value: head},
		{name: "title", value: title},
		{name: "executor", value: executor},
	}
	for _, field := range fields {
		if field.value == "" {
			return DraftPullRequestResult{}, fmt.Errorf("draft PR %s is required", field.name)
		}
	}
	for _, field := range fields[1:] {
		if strings.ContainsAny(field.value, "\r\n") {
			return DraftPullRequestResult{}, fmt.Errorf("draft PR %s must be a single line", field.name)
		}
	}

	full, err := githubRepositoryName(ctx, repo)
	if err != nil {
		return DraftPullRequestResult{}, err
	}
	body := draftPullRequestBody(request.Body, executor)
	out, err := runGitHubWithBodyFile(ctx, repo, body,
		"pr", "create",
		"--repo", full,
		"--draft",
		"--base", base,
		"--head", head,
		"--title", title,
	)
	if err != nil {
		return DraftPullRequestResult{}, err
	}
	number, err := githubx.ResourceNumberFromURL(out, "pull")
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("parse created draft PR: %w", err)
	}
	createdURL := strings.TrimSpace(out)
	record, err := viewDraftPullRequest(ctx, repo, full, number)
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("draft PR may have been created at %s, but verification failed: %w", createdURL, err)
	}
	expectedHead := head
	if index := strings.LastIndex(expectedHead, ":"); index >= 0 {
		expectedHead = expectedHead[index+1:]
	}
	switch {
	case !record.IsDraft:
		err = fmt.Errorf("PR #%d is not a draft", number)
	case record.BaseRefName != base:
		err = fmt.Errorf("PR #%d base is %q, expected %q", number, record.BaseRefName, base)
	case record.HeadRefName != expectedHead:
		err = fmt.Errorf("PR #%d head is %q, expected %q", number, record.HeadRefName, expectedHead)
	case record.Title != title:
		err = fmt.Errorf("PR #%d title does not match the request", number)
	case normalizeBodyForComparison(record.Body) != normalizeBodyForComparison(body):
		err = fmt.Errorf("PR #%d body does not match the request", number)
	}
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("draft PR may have been created at %s, but verification failed: %w", createdURL, err)
	}
	return DraftPullRequestResult{
		Number:   record.Number,
		URL:      record.URL,
		State:    "PENDING_USER_APPROVAL",
		Base:     record.BaseRefName,
		Head:     record.HeadRefName,
		Title:    record.Title,
		Executor: executor,
	}, nil
}

func githubRepositoryName(ctx context.Context, repo string) (string, error) {
	out, err := runGitHub(ctx, repo, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	full := strings.TrimSpace(out)
	parts := strings.Split(full, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("gh repo view returned invalid nameWithOwner %q", full)
	}
	return full, nil
}

func viewDraftPullRequest(ctx context.Context, repo, full string, number int) (draftPullRequestRecord, error) {
	out, err := runGitHub(ctx, repo,
		"pr", "view", fmt.Sprint(number),
		"--repo", full,
		"--json", "number,url,isDraft,baseRefName,headRefName,title,body",
	)
	if err != nil {
		return draftPullRequestRecord{}, err
	}
	if strings.TrimSpace(out) == "" {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned empty output")
	}
	var record draftPullRequestRecord
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		return draftPullRequestRecord{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	if record.Number != number || record.Number <= 0 {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned PR #%d, expected #%d", record.Number, number)
	}
	urlNumber, err := githubx.ResourceNumberFromURL(record.URL, "pull")
	if err != nil || urlNumber != number {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned invalid URL for PR #%d", number)
	}
	return record, nil
}

func draftPullRequestBody(body, executor string) string {
	content := strings.TrimRight(normalizeNewlines(body), "\n")
	if content != "" {
		content += "\n\n"
	}
	return fmt.Sprintf("%s<!-- GIA:DRAFT-PR:START -->\nexecutor: %s\nstate: PENDING_USER_APPROVAL\n<!-- GIA:DRAFT-PR:END -->\n", content, executor)
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

func normalizeBodyForComparison(value string) string {
	return strings.TrimRight(normalizeNewlines(value), "\n")
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
