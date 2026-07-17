// Package legacy maps existing GIA production behavior to application ports.
package legacy

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/adapters/planstore"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/execution"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/gitx"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/issue"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/workflow"
)

type Adapter struct {
	Repository string
	ConfigPath string
}

func NewServices(repository, configPath string) app.Services {
	adapter := &Adapter{Repository: repository, ConfigPath: configPath}
	plans := planstore.New(repository, configPath)
	executionService := app.ExecutionService{Selector: app.CapabilitySelector{Executors: []app.Executor{
		execution.LocalExecutor{Name: "gia-local", Root: repository},
	}}}
	return app.Services{
		Tasks:        app.TaskService{Tasks: adapter, Issues: adapter, Claimer: adapter, Policy: adapter},
		Validation:   app.ValidationService{Tasks: adapter, Planner: adapter},
		Workspace:    app.WorkspaceService{Workspace: adapter},
		PullRequests: app.PRService{PullRequests: adapter},
		Repository:   app.RepositoryService{Repository: adapter},
		Execution:    executionService,
		Plans: app.PlanService{
			Plans: plans, Context: plans, Policy: adapter, Execution: executionService,
		},
	}
}

func (a *Adapter) Evaluate(_ context.Context, task domain.Task, operation domain.Operation) domain.PolicyDecision {
	cfg, err := a.config()
	if err != nil {
		return domain.PolicyDecision{Allowed: false, Reasons: []string{err.Error()}}
	}
	if task.Executor != nil && !cfg.ExecutorAllowed(task.Executor.Name) {
		return domain.PolicyDecision{Allowed: false, Reasons: []string{fmt.Sprintf("executor %q is not allowed", task.Executor.Name)}}
	}
	return domain.PolicyDecision{Allowed: true, Reasons: []string{fmt.Sprintf("%s allowed by repository policy", operation)}}
}

func (a *Adapter) List(ctx context.Context) ([]domain.Task, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, err
	}
	items, err := issue.Scan(ctx, a.Repository, 20, cfg)
	if err != nil {
		return nil, err
	}
	tasks := make([]domain.Task, 0, len(items))
	for _, item := range items {
		tasks = append(tasks, taskFromItem(item, cfg))
	}
	return tasks, nil
}

func (a *Adapter) Get(ctx context.Context, id domain.TaskID) (domain.Task, error) {
	number, err := strconv.Atoi(string(id))
	if err != nil || number <= 0 {
		return domain.Task{}, fmt.Errorf("task id %q is not an Issue number", id)
	}
	cfg, err := a.config()
	if err != nil {
		return domain.Task{}, err
	}
	detail, err := issue.Get(ctx, a.Repository, number)
	if err != nil {
		return domain.Task{}, err
	}
	return taskFromDetail(detail, cfg), nil
}

func (*Adapter) Save(context.Context, domain.Task) error {
	return errors.New("legacy task persistence requires the atomic claim port")
}

func (a *Adapter) Create(ctx context.Context, request domain.CreateTaskRequest) (domain.Task, error) {
	cfg, err := a.config()
	if err != nil {
		return domain.Task{}, err
	}
	direction := strings.TrimSpace(request.Description)
	if direction == "" {
		direction = request.Title
	}
	spec := issue.FromDirection(direction, string(request.Risk), request.PreferredExecutor, cfg)
	if strings.TrimSpace(request.Title) != "" {
		spec.Title = strings.TrimSpace(request.Title)
	}
	created, err := issue.Create(ctx, a.Repository, spec, cfg)
	if err != nil {
		return domain.Task{}, err
	}
	return domain.Task{
		ID: domain.TaskID(strconv.Itoa(created.Number)), Title: created.Title, State: domain.TaskReady,
		Risk: request.Risk, Mode: domain.ExecutionRemote, Issue: created.Number, URL: created.URL,
	}, nil
}

func (a *Adapter) Claim(ctx context.Context, task domain.Task, executor domain.ExecutorIdentity) (domain.Task, error) {
	cfg, err := a.config()
	if err != nil {
		return domain.Task{}, err
	}
	if !cfg.ExecutorAllowed(executor.Name) {
		return domain.Task{}, fmt.Errorf("executor %q is not allowed by permissions.allowedExecutors", executor.Name)
	}
	number := task.Issue
	if number == 0 {
		number, err = strconv.Atoi(string(task.ID))
		if err != nil || number <= 0 {
			return domain.Task{}, fmt.Errorf("task id %q is not an Issue number", task.ID)
		}
	}
	result, err := workflow.Claim(ctx, a.Repository, number, executor.Name, cfg)
	if err != nil {
		return domain.Task{}, err
	}
	task.State = domain.TaskClaimed
	task.Mode = executor.Mode
	task.Executor = &executor
	task.Issue = result.Issue
	task.Branch = result.Branch
	task.Metadata = map[string]string{"head": result.Head, "legacyState": result.State}
	return task, nil
}

func (a *Adapter) Plan(_ context.Context, task domain.Task, profile string) (domain.ValidationPlan, error) {
	cfg, err := a.config()
	if err != nil {
		return domain.ValidationPlan{}, err
	}
	commands, ok := cfg.Validation.Profiles[profile]
	if !ok {
		return domain.ValidationPlan{}, fmt.Errorf("unknown validation profile %q", profile)
	}
	plan := domain.ValidationPlan{TaskID: task.ID, Profile: profile}
	required := map[domain.Capability]bool{domain.CapabilityCommand: true}
	for _, command := range commands {
		capabilities := commandCapabilities(command)
		for _, capability := range capabilities {
			required[capability] = true
		}
		name, args := platformShell(command)
		plan.Effects = append(plan.Effects, domain.Effect{
			Kind: domain.EffectCommand, Command: name, Args: args, Requires: capabilities,
		})
		plan.Checks = append(plan.Checks, command)
	}
	for _, capability := range []domain.Capability{
		domain.CapabilityCommand, domain.CapabilityGit, domain.CapabilityBuild, domain.CapabilityTest,
	} {
		if required[capability] {
			plan.Requires = append(plan.Requires, capability)
		}
	}
	return plan, nil
}

func (a *Adapter) Change(ctx context.Context, request domain.WorkspaceRequest) (domain.WorkspaceResult, error) {
	switch request.Action {
	case "create":
		result, err := workflow.CreateValidationWorktree(ctx, request.Repository, request.PR, request.Ref, request.Root)
		if err != nil {
			return domain.WorkspaceResult{}, err
		}
		return domain.WorkspaceResult{Path: result.Path, Branch: result.Branch, Ref: result.Ref, Head: result.Head}, nil
	case "remove":
		if strings.TrimSpace(request.Path) == "" {
			return domain.WorkspaceResult{}, errors.New("workspace path is required")
		}
		if err := gitx.RemoveWorktree(ctx, request.Repository, request.Path, request.Force); err != nil {
			return domain.WorkspaceResult{}, err
		}
		return domain.WorkspaceResult{Path: request.Path, Force: request.Force}, nil
	default:
		return domain.WorkspaceResult{}, fmt.Errorf("unsupported workspace action %q", request.Action)
	}
}

func (*Adapter) Request(ctx context.Context, request domain.PullRequestRequest) (domain.PullRequestResult, error) {
	result, err := workflow.RequestDraftPullRequest(ctx, workflow.DraftPullRequestRequest{
		Repo: request.Repository, Base: request.Base, Head: request.Head,
		Title: request.Title, Body: request.Body, Executor: request.Executor,
	})
	if err != nil {
		return domain.PullRequestResult{}, err
	}
	return domain.PullRequestResult{
		Number: result.Number, URL: result.URL, State: result.State, Base: result.Base,
		Head: result.Head, Title: result.Title, Executor: result.Executor,
	}, nil
}

func (a *Adapter) Inspect(ctx context.Context, request domain.RepositoryRequest) (domain.RepositoryReport, error) {
	switch request.Mode {
	case "doctor":
		report := workflow.DoctorWithConfig(ctx, request.Repository, request.ConfigPath)
		checks := make([]domain.Check, 0, len(report.Checks))
		for _, check := range report.Checks {
			checks = append(checks, domain.Check{Name: check.Name, OK: check.OK, Detail: check.Detail})
		}
		return domain.RepositoryReport{OK: report.OK, Checks: checks}, nil
	case "inspect":
		result, err := workflow.Status(ctx, request.Repository)
		if err != nil {
			return domain.RepositoryReport{}, err
		}
		return domain.RepositoryReport{
			OK: true, Repo: result.Repo, Branch: result.Branch, Head: result.Head,
			Clean: result.Clean, Worktrees: result.Worktrees,
		}, nil
	default:
		return domain.RepositoryReport{}, fmt.Errorf("unsupported repository inspection mode %q", request.Mode)
	}
}

func (a *Adapter) config() (config.Config, error) {
	return config.Load(a.Repository, a.ConfigPath)
}

func taskFromItem(item issue.Item, cfg config.Config) domain.Task {
	return taskFromLabels(item.Number, item.Title, "", item.URL, item.Labels, cfg)
}

func taskFromDetail(detail issue.Detail, cfg config.Config) domain.Task {
	return taskFromLabels(detail.Number, detail.Title, detail.Body, detail.URL, detail.Labels, cfg)
}

func taskFromLabels(number int, title, description, url string, labels []string, cfg config.Config) domain.Task {
	state := domain.TaskReady
	risk := domain.RiskMedium
	var executor *domain.ExecutorIdentity
	for _, label := range labels {
		switch label {
		case cfg.Issue.ClaimedLabel:
			state = domain.TaskClaimed
		case cfg.Issue.CompletedLabel:
			state = domain.TaskDone
		case "risk:low":
			risk = domain.RiskLow
		case "risk:high":
			risk = domain.RiskHigh
		}
		if strings.HasPrefix(label, "executor:") {
			name := strings.TrimSpace(strings.TrimPrefix(label, "executor:"))
			if name != "" {
				executor = &domain.ExecutorIdentity{Name: name, Mode: domain.ExecutionRemote, Capabilities: []domain.Capability{domain.CapabilityIssue, domain.CapabilityBranch}}
			}
		}
	}
	return domain.Task{
		ID: domain.TaskID(strconv.Itoa(number)), Title: title, Description: description,
		State: state, Risk: risk, Mode: domain.ExecutionRemote, Executor: executor,
		Issue: number, URL: url, Labels: append([]string(nil), labels...),
	}
}

func commandCapabilities(command string) []domain.Capability {
	capabilities := []domain.Capability{domain.CapabilityCommand}
	trimmed := strings.TrimSpace(command)
	if strings.HasPrefix(trimmed, "git ") {
		capabilities = append(capabilities, domain.CapabilityGit)
	}
	if strings.Contains(trimmed, "go test") {
		capabilities = append(capabilities, domain.CapabilityTest)
	}
	if strings.Contains(trimmed, "go build") {
		capabilities = append(capabilities, domain.CapabilityBuild)
	}
	return capabilities
}

func platformShell(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/d", "/s", "/c", command}
	}
	return "sh", []string{"-lc", command}
}
