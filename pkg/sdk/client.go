package sdk

import (
	"context"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type Client struct {
	services app.Services
}

func New(ports Ports) *Client {
	services := buildServices(ports)
	return &Client{services: services}
}

func (c *Client) ListTasks(ctx context.Context) ([]Task, error) {
	tasks, err := c.services.Tasks.List(ctx)
	if err != nil {
		return nil, err
	}
	return exportTasks(tasks), nil
}

func (c *Client) GetTask(ctx context.Context, id string) (Task, error) {
	task, err := c.services.Tasks.Get(ctx, domain.TaskID(id))
	return exportTask(task), err
}

func (c *Client) CreateTask(ctx context.Context, request CreateTaskRequest) (Task, error) {
	task, err := c.services.Tasks.Create(ctx, importCreateTaskRequest(request))
	return exportTask(task), err
}

func (c *Client) ClaimTask(ctx context.Context, id string, executor ExecutorIdentity) (Task, error) {
	task, err := c.services.Tasks.Claim(ctx, domain.TaskID(id), importExecutorIdentity(executor))
	return exportTask(task), err
}

func (c *Client) ValidationPlan(ctx context.Context, id, profile string) (ValidationPlan, error) {
	plan, err := c.services.Validation.Plan(ctx, domain.TaskID(id), profile)
	return exportValidationPlan(plan), err
}

func (c *Client) ChangeWorkspace(ctx context.Context, request WorkspaceRequest) (WorkspaceResult, error) {
	result, err := c.services.Workspace.Change(ctx, importWorkspaceRequest(request))
	return exportWorkspaceResult(result), err
}

func (c *Client) RequestPullRequest(ctx context.Context, request PullRequestRequest) (PullRequestResult, error) {
	result, err := c.services.PullRequests.Request(ctx, importPullRequestRequest(request))
	return exportPullRequestResult(result), err
}

func (c *Client) InspectRepository(ctx context.Context, request RepositoryRequest) (RepositoryReport, error) {
	report, err := c.services.Repository.Inspect(ctx, importRepositoryRequest(request))
	return exportRepositoryReport(report), err
}

func (c *Client) Execute(ctx context.Context, task Task, effects []Effect, preferred ExecutionMode) ([]Evidence, error) {
	internalEffects := make([]domain.Effect, 0, len(effects))
	for _, effect := range effects {
		internalEffects = append(internalEffects, importEffect(effect))
	}
	evidence, err := c.services.Execution.Run(ctx, importTask(task), internalEffects, domain.ExecutionMode(preferred))
	return exportEvidence(evidence), err
}

func (c *Client) CreatePlan(ctx context.Context, request CreatePlanRequest) (Plan, error) {
	plan, err := c.services.Plans.Create(ctx, importCreatePlanRequest(request))
	return exportPlan(plan), err
}

func (c *Client) ShowPlan(ctx context.Context, id PlanID) (PlanRecord, error) {
	record, err := c.services.Plans.Show(ctx, domain.PlanID(id))
	return exportPlanRecord(record), err
}

func (c *Client) DiffPlan(ctx context.Context, id PlanID) (PlanDiff, error) {
	diff, err := c.services.Plans.Diff(ctx, domain.PlanID(id))
	return exportPlanDiff(diff), err
}

func (c *Client) ApplyPlan(ctx context.Context, id PlanID) (PlanApplyResult, error) {
	result, err := c.services.Plans.Apply(ctx, domain.PlanID(id))
	return exportPlanApplyResult(result), err
}
