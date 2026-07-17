// Package app coordinates domain behavior through executor-neutral ports.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type TaskRepository interface {
	List(context.Context) ([]domain.Task, error)
	Get(context.Context, domain.TaskID) (domain.Task, error)
	Save(context.Context, domain.Task) error
}

type IssuePort interface {
	Create(context.Context, domain.CreateTaskRequest) (domain.Task, error)
}

type TaskClaimer interface {
	Claim(context.Context, domain.Task, domain.ExecutorIdentity) (domain.Task, error)
}

type ValidationPlanner interface {
	Plan(context.Context, domain.Task, string) (domain.ValidationPlan, error)
}

type WorkspacePort interface {
	Change(context.Context, domain.WorkspaceRequest) (domain.WorkspaceResult, error)
}

type PullRequestPort interface {
	Request(context.Context, domain.PullRequestRequest) (domain.PullRequestResult, error)
}

type RepositoryPort interface {
	Inspect(context.Context, domain.RepositoryRequest) (domain.RepositoryReport, error)
}

type Policy interface {
	Evaluate(context.Context, domain.Task, domain.Operation) domain.PolicyDecision
}

type Executor interface {
	Identity() domain.ExecutorIdentity
	Execute(context.Context, domain.Task, []domain.Effect) ([]domain.Evidence, error)
}

type ExecutorSelector interface {
	Select(context.Context, []domain.Capability, domain.ExecutionMode) (Executor, error)
}

type EvidenceRepository interface {
	Append(context.Context, domain.TaskID, []domain.Evidence) error
}

type PlanRepository interface {
	Create(context.Context, domain.Plan) error
	Get(context.Context, domain.PlanID) (domain.PlanRecord, error)
	BeginApply(context.Context, domain.PlanID, time.Time) error
	FinishApply(context.Context, domain.PlanApplyResult, time.Time) error
}

type PlanContext interface {
	Snapshot(context.Context, string) (domain.PlanSnapshot, error)
}

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Services struct {
	Tasks        TaskService
	Validation   ValidationService
	Workspace    WorkspaceService
	PullRequests PRService
	Repository   RepositoryService
	Execution    ExecutionService
	Plans        PlanService
}

type TaskService struct {
	Tasks   TaskRepository
	Issues  IssuePort
	Claimer TaskClaimer
	Policy  Policy
}

func (s TaskService) List(ctx context.Context) ([]domain.Task, error) {
	if s.Tasks == nil {
		return nil, errors.New("task repository is required")
	}
	return s.Tasks.List(ctx)
}

func (s TaskService) Get(ctx context.Context, id domain.TaskID) (domain.Task, error) {
	if s.Tasks == nil {
		return domain.Task{}, errors.New("task repository is required")
	}
	if strings.TrimSpace(string(id)) == "" {
		return domain.Task{}, errors.New("task id is required")
	}
	return s.Tasks.Get(ctx, id)
}

func (s TaskService) Create(ctx context.Context, request domain.CreateTaskRequest) (domain.Task, error) {
	if s.Issues == nil {
		return domain.Task{}, errors.New("issue port is required")
	}
	if strings.TrimSpace(request.Title) == "" {
		return domain.Task{}, errors.New("task title is required")
	}
	if request.Risk != domain.RiskLow && request.Risk != domain.RiskMedium && request.Risk != domain.RiskHigh {
		return domain.Task{}, fmt.Errorf("unsupported task risk %q", request.Risk)
	}
	if request.Mode != domain.ExecutionLocal && request.Mode != domain.ExecutionRemote {
		return domain.Task{}, fmt.Errorf("unsupported execution mode %q", request.Mode)
	}
	task, err := s.Issues.Create(ctx, request)
	if err != nil {
		return domain.Task{}, err
	}
	if err := task.Validate(); err != nil {
		return domain.Task{}, fmt.Errorf("issue port returned invalid task: %w", err)
	}
	return task, nil
}

func (s TaskService) Claim(ctx context.Context, id domain.TaskID, executor domain.ExecutorIdentity) (domain.Task, error) {
	if err := executor.Validate(); err != nil {
		return domain.Task{}, err
	}
	task, err := s.Get(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if s.Policy != nil {
		decision := s.Policy.Evaluate(ctx, task, domain.OperationTaskClaim)
		if !decision.Allowed {
			return domain.Task{}, fmt.Errorf("claim denied: %s", strings.Join(decision.Reasons, "; "))
		}
	}
	if !domain.CanTransition(task.State, domain.TaskClaimed) {
		return domain.Task{}, fmt.Errorf("cannot transition task from %s to %s", task.State, domain.TaskClaimed)
	}
	if s.Claimer != nil {
		claimed, claimErr := s.Claimer.Claim(ctx, task, executor)
		if claimErr != nil {
			return domain.Task{}, claimErr
		}
		if claimed.State != domain.TaskClaimed || claimed.Executor == nil {
			return domain.Task{}, errors.New("task claimer returned an unclaimed task")
		}
		if err := claimed.Validate(); err != nil {
			return domain.Task{}, fmt.Errorf("task claimer returned invalid task: %w", err)
		}
		return claimed, nil
	}
	if s.Tasks == nil {
		return domain.Task{}, errors.New("task repository is required")
	}
	task.State = domain.TaskClaimed
	task.Mode = executor.Mode
	task.Executor = &executor
	if err := task.Validate(); err != nil {
		return domain.Task{}, err
	}
	if err := s.Tasks.Save(ctx, task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

type ValidationService struct {
	Tasks   TaskRepository
	Planner ValidationPlanner
}

func (s ValidationService) Plan(ctx context.Context, id domain.TaskID, profile string) (domain.ValidationPlan, error) {
	if s.Tasks == nil || s.Planner == nil {
		return domain.ValidationPlan{}, errors.New("task repository and validation planner are required")
	}
	task, err := s.Tasks.Get(ctx, id)
	if err != nil {
		return domain.ValidationPlan{}, err
	}
	if strings.TrimSpace(profile) == "" {
		return domain.ValidationPlan{}, errors.New("validation profile is required")
	}
	plan, err := s.Planner.Plan(ctx, task, profile)
	if err != nil {
		return domain.ValidationPlan{}, err
	}
	if plan.TaskID != task.ID {
		return domain.ValidationPlan{}, errors.New("validation plan task id mismatch")
	}
	if len(plan.Requires) == 0 {
		return domain.ValidationPlan{}, errors.New("validation plan capabilities are required")
	}
	return plan, nil
}

type WorkspaceService struct {
	Workspace WorkspacePort
}

func (s WorkspaceService) Change(ctx context.Context, request domain.WorkspaceRequest) (domain.WorkspaceResult, error) {
	if s.Workspace == nil {
		return domain.WorkspaceResult{}, errors.New("workspace port is required")
	}
	if strings.TrimSpace(request.Repository) == "" {
		return domain.WorkspaceResult{}, errors.New("repository is required")
	}
	if request.Action != "create" && request.Action != "remove" {
		return domain.WorkspaceResult{}, fmt.Errorf("unsupported workspace action %q", request.Action)
	}
	return s.Workspace.Change(ctx, request)
}

type PRService struct {
	PullRequests PullRequestPort
}

func (s PRService) Request(ctx context.Context, request domain.PullRequestRequest) (domain.PullRequestResult, error) {
	if s.PullRequests == nil {
		return domain.PullRequestResult{}, errors.New("pull request port is required")
	}
	if strings.TrimSpace(request.Repository) == "" || strings.TrimSpace(request.Base) == "" || strings.TrimSpace(request.Head) == "" || strings.TrimSpace(request.Title) == "" || strings.TrimSpace(request.Executor) == "" {
		return domain.PullRequestResult{}, errors.New("repository, base, head, title, and executor are required")
	}
	return s.PullRequests.Request(ctx, request)
}

type RepositoryService struct {
	Repository RepositoryPort
}

func (s RepositoryService) Inspect(ctx context.Context, request domain.RepositoryRequest) (domain.RepositoryReport, error) {
	if s.Repository == nil {
		return domain.RepositoryReport{}, errors.New("repository port is required")
	}
	if strings.TrimSpace(request.Repository) == "" {
		return domain.RepositoryReport{}, errors.New("repository is required")
	}
	if request.Mode != "doctor" && request.Mode != "inspect" {
		return domain.RepositoryReport{}, fmt.Errorf("unsupported repository inspection mode %q", request.Mode)
	}
	return s.Repository.Inspect(ctx, request)
}

type ExecutionService struct {
	Selector ExecutorSelector
	Evidence EvidenceRepository
	Clock    Clock
}

func (s ExecutionService) Run(ctx context.Context, task domain.Task, effects []domain.Effect, preferred domain.ExecutionMode) ([]domain.Evidence, error) {
	return s.run(ctx, task, effects, preferred, domain.Plan{})
}

func (s ExecutionService) RunPlan(ctx context.Context, plan domain.Plan) ([]domain.Evidence, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	return s.run(ctx, plan.Task, plan.Effects, plan.PreferredMode, plan)
}

func (s ExecutionService) run(ctx context.Context, task domain.Task, effects []domain.Effect, preferred domain.ExecutionMode, plan domain.Plan) ([]domain.Evidence, error) {
	if s.Selector == nil {
		return nil, errors.New("executor selector is required")
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	required := make([]domain.Capability, 0)
	seen := map[domain.Capability]bool{}
	for _, effect := range effects {
		if err := effect.Validate(); err != nil {
			return nil, err
		}
		for _, capability := range effect.Requires {
			if !seen[capability] {
				seen[capability] = true
				required = append(required, capability)
			}
		}
	}
	if len(required) == 0 {
		return nil, errors.New("at least one execution capability is required")
	}
	executor, err := s.Selector.Select(ctx, required, preferred)
	if err != nil {
		return nil, err
	}
	evidence, err := executor.Execute(ctx, task, effects)
	if err != nil {
		return evidence, err
	}
	clock := s.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	for index := range evidence {
		if evidence[index].Observed.IsZero() {
			evidence[index].Observed = clock.Now()
		}
		if plan.ID != "" {
			evidence[index].PlanID = plan.ID
			evidence[index].BaseHead = plan.BaseHead
			evidence[index].ConfigDigest = plan.ConfigDigest
		}
	}
	if s.Evidence != nil {
		if err := s.Evidence.Append(ctx, task.ID, evidence); err != nil {
			return evidence, err
		}
	}
	return evidence, nil
}

type PlanService struct {
	Plans     PlanRepository
	Context   PlanContext
	Policy    Policy
	Execution ExecutionService
	Clock     Clock
}

func (s PlanService) Create(ctx context.Context, request domain.CreatePlanRequest) (domain.Plan, error) {
	if s.Plans == nil || s.Context == nil || s.Policy == nil {
		return domain.Plan{}, errors.New("plan repository, context, and policy are required")
	}
	if strings.TrimSpace(request.Repository) == "" {
		return domain.Plan{}, errors.New("plan repository is required")
	}
	if err := request.Task.Validate(); err != nil {
		return domain.Plan{}, fmt.Errorf("invalid plan task: %w", err)
	}
	if len(request.Effects) == 0 {
		return domain.Plan{}, errors.New("plan effects are required")
	}
	for _, effect := range request.Effects {
		if err := effect.Validate(); err != nil {
			return domain.Plan{}, fmt.Errorf("invalid plan effect: %w", err)
		}
	}
	snapshot, err := s.Context.Snapshot(ctx, request.Repository)
	if err != nil {
		return domain.Plan{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return domain.Plan{}, err
	}
	decision := s.Policy.Evaluate(ctx, request.Task, domain.OperationPlanCreate)
	if !decision.Allowed {
		return domain.Plan{}, fmt.Errorf("plan creation denied: %s", strings.Join(decision.Reasons, "; "))
	}
	applyDecision := s.Policy.Evaluate(ctx, request.Task, domain.OperationPlanApply)
	preferred := request.PreferredMode
	if preferred == "" {
		preferred = request.Task.Mode
	}
	plan, err := (domain.Plan{
		Repository: snapshot.Repository, BaseHead: snapshot.Head, ConfigDigest: snapshot.ConfigDigest,
		Task: request.Task, Effects: append([]domain.Effect(nil), request.Effects...), PreferredMode: preferred,
		PolicyDecisions: []domain.PlanPolicyDecision{
			{Operation: domain.OperationPlanCreate, Decision: decision},
			{Operation: domain.OperationPlanApply, Decision: applyDecision},
		},
		CreatedAt: s.now(),
	}).Seal()
	if err != nil {
		return domain.Plan{}, err
	}
	if err := s.Plans.Create(ctx, plan); err != nil {
		return domain.Plan{}, err
	}
	return plan, nil
}

func (s PlanService) Show(ctx context.Context, id domain.PlanID) (domain.PlanRecord, error) {
	if s.Plans == nil {
		return domain.PlanRecord{}, errors.New("plan repository is required")
	}
	if strings.TrimSpace(string(id)) == "" {
		return domain.PlanRecord{}, errors.New("plan id is required")
	}
	record, err := s.Plans.Get(ctx, id)
	if err != nil {
		return domain.PlanRecord{}, err
	}
	if record.Plan.ID != id || record.Status.PlanID != id {
		return domain.PlanRecord{}, errors.New("plan repository returned mismatched plan id")
	}
	if err := record.Plan.Validate(); err != nil {
		return domain.PlanRecord{}, err
	}
	if err := record.Status.Validate(); err != nil {
		return domain.PlanRecord{}, err
	}
	return record, nil
}

func (s PlanService) Diff(ctx context.Context, id domain.PlanID) (domain.PlanDiff, error) {
	if s.Context == nil {
		return domain.PlanDiff{}, errors.New("plan context is required")
	}
	record, err := s.Show(ctx, id)
	if err != nil {
		return domain.PlanDiff{}, err
	}
	actual, err := s.Context.Snapshot(ctx, record.Plan.Repository)
	if err != nil {
		return domain.PlanDiff{}, err
	}
	if err := actual.Validate(); err != nil {
		return domain.PlanDiff{}, err
	}
	return comparePlan(record, actual), nil
}

func (s PlanService) Apply(ctx context.Context, id domain.PlanID) (domain.PlanApplyResult, error) {
	if s.Plans == nil || s.Context == nil || s.Policy == nil {
		return domain.PlanApplyResult{}, errors.New("plan repository, context, and policy are required")
	}
	diff, err := s.Diff(ctx, id)
	if err != nil {
		return domain.PlanApplyResult{}, err
	}
	if !diff.Ready {
		return domain.PlanApplyResult{}, fmt.Errorf("plan is not ready: %s", strings.Join(diff.Reasons, "; "))
	}
	record, err := s.Show(ctx, id)
	if err != nil {
		return domain.PlanApplyResult{}, err
	}
	decision := s.Policy.Evaluate(ctx, record.Plan.Task, domain.OperationPlanApply)
	planDecision := domain.PlanPolicyDecision{Operation: domain.OperationPlanApply, Decision: decision}
	if !decision.Allowed {
		return domain.PlanApplyResult{PlanID: id, State: domain.PlanFailed, Decision: planDecision}, fmt.Errorf("plan apply denied: %s", strings.Join(decision.Reasons, "; "))
	}
	started := s.now()
	if err := s.Plans.BeginApply(ctx, id, started); err != nil {
		return domain.PlanApplyResult{}, err
	}
	actual, snapshotErr := s.Context.Snapshot(ctx, record.Plan.Repository)
	if snapshotErr != nil {
		result := domain.PlanApplyResult{PlanID: id, State: domain.PlanFailed, Decision: planDecision, Error: snapshotErr.Error()}
		return result, s.finishFailure(ctx, result, snapshotErr)
	}
	lockedDiff := comparePlan(domain.PlanRecord{Plan: record.Plan, Status: domain.PlanStatus{PlanID: id, State: domain.PlanPlanned}}, actual)
	if !lockedDiff.Ready {
		driftErr := fmt.Errorf("plan changed while acquiring apply lease: %s", strings.Join(lockedDiff.Reasons, "; "))
		result := domain.PlanApplyResult{PlanID: id, State: domain.PlanFailed, Decision: planDecision, Error: driftErr.Error()}
		return result, s.finishFailure(ctx, result, driftErr)
	}
	evidence, executeErr := s.Execution.RunPlan(ctx, record.Plan)
	state := domain.PlanApplied
	message := ""
	if executeErr != nil {
		state = domain.PlanFailed
		message = executeErr.Error()
	}
	result := domain.PlanApplyResult{PlanID: id, State: state, Decision: planDecision, Evidence: evidence, Error: message}
	if finishErr := s.Plans.FinishApply(ctx, result, s.now()); finishErr != nil {
		if executeErr != nil {
			return result, fmt.Errorf("%v; record plan result: %w", executeErr, finishErr)
		}
		return result, fmt.Errorf("record plan result: %w", finishErr)
	}
	return result, executeErr
}

func (s PlanService) finishFailure(ctx context.Context, result domain.PlanApplyResult, cause error) error {
	if finishErr := s.Plans.FinishApply(ctx, result, s.now()); finishErr != nil {
		return fmt.Errorf("%v; record plan failure: %w", cause, finishErr)
	}
	return cause
}

func (s PlanService) now() time.Time {
	clock := s.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	return clock.Now()
}

func comparePlan(record domain.PlanRecord, actual domain.PlanSnapshot) domain.PlanDiff {
	plan := record.Plan
	diff := domain.PlanDiff{
		PlanID:   idOrStatus(plan.ID, record.Status.PlanID),
		Expected: domain.PlanSnapshot{Repository: plan.Repository, Head: plan.BaseHead, ConfigDigest: plan.ConfigDigest},
		Actual:   actual, Status: record.Status.State,
	}
	if actual.Repository != plan.Repository {
		diff.Reasons = append(diff.Reasons, "repository changed")
	}
	diff.HeadMatches = actual.Head == plan.BaseHead
	if !diff.HeadMatches {
		diff.Reasons = append(diff.Reasons, "HEAD changed")
	}
	diff.ConfigMatches = actual.ConfigDigest == plan.ConfigDigest
	if !diff.ConfigMatches {
		diff.Reasons = append(diff.Reasons, "config changed")
	}
	if record.Status.State != domain.PlanPlanned {
		diff.Reasons = append(diff.Reasons, fmt.Sprintf("plan status is %s", record.Status.State))
	}
	diff.Ready = len(diff.Reasons) == 0
	return diff
}

func idOrStatus(planID, statusID domain.PlanID) domain.PlanID {
	if planID != "" {
		return planID
	}
	return statusID
}

type CapabilitySelector struct {
	Executors []Executor
}

func (s CapabilitySelector) Select(_ context.Context, required []domain.Capability, preferred domain.ExecutionMode) (Executor, error) {
	var fallback Executor
	for _, executor := range s.Executors {
		if executor == nil {
			continue
		}
		identity := executor.Identity()
		if !identity.Supports(required...) {
			continue
		}
		if identity.Mode == preferred {
			return executor, nil
		}
		if fallback == nil {
			fallback = executor
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("no executor provides capabilities %v", required)
}
