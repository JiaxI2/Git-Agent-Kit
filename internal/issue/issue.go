package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/example/git-isolated-agent-kit/internal/config"
	"github.com/example/git-isolated-agent-kit/internal/githubx"
)

type Spec struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Labels   []string `json:"labels"`
	Risk     string   `json:"risk"`
	Executor string   `json:"executor"`
}
type Created struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}
type Item struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Labels []string `json:"labels"`
}
type Detail struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	URL    string   `json:"url"`
	Labels []string `json:"labels"`
}
type ListOptions struct {
	State  string
	Labels []string
	Limit  int
}

type issueRecord struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

var runGitHub = githubx.Run
var runGitHubWithBodyFile = githubx.RunWithBodyFile

func FromDirection(direction, risk, executor string, cfg config.Config) Spec {
	d := strings.TrimSpace(direction)
	if risk == "auto" {
		risk = inferRisk(d)
	}
	title := compactTitle(d)
	body := fmt.Sprintf(`## 背景与方向
%s

## 目标
将上述方向转换为最小充分、可验证、可回滚的代码变更。

## 非目标
- 不修改无关模块。
- 不绕过分支保护、PR、验证或发布门禁。
- 不执行 force push、破坏性清理或未授权跨仓库修改。

## 自动执行约束
- 首选执行器：%s
- 风险等级：%s
- 单分支单写者。
- 仅创建任务分支和 Draft PR，不直接合并或发布。
- 本地或硬件相关事项必须列为未验证并移交本地执行器。

## 验收标准
- [ ] 变更范围与目标一致。
- [ ] 自动化测试通过。
- [ ] PR 明确记录已验证、未验证、风险与回滚。
- [ ] 验证结果绑定明确 commit SHA。
- [ ] 工作区和子模块保持干净。

## 回滚
通过 revert PR 的 squash commit 回滚；禁止重写共享历史。

<!-- GIA:TASK:START -->
executor: %s
risk: %s
state: READY
<!-- GIA:TASK:END -->
`, d, executor, risk, executor, risk)
	labels := []string{cfg.Issue.ReadyLabel, "risk:" + risk, "executor:" + executor}
	return Spec{Title: title, Body: body, Labels: labels, Risk: risk, Executor: executor}
}

func Create(ctx context.Context, repo string, spec Spec, cfg config.Config) (Created, error) {
	full, err := repoName(ctx, repo)
	if err != nil {
		return Created{}, err
	}
	attachedLabels, err := normalizeLabels(spec.Labels)
	if err != nil {
		return Created{}, err
	}
	bootstrapLabels, err := normalizeLabels(append([]string{
		cfg.Issue.ReadyLabel,
		cfg.Issue.ClaimedLabel,
		cfg.Issue.CompletedLabel,
	}, attachedLabels...))
	if err != nil {
		return Created{}, err
	}
	if err := ensureLabels(ctx, repo, full, bootstrapLabels); err != nil {
		return Created{}, err
	}

	args := []string{"issue", "create", "--repo", full, "--title", spec.Title}
	for _, l := range attachedLabels {
		args = append(args, "--label", l)
	}
	out, err := runGitHubWithBodyFile(ctx, repo, spec.Body, args...)
	if err != nil {
		return Created{}, err
	}
	number, err := issueNumberFromURL(out)
	if err != nil {
		return Created{}, fmt.Errorf("parse created issue: %w", err)
	}
	createdURL := strings.TrimSpace(out)
	record, err := viewIssue(ctx, repo, full, number)
	if err != nil {
		return Created{}, fmt.Errorf("issue may have been created at %s, but verification failed: %w", createdURL, err)
	}
	if err := requireLabels(record.Labels, attachedLabels); err != nil {
		return Created{}, fmt.Errorf("issue may have been created at %s, but verification failed: %w", createdURL, err)
	}
	return Created{Number: record.Number, URL: record.URL, Title: record.Title}, nil
}

func Scan(ctx context.Context, repo string, limit int, cfg config.Config) ([]Item, error) {
	return ListReady(ctx, repo, limit, cfg)
}

func Get(ctx context.Context, repo string, number int) (Detail, error) {
	if number <= 0 {
		return Detail{}, fmt.Errorf("issue number must be positive")
	}
	full, err := repoName(ctx, repo)
	if err != nil {
		return Detail{}, err
	}
	record, err := viewIssue(ctx, repo, full, number)
	if err != nil {
		return Detail{}, err
	}
	labels := make([]string, 0, len(record.Labels))
	for _, label := range record.Labels {
		name := strings.TrimSpace(label.Name)
		if name == "" {
			return Detail{}, fmt.Errorf("gh issue view returned an empty label for issue #%d", number)
		}
		labels = append(labels, name)
	}
	return Detail{
		Number: record.Number,
		Title:  record.Title,
		Body:   record.Body,
		URL:    record.URL,
		Labels: labels,
	}, nil
}

func ListReady(ctx context.Context, repo string, limit int, cfg config.Config) ([]Item, error) {
	if strings.TrimSpace(cfg.Issue.ReadyLabel) == "" {
		return nil, fmt.Errorf("issue.readyLabel is required")
	}
	return List(ctx, repo, ListOptions{State: "open", Labels: []string{cfg.Issue.ReadyLabel}, Limit: limit})
}

func List(ctx context.Context, repo string, opts ListOptions) ([]Item, error) {
	full, err := repoName(ctx, repo)
	if err != nil {
		return nil, err
	}
	state := strings.ToLower(strings.TrimSpace(opts.State))
	if state == "" {
		state = "open"
	}
	if state != "open" && state != "closed" && state != "all" {
		return nil, fmt.Errorf("invalid issue state %q", opts.State)
	}
	if opts.Limit == 0 {
		opts.Limit = 20
	}
	if opts.Limit < 0 {
		return nil, fmt.Errorf("issue list limit must be positive")
	}
	labels, err := normalizeLabelsAllowEmpty(opts.Labels)
	if err != nil {
		return nil, err
	}
	args := []string{"issue", "list", "--repo", full, "--state", state, "--limit", strconv.Itoa(opts.Limit), "--json", "number,title,url,labels"}
	for _, label := range labels {
		args = append(args, "--label", label)
	}
	out, err := runGitHub(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("gh issue list returned empty output")
	}
	var raw []issueRecord
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse gh issue list output: %w", err)
	}
	items := make([]Item, 0, len(raw))
	for _, r := range raw {
		if r.Number <= 0 {
			return nil, fmt.Errorf("gh issue list returned invalid issue number %d", r.Number)
		}
		urlNumber, err := issueNumberFromURL(r.URL)
		if err != nil || urlNumber != r.Number {
			return nil, fmt.Errorf("gh issue list returned invalid URL for issue #%d", r.Number)
		}
		if strings.TrimSpace(r.Title) == "" {
			return nil, fmt.Errorf("gh issue list returned an empty title for issue #%d", r.Number)
		}
		labs := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			name := strings.TrimSpace(l.Name)
			if name == "" {
				return nil, fmt.Errorf("gh issue list returned an empty label for issue #%d", r.Number)
			}
			labs = append(labs, name)
		}
		items = append(items, Item{r.Number, r.Title, r.URL, labs})
	}
	return items, nil
}

func repoName(ctx context.Context, repo string) (string, error) {
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

func ensureLabels(ctx context.Context, repo, full string, labels []string) error {
	existing, err := repositoryLabels(ctx, repo, full)
	if err != nil {
		return err
	}
	for _, label := range labels {
		if existing[strings.ToLower(label)] {
			continue
		}
		color, description := labelStyle(label)
		if _, err := runGitHub(ctx, repo, "label", "create", label, "--repo", full, "--color", color, "--description", description); err != nil {
			refreshed, refreshErr := repositoryLabels(ctx, repo, full)
			if refreshErr == nil && refreshed[strings.ToLower(label)] {
				existing = refreshed
				continue
			}
			return fmt.Errorf("bootstrap GitHub label %q: %w", label, err)
		}
		existing[strings.ToLower(label)] = true
	}
	final, err := repositoryLabels(ctx, repo, full)
	if err != nil {
		return err
	}
	for _, label := range labels {
		if !final[strings.ToLower(label)] {
			return fmt.Errorf("GitHub label %q is still missing after bootstrap", label)
		}
	}
	return nil
}

func repositoryLabels(ctx context.Context, repo, full string) (map[string]bool, error) {
	out, err := runGitHub(ctx, repo, "label", "list", "--repo", full, "--limit", "1000", "--json", "name")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("gh label list returned empty output")
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse gh label list output: %w", err)
	}
	labels := make(map[string]bool, len(raw))
	for _, label := range raw {
		name := strings.TrimSpace(label.Name)
		if name == "" {
			return nil, fmt.Errorf("gh label list returned an empty label name")
		}
		labels[strings.ToLower(name)] = true
	}
	return labels, nil
}

func viewIssue(ctx context.Context, repo, full string, number int) (issueRecord, error) {
	out, err := runGitHub(ctx, repo, "issue", "view", strconv.Itoa(number), "--repo", full, "--json", "number,title,body,url,labels")
	if err != nil {
		return issueRecord{}, err
	}
	if strings.TrimSpace(out) == "" {
		return issueRecord{}, fmt.Errorf("gh issue view returned empty output")
	}
	var record issueRecord
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		return issueRecord{}, fmt.Errorf("parse gh issue view output: %w", err)
	}
	if record.Number != number || record.Number <= 0 {
		return issueRecord{}, fmt.Errorf("gh issue view returned issue #%d, expected #%d", record.Number, number)
	}
	urlNumber, err := issueNumberFromURL(record.URL)
	if err != nil || urlNumber != number {
		return issueRecord{}, fmt.Errorf("gh issue view returned invalid URL for issue #%d", number)
	}
	if strings.TrimSpace(record.Title) == "" {
		return issueRecord{}, fmt.Errorf("gh issue view returned an empty title for issue #%d", number)
	}
	return record, nil
}

func issueNumberFromURL(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return 0, fmt.Errorf("invalid issue URL %q", value)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[len(parts)-2] != "issues" {
		return 0, fmt.Errorf("invalid issue URL %q", value)
	}
	number, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid issue URL %q", value)
	}
	return number, nil
}

func requireLabels(actual []struct {
	Name string `json:"name"`
}, required []string) error {
	present := make(map[string]bool, len(actual))
	for _, label := range actual {
		present[strings.ToLower(strings.TrimSpace(label.Name))] = true
	}
	for _, label := range required {
		if !present[strings.ToLower(label)] {
			return fmt.Errorf("required label %q is not attached", label)
		}
	}
	return nil
}

func normalizeLabels(labels []string) ([]string, error) {
	normalized, err := normalizeLabelsAllowEmpty(labels)
	if err != nil {
		return nil, err
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one GitHub label is required")
	}
	return normalized, nil
}

func normalizeLabelsAllowEmpty(labels []string) ([]string, error) {
	seen := make(map[string]bool, len(labels))
	normalized := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			return nil, fmt.Errorf("GitHub label must not be empty")
		}
		if len([]rune(label)) > 50 {
			return nil, fmt.Errorf("GitHub label %q exceeds 50 characters", label)
		}
		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, label)
	}
	return normalized, nil
}

func labelStyle(label string) (string, string) {
	switch {
	case label == "agent:ready":
		return "0E8A16", "Ready for a GIA executor"
	case label == "agent:claimed":
		return "FBCA04", "Claimed by a GIA executor"
	case label == "agent:completed":
		return "5319E7", "Completed through the GIA lifecycle"
	case strings.HasPrefix(label, "risk:"):
		return "D93F0B", "GIA risk classification"
	case strings.HasPrefix(label, "executor:"):
		return "1D76DB", "Preferred or current GIA executor"
	default:
		return "BFD4F2", "Managed by GIA"
	}
}

func compactTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > 70 {
		r = r[:70]
	}
	return "[Agent] " + string(r)
}
func inferRisk(s string) string {
	l := strings.ToLower(s)
	high := []string{"release", "发布", "权限", "secret", "token", "workflow", "hook", "submodule", "删除", "迁移", "安全"}
	for _, x := range high {
		if strings.Contains(l, x) {
			return "high"
		}
	}
	med := []string{"refactor", "重构", "upgrade", "更新", "架构", "依赖"}
	for _, x := range med {
		if strings.Contains(l, x) {
			return "medium"
		}
	}
	return "low"
}
