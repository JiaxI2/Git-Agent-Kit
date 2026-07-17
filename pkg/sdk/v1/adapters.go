package v1

import (
	"context"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

func buildServices(ports Ports) app.Services {
	var tasks app.TaskRepository
	if ports.Tasks != nil {
		tasks = taskStoreAdapter{port: ports.Tasks}
	}
	var issues app.IssuePort
	if ports.Issues != nil {
		issues = issueAdapter{port: ports.Issues}
	}
	var claimer app.TaskClaimer
	if ports.Claimer != nil {
		claimer = claimerAdapter{port: ports.Claimer}
	}
	var validation app.ValidationPlanner
	if ports.Validation != nil {
		validation = validationAdapter{port: ports.Validation}
	}
	var workspace app.WorkspacePort
	if ports.Workspace != nil {
		workspace = workspaceAdapter{port: ports.Workspace}
	}
	var pullRequests app.PullRequestPort
	if ports.PullRequests != nil {
		pullRequests = pullRequestAdapter{port: ports.PullRequests}
	}
	var repository app.RepositoryPort
	if ports.Repository != nil {
		repository = repositoryAdapter{port: ports.Repository}
	}
	var policy app.Policy
	if ports.Policy != nil {
		policy = policyAdapter{port: ports.Policy}
	}
	executors := make([]app.Executor, 0, len(ports.Executors))
	for _, executor := range ports.Executors {
		if executor != nil {
			executors = append(executors, executorAdapter{port: executor})
		}
	}
	var selector app.ExecutorSelector
	if len(executors) > 0 {
		selector = app.CapabilitySelector{Executors: executors}
	}
	var evidence app.EvidenceRepository
	if ports.Evidence != nil {
		evidence = evidenceAdapter{port: ports.Evidence}
	}
	var clock app.Clock
	if ports.Clock != nil {
		clock = clockAdapter{port: ports.Clock}
	}
	return app.Services{
		Tasks:        app.TaskService{Tasks: tasks, Issues: issues, Claimer: claimer, Policy: policy},
		Validation:   app.ValidationService{Tasks: tasks, Planner: validation},
		Workspace:    app.WorkspaceService{Workspace: workspace},
		PullRequests: app.PRService{PullRequests: pullRequests},
		Repository:   app.RepositoryService{Repository: repository},
		Execution:    app.ExecutionService{Selector: selector, Evidence: evidence, Clock: clock},
	}
}

type taskStoreAdapter struct{ port TaskStore }

func (a taskStoreAdapter) List(ctx context.Context) ([]domain.Task, error) {
	tasks, err := a.port.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, importTask(task))
	}
	return result, nil
}
func (a taskStoreAdapter) Get(ctx context.Context, id domain.TaskID) (domain.Task, error) {
	task, err := a.port.Get(ctx, string(id))
	return importTask(task), err
}
func (a taskStoreAdapter) Save(ctx context.Context, task domain.Task) error {
	return a.port.Save(ctx, exportTask(task))
}

type issueAdapter struct{ port IssueGateway }

func (a issueAdapter) Create(ctx context.Context, request domain.CreateTaskRequest) (domain.Task, error) {
	task, err := a.port.Create(ctx, exportCreateTaskRequest(request))
	return importTask(task), err
}

type claimerAdapter struct{ port TaskClaimer }

func (a claimerAdapter) Claim(ctx context.Context, task domain.Task, executor domain.ExecutorIdentity) (domain.Task, error) {
	claimed, err := a.port.Claim(ctx, exportTask(task), exportExecutorIdentity(executor))
	return importTask(claimed), err
}

type validationAdapter struct{ port ValidationPlanner }

func (a validationAdapter) Plan(ctx context.Context, task domain.Task, profile string) (domain.ValidationPlan, error) {
	plan, err := a.port.Plan(ctx, exportTask(task), profile)
	return importValidationPlan(plan), err
}

type workspaceAdapter struct{ port WorkspaceGateway }

func (a workspaceAdapter) Change(ctx context.Context, request domain.WorkspaceRequest) (domain.WorkspaceResult, error) {
	result, err := a.port.Change(ctx, exportWorkspaceRequest(request))
	return importWorkspaceResult(result), err
}

type pullRequestAdapter struct{ port PullRequestGateway }

func (a pullRequestAdapter) Request(ctx context.Context, request domain.PullRequestRequest) (domain.PullRequestResult, error) {
	result, err := a.port.Request(ctx, exportPullRequestRequest(request))
	return importPullRequestResult(result), err
}

type repositoryAdapter struct{ port RepositoryGateway }

func (a repositoryAdapter) Inspect(ctx context.Context, request domain.RepositoryRequest) (domain.RepositoryReport, error) {
	report, err := a.port.Inspect(ctx, exportRepositoryRequest(request))
	return importRepositoryReport(report), err
}

type policyAdapter struct{ port Policy }

func (a policyAdapter) Evaluate(ctx context.Context, task domain.Task, operation domain.Operation) domain.PolicyDecision {
	decision := a.port.Evaluate(ctx, exportTask(task), string(operation))
	return domain.PolicyDecision{Allowed: decision.Allowed, Reasons: append([]string(nil), decision.Reasons...)}
}

type executorAdapter struct{ port Executor }

func (a executorAdapter) Identity() domain.ExecutorIdentity {
	return importExecutorIdentity(a.port.Identity())
}
func (a executorAdapter) Execute(ctx context.Context, task domain.Task, effects []domain.Effect) ([]domain.Evidence, error) {
	publicEffects := make([]Effect, 0, len(effects))
	for _, effect := range effects {
		publicEffects = append(publicEffects, exportEffect(effect))
	}
	evidence, err := a.port.Execute(ctx, exportTask(task), publicEffects)
	return importEvidence(evidence), err
}

type evidenceAdapter struct{ port EvidenceStore }

func (a evidenceAdapter) Append(ctx context.Context, id domain.TaskID, evidence []domain.Evidence) error {
	return a.port.Append(ctx, string(id), exportEvidence(evidence))
}

type clockAdapter struct{ port Clock }

func (a clockAdapter) Now() time.Time { return a.port.Now() }

func importTask(task Task) domain.Task {
	var executor *domain.ExecutorIdentity
	if task.Executor != nil {
		value := importExecutorIdentity(*task.Executor)
		executor = &value
	}
	return domain.Task{
		ID: domain.TaskID(task.ID), Title: task.Title, Description: task.Description,
		State: domain.TaskState(task.State), Risk: domain.Risk(task.Risk), Mode: domain.ExecutionMode(task.Mode),
		Executor: executor, Branch: task.Branch, Issue: task.Issue, URL: task.URL,
		Labels: append([]string(nil), task.Labels...), Metadata: cloneStringMap(task.Metadata),
	}
}

func exportTask(task domain.Task) Task {
	var executor *ExecutorIdentity
	if task.Executor != nil {
		value := exportExecutorIdentity(*task.Executor)
		executor = &value
	}
	return Task{
		ID: string(task.ID), Title: task.Title, Description: task.Description,
		State: TaskState(task.State), Risk: Risk(task.Risk), Mode: ExecutionMode(task.Mode),
		Executor: executor, Branch: task.Branch, Issue: task.Issue, URL: task.URL,
		Labels: append([]string(nil), task.Labels...), Metadata: cloneStringMap(task.Metadata),
	}
}

func exportTasks(tasks []domain.Task) []Task {
	result := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, exportTask(task))
	}
	return result
}

func importExecutorIdentity(identity ExecutorIdentity) domain.ExecutorIdentity {
	capabilities := make([]domain.Capability, 0, len(identity.Capabilities))
	for _, capability := range identity.Capabilities {
		capabilities = append(capabilities, domain.Capability(capability))
	}
	return domain.ExecutorIdentity{Name: identity.Name, Mode: domain.ExecutionMode(identity.Mode), Capabilities: capabilities, Metadata: cloneAnyMap(identity.Metadata)}
}

func exportExecutorIdentity(identity domain.ExecutorIdentity) ExecutorIdentity {
	capabilities := make([]Capability, 0, len(identity.Capabilities))
	for _, capability := range identity.Capabilities {
		capabilities = append(capabilities, Capability(capability))
	}
	return ExecutorIdentity{Name: identity.Name, Mode: ExecutionMode(identity.Mode), Capabilities: capabilities, Metadata: cloneAnyMap(identity.Metadata)}
}

func importCreateTaskRequest(request CreateTaskRequest) domain.CreateTaskRequest {
	return domain.CreateTaskRequest{Title: request.Title, Description: request.Description, Risk: domain.Risk(request.Risk), PreferredExecutor: request.PreferredExecutor, Mode: domain.ExecutionMode(request.Mode)}
}
func exportCreateTaskRequest(request domain.CreateTaskRequest) CreateTaskRequest {
	return CreateTaskRequest{Title: request.Title, Description: request.Description, Risk: Risk(request.Risk), PreferredExecutor: request.PreferredExecutor, Mode: ExecutionMode(request.Mode)}
}

func importEffect(effect Effect) domain.Effect {
	requires := make([]domain.Capability, 0, len(effect.Requires))
	for _, capability := range effect.Requires {
		requires = append(requires, domain.Capability(capability))
	}
	return domain.Effect{Kind: domain.EffectKind(effect.Kind), Target: effect.Target, Workdir: effect.Workdir, Command: effect.Command, Args: append([]string(nil), effect.Args...), Parameters: cloneStringMap(effect.Parameters), Requires: requires}
}
func exportEffect(effect domain.Effect) Effect {
	requires := make([]Capability, 0, len(effect.Requires))
	for _, capability := range effect.Requires {
		requires = append(requires, Capability(capability))
	}
	return Effect{Kind: string(effect.Kind), Target: effect.Target, Workdir: effect.Workdir, Command: effect.Command, Args: append([]string(nil), effect.Args...), Parameters: cloneStringMap(effect.Parameters), Requires: requires}
}

func importValidationPlan(plan ValidationPlan) domain.ValidationPlan {
	effects := make([]domain.Effect, 0, len(plan.Effects))
	for _, effect := range plan.Effects {
		effects = append(effects, importEffect(effect))
	}
	requires := make([]domain.Capability, 0, len(plan.Requires))
	for _, capability := range plan.Requires {
		requires = append(requires, domain.Capability(capability))
	}
	return domain.ValidationPlan{TaskID: domain.TaskID(plan.TaskID), Profile: plan.Profile, Effects: effects, Checks: append([]string(nil), plan.Checks...), Requires: requires}
}
func exportValidationPlan(plan domain.ValidationPlan) ValidationPlan {
	effects := make([]Effect, 0, len(plan.Effects))
	for _, effect := range plan.Effects {
		effects = append(effects, exportEffect(effect))
	}
	requires := make([]Capability, 0, len(plan.Requires))
	for _, capability := range plan.Requires {
		requires = append(requires, Capability(capability))
	}
	return ValidationPlan{TaskID: string(plan.TaskID), Profile: plan.Profile, Effects: effects, Checks: append([]string(nil), plan.Checks...), Requires: requires}
}

func importWorkspaceRequest(request WorkspaceRequest) domain.WorkspaceRequest {
	return domain.WorkspaceRequest{Repository: request.Repository, Action: request.Action, PR: request.PR, Ref: request.Ref, Root: request.Root, Path: request.Path, Force: request.Force}
}
func exportWorkspaceRequest(request domain.WorkspaceRequest) WorkspaceRequest {
	return WorkspaceRequest{Repository: request.Repository, Action: request.Action, PR: request.PR, Ref: request.Ref, Root: request.Root, Path: request.Path, Force: request.Force}
}
func importWorkspaceResult(result WorkspaceResult) domain.WorkspaceResult {
	return domain.WorkspaceResult{Path: result.Path, Branch: result.Branch, Ref: result.Ref, Head: result.Head, Force: result.Force}
}
func exportWorkspaceResult(result domain.WorkspaceResult) WorkspaceResult {
	return WorkspaceResult{Path: result.Path, Branch: result.Branch, Ref: result.Ref, Head: result.Head, Force: result.Force}
}

func importPullRequestRequest(request PullRequestRequest) domain.PullRequestRequest {
	return domain.PullRequestRequest{Repository: request.Repository, Base: request.Base, Head: request.Head, Title: request.Title, Body: request.Body, Executor: request.Executor}
}
func exportPullRequestRequest(request domain.PullRequestRequest) PullRequestRequest {
	return PullRequestRequest{Repository: request.Repository, Base: request.Base, Head: request.Head, Title: request.Title, Body: request.Body, Executor: request.Executor}
}
func importPullRequestResult(result PullRequestResult) domain.PullRequestResult {
	return domain.PullRequestResult{Number: result.Number, URL: result.URL, State: result.State, Base: result.Base, Head: result.Head, Title: result.Title, Executor: result.Executor}
}
func exportPullRequestResult(result domain.PullRequestResult) PullRequestResult {
	return PullRequestResult{Number: result.Number, URL: result.URL, State: result.State, Base: result.Base, Head: result.Head, Title: result.Title, Executor: result.Executor}
}

func importRepositoryRequest(request RepositoryRequest) domain.RepositoryRequest {
	return domain.RepositoryRequest{Repository: request.Repository, ConfigPath: request.ConfigPath, Mode: request.Mode}
}
func exportRepositoryRequest(request domain.RepositoryRequest) RepositoryRequest {
	return RepositoryRequest{Repository: request.Repository, ConfigPath: request.ConfigPath, Mode: request.Mode}
}
func importRepositoryReport(report RepositoryReport) domain.RepositoryReport {
	checks := make([]domain.Check, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, domain.Check{Name: check.Name, OK: check.OK, Detail: check.Detail})
	}
	return domain.RepositoryReport{OK: report.OK, Repo: report.Repo, Branch: report.Branch, Head: report.Head, Clean: report.Clean, Worktrees: report.Worktrees, Checks: checks}
}
func exportRepositoryReport(report domain.RepositoryReport) RepositoryReport {
	checks := make([]Check, 0, len(report.Checks))
	for _, check := range report.Checks {
		checks = append(checks, Check{Name: check.Name, OK: check.OK, Detail: check.Detail})
	}
	return RepositoryReport{OK: report.OK, Repo: report.Repo, Branch: report.Branch, Head: report.Head, Clean: report.Clean, Worktrees: report.Worktrees, Checks: checks}
}

func importEvidence(items []Evidence) []domain.Evidence {
	result := make([]domain.Evidence, 0, len(items))
	for _, item := range items {
		result = append(result, domain.Evidence{Kind: item.Kind, Source: item.Source, Summary: item.Summary, Reference: item.Reference, Observed: item.Observed, Metadata: cloneAnyMap(item.Metadata)})
	}
	return result
}
func exportEvidence(items []domain.Evidence) []Evidence {
	result := make([]Evidence, 0, len(items))
	for _, item := range items {
		result = append(result, Evidence{Kind: item.Kind, Source: item.Source, Summary: item.Summary, Reference: item.Reference, Observed: item.Observed, Metadata: cloneAnyMap(item.Metadata)})
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
