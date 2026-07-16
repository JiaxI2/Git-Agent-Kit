package issue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/example/git-isolated-agent-kit/internal/config"
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
	args := []string{"issue", "create", "--repo", full, "--title", spec.Title, "--body", spec.Body}
	for _, l := range spec.Labels {
		args = append(args, "--label", l)
	}
	out, err := gh(ctx, repo, args...)
	if err != nil {
		return Created{}, err
	}
	url := strings.TrimSpace(out)
	re := regexp.MustCompile(`/issues/(\d+)$`)
	m := re.FindStringSubmatch(url)
	n := 0
	if len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &n)
	}
	return Created{Number: n, URL: url, Title: spec.Title}, nil
}

func Scan(ctx context.Context, repo string, limit int, cfg config.Config) ([]Item, error) {
	full, err := repoName(ctx, repo)
	if err != nil {
		return nil, err
	}
	out, err := gh(ctx, repo, "issue", "list", "--repo", full, "--state", "open", "--label", cfg.Issue.ReadyLabel, "--limit", fmt.Sprint(limit), "--json", "number,title,url,labels")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Number     int `json:"number"`
		Title, URL string
		Labels     []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(raw))
	for _, r := range raw {
		labs := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labs = append(labs, l.Name)
		}
		items = append(items, Item{r.Number, r.Title, r.URL, labs})
	}
	return items, nil
}

func repoName(ctx context.Context, repo string) (string, error) {
	out, err := gh(ctx, repo, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	return strings.TrimSpace(out), err
}

func gh(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = repo
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
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
