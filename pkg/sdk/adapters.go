package sdk

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
	var plans app.PlanRepository
	if ports.Plans != nil {
		plans = planStoreAdapter{port: ports.Plans}
	}
	var planContext app.PlanContext
	if ports.PlanContext != nil {
		planContext = planContextAdapter{port: ports.PlanContext}
	}
	var clock app.Clock
	if ports.Clock != nil {
		clock = clockAdapter{port: ports.Clock}
	}
	execution := app.ExecutionService{Selector: selector, Evidence: evidence, Clock: clock}
	return app.Services{
		Tasks:        app.TaskService{Tasks: tasks, Issues: issues, Claimer: claimer, Policy: policy},
		Validation:   app.ValidationService{Tasks: tasks, Planner: validation},
		Workspace:    app.WorkspaceService{Workspace: workspace},
		PullRequests: app.PRService{PullRequests: pullRequests},
		Repository:   app.RepositoryService{Repository: repository},
		Execution:    execution,
		Plans: app.PlanService{
			Plans: plans, Context: planContext, Policy: policy, Execution: execution, Clock: clock,
		},
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

type planStoreAdapter struct{ port PlanStore }

func (a planStoreAdapter) Create(ctx context.Context, plan domain.Plan) error {
	return a.port.Create(ctx, exportPlan(plan))
}
func (a planStoreAdapter) Get(ctx context.Context, id domain.PlanID) (domain.PlanRecord, error) {
	record, err := a.port.Get(ctx, PlanID(id))
	return importPlanRecord(record), err
}
func (a planStoreAdapter) BeginApply(ctx context.Context, id domain.PlanID, started time.Time) error {
	return a.port.BeginApply(ctx, PlanID(id), started)
}
func (a planStoreAdapter) FinishApply(ctx context.Context, result domain.PlanApplyResult, finished time.Time) error {
	return a.port.FinishApply(ctx, exportPlanApplyResult(result), finished)
}

type planContextAdapter struct{ port PlanContext }

func (a planContextAdapter) Snapshot(ctx context.Context, repository string) (domain.PlanSnapshot, error) {
	snapshot, err := a.port.Snapshot(ctx, repository)
	return importPlanSnapshot(snapshot), err
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
		result = append(result, domain.Evidence{
			Kind: item.Kind, Source: item.Source, Summary: item.Summary, Reference: item.Reference, Observed: item.Observed,
			PlanID: domain.PlanID(item.PlanID), BaseHead: item.BaseHead, ConfigDigest: item.ConfigDigest, Metadata: cloneAnyMap(item.Metadata),
		})
	}
	return result
}
func exportEvidence(items []domain.Evidence) []Evidence {
	result := make([]Evidence, 0, len(items))
	for _, item := range items {
		result = append(result, Evidence{
			Kind: item.Kind, Source: item.Source, Summary: item.Summary, Reference: item.Reference, Observed: item.Observed,
			PlanID: PlanID(item.PlanID), BaseHead: item.BaseHead, ConfigDigest: item.ConfigDigest, Metadata: cloneAnyMap(item.Metadata),
		})
	}
	return result
}

func importCreatePlanRequest(request CreatePlanRequest) domain.CreatePlanRequest {
	effects := make([]domain.Effect, 0, len(request.Effects))
	for _, effect := range request.Effects {
		effects = append(effects, importEffect(effect))
	}
	return domain.CreatePlanRequest{
		Repository: request.Repository, Task: importTask(request.Task), Effects: effects,
		PreferredMode: domain.ExecutionMode(request.PreferredMode),
	}
}

func importPlan(plan Plan) domain.Plan {
	effects := make([]domain.Effect, 0, len(plan.Effects))
	for _, effect := range plan.Effects {
		effects = append(effects, importEffect(effect))
	}
	decisions := make([]domain.PlanPolicyDecision, 0, len(plan.PolicyDecisions))
	for _, decision := range plan.PolicyDecisions {
		decisions = append(decisions, domain.PlanPolicyDecision{
			Operation: domain.Operation(decision.Operation),
			Decision:  domain.PolicyDecision{Allowed: decision.Decision.Allowed, Reasons: append([]string(nil), decision.Decision.Reasons...)},
		})
	}
	requires := make([]domain.Capability, 0, len(plan.Requires))
	for _, capability := range plan.Requires {
		requires = append(requires, domain.Capability(capability))
	}
	return domain.Plan{
		ID: domain.PlanID(plan.ID), Repository: plan.Repository, BaseHead: plan.BaseHead,
		Task: importTask(plan.Task), Effects: effects, PolicyDecisions: decisions, Requires: requires,
		ConfigDigest: plan.ConfigDigest, PreferredMode: domain.ExecutionMode(plan.PreferredMode), CreatedAt: plan.CreatedAt,
	}
}

func exportPlan(plan domain.Plan) Plan {
	effects := make([]Effect, 0, len(plan.Effects))
	for _, effect := range plan.Effects {
		effects = append(effects, exportEffect(effect))
	}
	decisions := make([]PlanPolicyDecision, 0, len(plan.PolicyDecisions))
	for _, decision := range plan.PolicyDecisions {
		decisions = append(decisions, PlanPolicyDecision{
			Operation: string(decision.Operation),
			Decision:  PolicyDecision{Allowed: decision.Decision.Allowed, Reasons: append([]string(nil), decision.Decision.Reasons...)},
		})
	}
	requires := make([]Capability, 0, len(plan.Requires))
	for _, capability := range plan.Requires {
		requires = append(requires, Capability(capability))
	}
	return Plan{
		ID: PlanID(plan.ID), Repository: plan.Repository, BaseHead: plan.BaseHead,
		Task: exportTask(plan.Task), Effects: effects, PolicyDecisions: decisions, Requires: requires,
		ConfigDigest: plan.ConfigDigest, PreferredMode: ExecutionMode(plan.PreferredMode), CreatedAt: plan.CreatedAt,
	}
}

func importPlanSnapshot(snapshot PlanSnapshot) domain.PlanSnapshot {
	return domain.PlanSnapshot{Repository: snapshot.Repository, Head: snapshot.Head, ConfigDigest: snapshot.ConfigDigest}
}

func exportPlanSnapshot(snapshot domain.PlanSnapshot) PlanSnapshot {
	return PlanSnapshot{Repository: snapshot.Repository, Head: snapshot.Head, ConfigDigest: snapshot.ConfigDigest}
}

func importPlanRecord(record PlanRecord) domain.PlanRecord {
	return domain.PlanRecord{
		Plan: importPlan(record.Plan),
		Status: domain.PlanStatus{
			PlanID: domain.PlanID(record.Status.PlanID), State: domain.PlanState(record.Status.State),
			StartedAt: record.Status.StartedAt, FinishedAt: record.Status.FinishedAt, Error: record.Status.Error,
		},
	}
}

func exportPlanRecord(record domain.PlanRecord) PlanRecord {
	return PlanRecord{
		Plan: exportPlan(record.Plan),
		Status: PlanStatus{
			PlanID: PlanID(record.Status.PlanID), State: PlanState(record.Status.State),
			StartedAt: record.Status.StartedAt, FinishedAt: record.Status.FinishedAt, Error: record.Status.Error,
		},
	}
}

func exportPlanDiff(diff domain.PlanDiff) PlanDiff {
	return PlanDiff{
		PlanID: PlanID(diff.PlanID), Expected: exportPlanSnapshot(diff.Expected), Actual: exportPlanSnapshot(diff.Actual),
		Status: PlanState(diff.Status), HeadMatches: diff.HeadMatches, ConfigMatches: diff.ConfigMatches,
		Ready: diff.Ready, Reasons: append([]string(nil), diff.Reasons...),
	}
}

func exportPlanApplyResult(result domain.PlanApplyResult) PlanApplyResult {
	return PlanApplyResult{
		PlanID: PlanID(result.PlanID), State: PlanState(result.State),
		Decision: PlanPolicyDecision{
			Operation: string(result.Decision.Operation),
			Decision:  PolicyDecision{Allowed: result.Decision.Decision.Allowed, Reasons: append([]string(nil), result.Decision.Decision.Reasons...)},
		},
		Evidence: exportEvidence(result.Evidence), Error: result.Error,
	}
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
