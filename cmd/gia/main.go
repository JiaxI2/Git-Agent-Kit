package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/adapters/legacy"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/gitx"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/issue"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/notify"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/validate"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/workflow"
)

var version = "dev"
var buildApplicationServices = legacy.NewServices

type output struct {
	OK       bool        `json:"ok"`
	Command  string      `json:"command"`
	Data     interface{} `json:"data,omitempty"`
	Guidance *guidance   `json:"guidance,omitempty"`
	Error    string      `json:"error,omitempty"`
}

type guidance struct {
	Summary         string                 `json:"summary"`
	Query           map[string]interface{} `json:"query,omitempty"`
	PossibleReasons []string               `json:"possibleReasons,omitempty"`
	NextSteps       []string               `json:"nextSteps,omitempty"`
}

type initData struct {
	Repo      string   `json:"repo"`
	Config    string   `json:"config"`
	NextSteps []string `json:"nextSteps"`
}

type prRequestData struct {
	Number   int    `json:"number"`
	URL      string `json:"url"`
	Draft    bool   `json:"draft"`
	State    string `json:"state"`
	Issue    int    `json:"issue"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	HeadSHA  string `json:"headSha"`
	Title    string `json:"title"`
	Executor string `json:"executor"`
}

var getIssueDetail = issue.Get
var requestDraftPullRequest = workflow.RequestDraftPullRequest

type reportedError struct {
	err error
}

func (e reportedError) Error() string {
	return e.err.Error()
}

func (e reportedError) Unwrap() error {
	return e.err
}

func main() {
	if code := execute(context.Background(), os.Args[1:]); code != 0 {
		os.Exit(code)
	}
}

func execute(ctx context.Context, args []string) int {
	if err := run(ctx, args); err != nil {
		var reported reportedError
		if !errors.As(err, &reported) {
			emit(output{OK: false, Command: commandPath(args), Error: err.Error()})
		}
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	if path, ok := requestedHelp(args); ok {
		return showHelp(path)
	}
	switch args[0] {
	case "version":
		emit(output{OK: true, Command: "version", Data: map[string]string{"version": version}})
		return nil
	case "init":
		return cmdInit(args[1:])
	case "doctor":
		return cmdDoctor(ctx, args[1:])
	case "issue":
		return cmdIssue(ctx, args[1:])
	case "scan":
		return cmdScan(ctx, args[1:])
	case "claim":
		return cmdClaim(ctx, args[1:])
	case "pr":
		return cmdPR(ctx, args[1:])
	case "worktree":
		return cmdWorktree(ctx, args[1:])
	case "validate":
		return cmdValidate(ctx, args[1:])
	case "plan":
		return cmdPlan(ctx, args[1:])
	case "handoff":
		return cmdHandoff(ctx, args[1:])
	case "status":
		return cmdStatus(ctx, args[1:])
	case "feedback":
		return cmdFeedback(ctx, args[1:])
	case "notify":
		return cmdNotify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repo := fs.String("repo", ".", "target repository")
	format := fs.String("format", "json", "json|yaml|yml")
	force := fs.Bool("force", false, "overwrite existing config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	abs, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	configPath, err := workflow.InitializeWithFormat(abs, *format, *force)
	if err != nil {
		return err
	}
	displayConfig, err := filepath.Rel(abs, configPath)
	if err != nil {
		displayConfig = configPath
	}
	displayConfig = filepath.ToSlash(displayConfig)
	emit(output{
		OK:      true,
		Command: "init",
		Data: initData{
			Repo:   abs,
			Config: configPath,
			NextSteps: []string{
				"检查 " + displayConfig + "，并按目标仓库技术栈调整验证命令。",
				"若团队共享配置：git add .gia && git commit；若仅本地使用：将 .gia/ 加入 .gitignore。",
				"运行 gia doctor --repo <repository> 检查 Git、GitHub CLI、认证和配置。",
			},
		},
	})
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	services := legacy.NewServices(*repo, *configPath)
	report, err := services.Repository.Inspect(ctx, domain.RepositoryRequest{Repository: *repo, ConfigPath: *configPath, Mode: "doctor"})
	if err != nil {
		return err
	}
	if !report.OK {
		return emitFailure("doctor", report, errors.New("doctor checks failed"))
	}
	emit(output{OK: true, Command: "doctor", Data: report})
	return nil
}

func cmdIssue(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gia issue create|list")
	}
	if args[0] == "list" {
		return cmdIssueList(ctx, args[1:])
	}
	if args[0] != "create" {
		return errors.New("usage: gia issue create|list")
	}
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	direction := fs.String("direction", "", "optimization direction or task request")
	risk := fs.String("risk", "auto", "auto|low|medium|high")
	executor := fs.String("executor", "web-agent", "preferred executor")
	dryRun := fs.Bool("dry-run", false, "render only")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*direction) == "" {
		return errors.New("--direction is required")
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	if !cfg.ExecutorAllowed(*executor) {
		return fmt.Errorf("executor %q is not allowed by permissions.allowedExecutors", strings.TrimSpace(*executor))
	}
	spec := issue.FromDirection(*direction, *risk, *executor, cfg)
	if *dryRun {
		emit(output{OK: true, Command: "issue create", Data: spec})
		return nil
	}
	created, err := issue.Create(ctx, *repo, spec, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "issue create", Data: created})
	return nil
}

func cmdScan(ctx context.Context, args []string) error {
	return listIssues(ctx, args, "scan")
}

func cmdIssueList(ctx context.Context, args []string) error {
	return listIssues(ctx, args, "issue list")
}

func listIssues(ctx context.Context, args []string, command string) error {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	limit := fs.Int("limit", 20, "maximum issues")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		return errors.New("--limit must be greater than zero")
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	items, err := issue.Scan(ctx, *repo, *limit, cfg)
	if err != nil {
		return err
	}
	emit(issueListOutput(command, items, cfg, *limit))
	return nil
}

func issueListOutput(command string, items []issue.Item, cfg config.Config, limit int) output {
	result := output{OK: true, Command: command, Data: items}
	if len(items) == 0 {
		result.Guidance = &guidance{
			Summary: "没有找到可认领的 Issue。",
			Query: map[string]interface{}{
				"state": "open",
				"label": cfg.Issue.ReadyLabel,
				"limit": limit,
			},
			PossibleReasons: []string{
				"仓库当前没有打开且带有 ready 标签的 Issue。",
				"Issue 使用了不同的 ready 标签，或已被认领/关闭。",
				"当前 gh 登录账号无权读取目标仓库 Issue。",
			},
			NextSteps: []string{
				"运行 gia issue create --repo <repository> --direction <text> 创建任务。",
				"在 GitHub 检查 Issue 状态和标签，或核对生效配置的 issue.readyLabel。",
				"运行 gia doctor --repo <repository> 检查 GitHub CLI 与认证。",
			},
		}
	}
	return result
}

func cmdClaim(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	number := fs.Int("issue", 0, "issue number")
	executor := fs.String("executor", "web-agent", "executor")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *number <= 0 {
		return errors.New("--issue is required")
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	if !cfg.ExecutorAllowed(*executor) {
		return fmt.Errorf("executor %q is not allowed by permissions.allowedExecutors", strings.TrimSpace(*executor))
	}
	result, err := workflow.Claim(ctx, *repo, *number, *executor, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "claim", Data: result})
	return nil
}

func cmdPR(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "request" {
		return errors.New("usage: gia pr request --issue <number>")
	}
	fs := flag.NewFlagSet("pr request", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	number := fs.Int("issue", 0, "issue number")
	title := fs.String("title", "", "pull request title; defaults to Issue title")
	bodyFile := fs.String("body-file", "", "pull request body file that resolves within repository; defaults to Issue body")
	executor := fs.String("executor", "web-agent", "requesting executor")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *number <= 0 {
		return errors.New("--issue is required")
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	result, err := prepareDraftPullRequest(ctx, *repo, *number, *title, *bodyFile, *executor, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "pr request", Data: result})
	return nil
}

func prepareDraftPullRequest(ctx context.Context, repo string, issueNumber int, title, bodyFile, executor string, cfg config.Config) (prRequestData, error) {
	executor = strings.TrimSpace(executor)
	if !cfg.ExecutorAllowed(executor) {
		return prRequestData{}, fmt.Errorf("executor %q is not allowed by permissions.allowedExecutors", executor)
	}
	repo, err := filepath.Abs(repo)
	if err != nil {
		return prRequestData{}, err
	}
	base := strings.TrimSpace(cfg.DefaultBranch)
	if base == "" {
		return prRequestData{}, errors.New("defaultBranch is required")
	}
	branch, err := gitx.Branch(ctx, repo)
	if err != nil {
		return prRequestData{}, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return prRequestData{}, errors.New("cannot request a Draft PR from detached HEAD")
	}
	if branch == base {
		return prRequestData{}, fmt.Errorf("cannot request a Draft PR from default branch %q", base)
	}
	if config.MatchAny(cfg.Protected.Branches, branch) {
		return prRequestData{}, fmt.Errorf("cannot request a Draft PR from protected branch %q", branch)
	}
	clean, err := gitx.IsClean(ctx, repo)
	if err != nil {
		return prRequestData{}, err
	}
	if !clean {
		return prRequestData{}, errors.New("repository is dirty; commit or stash changes before requesting a Draft PR")
	}
	headSHA, err := gitx.Head(ctx, repo)
	if err != nil {
		return prRequestData{}, err
	}
	remote := cfg.ValidationRemote()
	if _, err := gitx.Run(ctx, repo, "fetch", "--prune", remote); err != nil {
		return prRequestData{}, err
	}
	remoteRef := "refs/remotes/" + remote + "/" + branch
	remoteSHA, err := gitx.Run(ctx, repo, "rev-parse", "--verify", remoteRef+"^{commit}")
	if err != nil {
		return prRequestData{}, fmt.Errorf("current branch %q is not available at %s; push it before requesting a Draft PR: %w", branch, remoteRef, err)
	}
	if headSHA != remoteSHA {
		return prRequestData{}, fmt.Errorf("local HEAD %s does not match remote branch tip %s at %s", headSHA, remoteSHA, remoteRef)
	}
	detail, err := getIssueDetail(ctx, repo, issueNumber)
	if err != nil {
		return prRequestData{}, err
	}
	if err := requireClaimedIssue(detail, executor, cfg); err != nil {
		return prRequestData{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = detail.Title
	}
	body := detail.Body
	if path := strings.TrimSpace(bodyFile); path != "" {
		data, err := readRepositoryFile(repo, path)
		if err != nil {
			return prRequestData{}, fmt.Errorf("read Draft PR body file %s: %w", path, err)
		}
		body = string(data)
	}
	created, err := requestDraftPullRequest(ctx, workflow.DraftPullRequestRequest{
		Repo:     repo,
		Base:     base,
		Head:     branch,
		Title:    title,
		Body:     body,
		Executor: executor,
	})
	if err != nil {
		return prRequestData{}, err
	}
	if created.State != "PENDING_USER_APPROVAL" {
		return prRequestData{}, fmt.Errorf("Draft PR returned unexpected workflow state %q", created.State)
	}
	return prRequestData{
		Number:   created.Number,
		URL:      created.URL,
		Draft:    true,
		State:    created.State,
		Issue:    issueNumber,
		Base:     created.Base,
		Head:     created.Head,
		HeadSHA:  headSHA,
		Title:    created.Title,
		Executor: created.Executor,
	}, nil
}

func requireClaimedIssue(detail issue.Detail, executor string, cfg config.Config) error {
	claimedLabel := strings.TrimSpace(cfg.Issue.ClaimedLabel)
	if claimedLabel == "" {
		return errors.New("issue.claimedLabel is required")
	}
	claimed := false
	for _, label := range detail.Labels {
		label = strings.TrimSpace(label)
		if strings.EqualFold(label, claimedLabel) {
			claimed = true
		}
		if strings.HasPrefix(strings.ToLower(label), "executor:") {
			labelExecutor := strings.TrimSpace(label[len("executor:"):])
			if !strings.EqualFold(labelExecutor, executor) {
				return fmt.Errorf("issue #%d executor label %q does not match requesting executor %q", detail.Number, label, executor)
			}
		}
	}
	if !claimed {
		return fmt.Errorf("issue #%d is not claimed; missing label %q", detail.Number, claimedLabel)
	}
	return nil
}

func readRepositoryFile(repo, path string) ([]byte, error) {
	candidate := filepath.Clean(path)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(repo, candidate)
	}
	repoPath, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return nil, fmt.Errorf("resolve repository path: %w", err)
	}
	candidatePath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(repoPath, candidatePath)
	if err != nil {
		return nil, err
	}
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path resolves outside repository: %s", candidatePath)
	}
	return os.ReadFile(candidatePath)
}

func cmdWorktree(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gia worktree create|remove")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("worktree create", flag.ContinueOnError)
		repo := fs.String("repo", ".", "repository")
		pr := fs.Int("pr", 0, "pull request number")
		ref := fs.String("ref", "", "remote ref when PR lookup is unavailable")
		root := fs.String("root", "", "worktree root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *pr <= 0 && *ref == "" {
			return errors.New("--pr or --ref is required")
		}
		services := legacy.NewServices(*repo, "")
		result, err := services.Workspace.Change(ctx, domain.WorkspaceRequest{Repository: *repo, Action: "create", PR: *pr, Ref: *ref, Root: *root})
		if err != nil {
			return err
		}
		emit(output{OK: true, Command: "worktree create", Data: result})
		return nil
	case "remove":
		fs := flag.NewFlagSet("worktree remove", flag.ContinueOnError)
		repo := fs.String("repo", ".", "repository")
		path := fs.String("path", "", "worktree path")
		force := fs.Bool("force", false, "remove dirty worktree")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *path == "" {
			return errors.New("--path is required")
		}
		services := legacy.NewServices(*repo, "")
		result, err := services.Workspace.Change(ctx, domain.WorkspaceRequest{Repository: *repo, Action: "remove", Path: *path, Force: *force})
		if err != nil {
			return err
		}
		emit(output{OK: true, Command: "worktree remove", Data: map[string]interface{}{"path": result.Path, "force": result.Force}})
		return nil
	default:
		return errors.New("usage: gia worktree create|remove")
	}
}

func cmdValidate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	profile := fs.String("profile", "full", "smoke|full|release")
	expected := fs.String("expected-head", "", "expected commit SHA")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	report, err := validate.Run(ctx, *repo, *profile, *expected, cfg)
	if err != nil {
		return emitFailure("validate", report, err)
	}
	if !report.OK {
		return emitFailure("validate", report, errors.New("validation failed"))
	}
	emit(output{OK: true, Command: "validate", Data: report})
	return nil
}

func cmdHandoff(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	pr := fs.Int("pr", 0, "pull request number")
	to := fs.String("to", "", "next executor")
	state := fs.String("state", "", "workflow state")
	note := fs.String("note", "", "handoff note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pr <= 0 || *to == "" || *state == "" {
		return errors.New("--pr, --to and --state are required")
	}
	result, err := workflow.Handoff(ctx, *repo, *pr, *to, *state, *note)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "handoff", Data: result})
	return nil
}

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := workflow.Status(ctx, *repo)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "status", Data: result})
	return nil
}

func cmdPlan(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gia plan create|show|diff|apply")
	}
	command := args[0]
	fs := flag.NewFlagSet("plan "+command, flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	input := fs.String("input", "", "repository-local JSON plan request")
	id := fs.String("id", "", "content-addressed plan id")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	planID := strings.TrimSpace(*id)
	if planID == "" && fs.NArg() == 1 {
		planID = strings.TrimSpace(fs.Arg(0))
	} else if fs.NArg() != 0 {
		return errors.New("plan accepts at most one positional id")
	}
	services := buildApplicationServices(*repo, *configPath)
	var (
		data any
		err  error
	)
	switch command {
	case "create":
		if strings.TrimSpace(*input) == "" {
			return errors.New("--input is required")
		}
		content, readErr := readRepositoryFile(*repo, *input)
		if readErr != nil {
			return readErr
		}
		var request domain.CreatePlanRequest
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&request); decodeErr != nil {
			return fmt.Errorf("decode plan request: %w", decodeErr)
		}
		var extra any
		if decodeErr := decoder.Decode(&extra); !errors.Is(decodeErr, io.EOF) {
			if decodeErr == nil {
				return errors.New("plan request must contain exactly one JSON value")
			}
			return fmt.Errorf("decode plan request: %w", decodeErr)
		}
		if strings.TrimSpace(request.Repository) == "" {
			request.Repository = *repo
		}
		data, err = services.Plans.Create(ctx, request)
	case "show":
		if planID == "" {
			return errors.New("plan id is required")
		}
		data, err = services.Plans.Show(ctx, domain.PlanID(planID))
	case "diff":
		if planID == "" {
			return errors.New("plan id is required")
		}
		data, err = services.Plans.Diff(ctx, domain.PlanID(planID))
	case "apply":
		if planID == "" {
			return errors.New("plan id is required")
		}
		data, err = services.Plans.Apply(ctx, domain.PlanID(planID))
	default:
		return errors.New("usage: gia plan create|show|diff|apply")
	}
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "plan " + command, Data: data})
	return nil
}

func cmdFeedback(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("feedback", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	category := fs.String("category", "improvement", "bug|improvement|ux|security")
	message := fs.String("message", "", "feedback text")
	pr := fs.Int("pr", 0, "related PR")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*message) == "" {
		return errors.New("--message is required")
	}
	result, err := workflow.RecordFeedback(ctx, *repo, *category, *message, *pr)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "feedback", Data: result})
	return nil
}

func cmdNotify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	configPath := addConfigFlag(fs)
	event := fs.String("event", "manual", "event name")
	message := fs.String("message", "", "message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*repo, *configPath)
	if err != nil {
		return err
	}
	result, err := notify.Send(ctx, *event, *message, cfg.Notifications)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "notify", Data: result})
	return nil
}

func addConfigFlag(fs *flag.FlagSet) *string {
	return fs.String("config", "", "explicit config path; overrides .gia config discovery")
}

func emitFailure(command string, data interface{}, err error) error {
	emit(output{OK: false, Command: command, Data: data, Error: err.Error()})
	return reportedError{err: err}
}

func emit(v output) {
	v.Command = strings.TrimSpace(v.Command)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(v)
}

func commandPath(args []string) string {
	if len(args) == 0 {
		return ""
	}
	command := args[0]
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		switch command {
		case "issue", "pr", "worktree", "plan":
			command += " " + args[1]
		}
	}
	return command
}

func usage() {
	fmt.Printf(`gia %s - Git Isolated Agent Kit

Commands:
  help       show top-level or command-specific help
  init       initialize .gia in a repository
  doctor     check dependencies and repository safety
  issue      create or list structured GitHub issues
  scan       find executable agent issues
  claim      atomically claim an issue and prepare a branch
  pr         request a Draft PR for user approval
  worktree   create/remove isolated validation worktrees
  validate   run smoke/full/release profile bound to a commit
  plan       create, inspect, compare, or apply an immutable execution plan
  handoff    transfer single-writer ownership through PR metadata
  status     inspect branch/worktree/repository state
  feedback   record iterative kit feedback
  notify     send configured notifications
  version    print version

Run "gia help <command>" or "gia <command> --help" for command usage.
`, version)
}

func requestedHelp(args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	if isHelpToken(args[0]) {
		return nil, true
	}
	if args[0] == "help" {
		return args[1:], true
	}
	if len(args) >= 2 && isHelpToken(args[1]) {
		return args[:1], true
	}
	if len(args) >= 3 && (args[0] == "issue" || args[0] == "pr" || args[0] == "worktree" || args[0] == "plan") && isHelpToken(args[2]) {
		return args[:2], true
	}
	return nil, false
}

func isHelpToken(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func showHelp(path []string) error {
	if len(path) == 0 {
		usage()
		return nil
	}
	key := strings.Join(path, " ")
	text, ok := commandHelp[key]
	if !ok {
		return fmt.Errorf("unknown help topic %q", key)
	}
	fmt.Print(text)
	return nil
}

var commandHelp = map[string]string{
	"init":            "Usage: gia init [--repo <path>] [--format json|yaml|yml] [--force]\nInitialize exactly one .gia configuration and the templates in a repository.\n",
	"doctor":          "Usage: gia doctor [--repo <path>] [--config <path>]\nCheck Git, GitHub CLI, Go, repository, authentication, identity mode, approval policy, and permission boundaries.\n",
	"issue":           "Usage: gia issue create|list\nCreate a structured task Issue or list ready Issues.\n",
	"issue create":    "Usage: gia issue create [--repo <path>] [--config <path>] --direction <text> [--risk auto|low|medium|high] [--executor <name>] [--dry-run]\n",
	"issue list":      "Usage: gia issue list [--repo <path>] [--config <path>] [--limit <count>]\nList open Issues carrying the configured ready label.\n",
	"scan":            "Usage: gia scan [--repo <path>] [--config <path>] [--limit <count>]\nFind open Issues carrying the configured ready label.\n",
	"claim":           "Usage: gia claim [--repo <path>] [--config <path>] --issue <number> [--executor <name>]\n",
	"pr":              "Usage: gia pr request\nRequest a Draft PR; approval, merge, and release remain external user operations.\n",
	"pr request":      "Usage: gia pr request [--repo <path>] [--config <path>] --issue <number> [--title <text>] [--body-file <path>] [--executor <name>]\n",
	"worktree":        "Usage: gia worktree create|remove\nCreate or safely remove an isolated validation worktree.\n",
	"worktree create": "Usage: gia worktree create [--repo <path>] (--pr <number> | --ref <remote-ref>) [--root <path>]\n",
	"worktree remove": "Usage: gia worktree remove [--repo <path>] --path <worktree-path> [--force]\n",
	"validate":        "Usage: gia validate [--repo <path>] [--config <path>] [--profile smoke|full|release] [--expected-head <sha>]\n",
	"plan":            "Usage: gia plan create|show|diff|apply\nPersist and inspect an immutable plan before explicitly applying it.\n",
	"plan create":     "Usage: gia plan create [--repo <path>] [--config <path>] --input <request.json>\n",
	"plan show":       "Usage: gia plan show [--repo <path>] [--config <path>] <plan-id>\n",
	"plan diff":       "Usage: gia plan diff [--repo <path>] [--config <path>] <plan-id>\n",
	"plan apply":      "Usage: gia plan apply [--repo <path>] [--config <path>] <plan-id>\n",
	"handoff":         "Usage: gia handoff [--repo <path>] --pr <number> --to <executor> --state <state> [--note <text>]\n",
	"status":          "Usage: gia status [--repo <path>]\nShow branch, HEAD, cleanliness, and worktree state.\n",
	"feedback":        "Usage: gia feedback [--repo <path>] [--category bug|improvement|ux|security] --message <text> [--pr <number>]\n",
	"notify":          "Usage: gia notify [--repo <path>] [--config <path>] [--event <name>] [--message <text>]\n",
	"version":         "Usage: gia version\nPrint the GIA Kit version.\n",
}
