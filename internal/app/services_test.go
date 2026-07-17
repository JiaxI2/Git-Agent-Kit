package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type memoryTasks struct {
	task  domain.Task
	saved bool
}

func (m *memoryTasks) List(context.Context) ([]domain.Task, error) { return []domain.Task{m.task}, nil }
func (m *memoryTasks) Get(context.Context, domain.TaskID) (domain.Task, error) {
	return m.task, nil
}
func (m *memoryTasks) Save(_ context.Context, task domain.Task) error {
	m.task, m.saved = task, true
	return nil
}

type fixedPolicy struct{ allowed bool }

func (p fixedPolicy) Evaluate(context.Context, domain.Task, domain.Operation) domain.PolicyDecision {
	return domain.PolicyDecision{Allowed: p.allowed, Reasons: []string{"policy"}}
}

type fakeExecutor struct {
	identity domain.ExecutorIdentity
	called   bool
	err      error
}

func (e *fakeExecutor) Identity() domain.ExecutorIdentity { return e.identity }
func (e *fakeExecutor) Execute(context.Context, domain.Task, []domain.Effect) ([]domain.Evidence, error) {
	e.called = true
	return []domain.Evidence{{Kind: "test", Source: e.identity.Name, Summary: "ok"}}, e.err
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

type fakeWorkspace struct{ called bool }

func (w *fakeWorkspace) Change(_ context.Context, request domain.WorkspaceRequest) (domain.WorkspaceResult, error) {
	w.called = true
	return domain.WorkspaceResult{Path: request.Path}, nil
}

type fakePR struct{ called bool }

func (p *fakePR) Request(context.Context, domain.PullRequestRequest) (domain.PullRequestResult, error) {
	p.called = true
	return domain.PullRequestResult{Number: 4, URL: "https://example.invalid/4"}, nil
}

type fakeRepository struct{ called bool }

func (r *fakeRepository) Inspect(context.Context, domain.RepositoryRequest) (domain.RepositoryReport, error) {
	r.called = true
	return domain.RepositoryReport{OK: true, Repo: "repo"}, nil
}

func readyTask() domain.Task {
	return domain.Task{ID: "3", Title: "Architecture", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}
}

type memoryPlans struct {
	mu        sync.Mutex
	record    domain.PlanRecord
	result    domain.PlanApplyResult
	beginRuns int
}

func (m *memoryPlans) Create(_ context.Context, plan domain.Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record = domain.PlanRecord{Plan: plan, Status: domain.PlanStatus{PlanID: plan.ID, State: domain.PlanPlanned}}
	return nil
}

func (m *memoryPlans) Get(_ context.Context, id domain.PlanID) (domain.PlanRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.record.Plan.ID != id {
		return domain.PlanRecord{}, errors.New("plan not found")
	}
	return m.record, nil
}

func (m *memoryPlans) BeginApply(_ context.Context, id domain.PlanID, started time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.record.Plan.ID != id || m.record.Status.State != domain.PlanPlanned {
		return errors.New("plan has already been attempted")
	}
	m.beginRuns++
	m.record.Status.State = domain.PlanApplying
	m.record.Status.StartedAt = started
	return nil
}

func (m *memoryPlans) FinishApply(_ context.Context, result domain.PlanApplyResult, finished time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.record.Plan.ID != result.PlanID || m.record.Status.State != domain.PlanApplying {
		return errors.New("plan is not applying")
	}
	m.result = result
	m.record.Status.State = result.State
	m.record.Status.FinishedAt = finished
	m.record.Status.Error = result.Error
	return nil
}

type sequencePlanContext struct {
	mu        sync.Mutex
	snapshots []domain.PlanSnapshot
	calls     int
}

func (c *sequencePlanContext) Snapshot(context.Context, string) (domain.PlanSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.snapshots) == 0 {
		return domain.PlanSnapshot{}, errors.New("snapshot unavailable")
	}
	index := c.calls
	if index >= len(c.snapshots) {
		index = len(c.snapshots) - 1
	}
	c.calls++
	return c.snapshots[index], nil
}

func planSnapshot(head, digestByte string) domain.PlanSnapshot {
	return domain.PlanSnapshot{
		Repository:   "C:/repo",
		Head:         strings.Repeat(head, 40),
		ConfigDigest: "sha256:" + strings.Repeat(digestByte, 64),
	}
}

func planRequest() domain.CreatePlanRequest {
	return domain.CreatePlanRequest{
		Repository: "C:/repo",
		Task:       domain.Task{ID: "5", Title: "Inspectable Plan", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionLocal},
		Effects:    []domain.Effect{{Kind: domain.EffectCommand, Command: "go", Args: []string{"test", "./..."}, Requires: []domain.Capability{domain.CapabilityCommand}}},
	}
}

func planService(store *memoryPlans, snapshots *sequencePlanContext, executor *fakeExecutor) PlanService {
	clock := fixedClock{at: time.Unix(1, 0).UTC()}
	return PlanService{
		Plans: store, Context: snapshots, Policy: fixedPolicy{allowed: true}, Clock: clock,
		Execution: ExecutionService{
			Selector: CapabilitySelector{Executors: []Executor{executor}},
			Clock:    clock,
		},
	}
}

func TestTaskServiceClaimPersistsLegalTransition(t *testing.T) {
	store := &memoryTasks{task: readyTask()}
	executor := domain.ExecutorIdentity{Name: "agent", Mode: domain.ExecutionRemote, Capabilities: []domain.Capability{domain.CapabilityIssue}}
	claimed, err := (TaskService{Tasks: store, Policy: fixedPolicy{allowed: true}}).Claim(context.Background(), "3", executor)
	if err != nil {
		t.Fatal(err)
	}
	if !store.saved || claimed.State != domain.TaskClaimed || claimed.Executor == nil || claimed.Executor.Name != "agent" {
		t.Fatalf("claim was not persisted: %+v saved=%t", claimed, store.saved)
	}
}

func TestTaskServiceClaimFailsClosedOnPolicyDenial(t *testing.T) {
	store := &memoryTasks{task: readyTask()}
	executor := domain.ExecutorIdentity{Name: "agent", Mode: domain.ExecutionRemote, Capabilities: []domain.Capability{domain.CapabilityIssue}}
	if _, err := (TaskService{Tasks: store, Policy: fixedPolicy{}}).Claim(context.Background(), "3", executor); err == nil {
		t.Fatal("policy denial was ignored")
	}
	if store.saved {
		t.Fatal("denied task was persisted")
	}
}

func TestExecutionServiceSelectsByCapabilities(t *testing.T) {
	local := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}
	remote := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "remote", Mode: domain.ExecutionRemote, Capabilities: []domain.Capability{domain.CapabilityCI}}}
	service := ExecutionService{Selector: CapabilitySelector{Executors: []Executor{local, remote}}, Clock: fixedClock{at: time.Unix(1, 0).UTC()}}
	effect := domain.Effect{Kind: domain.EffectRemoteOperation, Requires: []domain.Capability{domain.CapabilityCI}}
	evidence, err := service.Run(context.Background(), readyTask(), []domain.Effect{effect}, domain.ExecutionLocal)
	if err != nil {
		t.Fatal(err)
	}
	if local.called || !remote.called {
		t.Fatalf("capability selection local=%t remote=%t", local.called, remote.called)
	}
	if len(evidence) != 1 || evidence[0].Observed.IsZero() {
		t.Fatalf("missing timestamped evidence: %+v", evidence)
	}
}

func TestCapabilitySelectorRejectsMissingCapabilities(t *testing.T) {
	selector := CapabilitySelector{Executors: []Executor{&fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}}}
	if _, err := selector.Select(context.Background(), []domain.Capability{domain.CapabilityCI}, domain.ExecutionRemote); err == nil {
		t.Fatal("missing capability was accepted")
	}
}

func TestWorkspacePRAndRepositoryServicesDelegate(t *testing.T) {
	workspace := &fakeWorkspace{}
	if _, err := (WorkspaceService{Workspace: workspace}).Change(context.Background(), domain.WorkspaceRequest{Repository: ".", Action: "remove", Path: "wt"}); err != nil || !workspace.called {
		t.Fatalf("workspace delegation failed: %v", err)
	}
	pullRequests := &fakePR{}
	if _, err := (PRService{PullRequests: pullRequests}).Request(context.Background(), domain.PullRequestRequest{Repository: ".", Base: "main", Head: "agent/x", Title: "x", Executor: "agent"}); err != nil || !pullRequests.called {
		t.Fatalf("PR delegation failed: %v", err)
	}
	repository := &fakeRepository{}
	if _, err := (RepositoryService{Repository: repository}).Inspect(context.Background(), domain.RepositoryRequest{Repository: ".", Mode: "inspect"}); err != nil || !repository.called {
		t.Fatalf("repository delegation failed: %v", err)
	}
}

func TestServicesFailClosedWhenPortsAreMissing(t *testing.T) {
	if _, err := (TaskService{}).List(context.Background()); err == nil {
		t.Fatal("missing task repository was accepted")
	}
	if _, err := (WorkspaceService{}).Change(context.Background(), domain.WorkspaceRequest{Repository: ".", Action: "create"}); err == nil {
		t.Fatal("missing workspace port was accepted")
	}
	if _, err := (PRService{}).Request(context.Background(), domain.PullRequestRequest{}); err == nil {
		t.Fatal("missing PR port was accepted")
	}
	if _, err := (RepositoryService{}).Inspect(context.Background(), domain.RepositoryRequest{}); err == nil {
		t.Fatal("missing repository port was accepted")
	}
}

func TestPlanServiceCreateShowAndDiffDoNotExecute(t *testing.T) {
	store := &memoryPlans{}
	snapshot := planSnapshot("a", "b")
	contextPort := &sequencePlanContext{snapshots: []domain.PlanSnapshot{snapshot}}
	executor := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}
	service := planService(store, contextPort, executor)

	plan, err := service.Create(context.Background(), planRequest())
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Show(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.Diff(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called || record.Status.State != domain.PlanPlanned || !diff.Ready {
		t.Fatalf("create/show/diff changed execution state: called=%t status=%s diff=%+v", executor.called, record.Status.State, diff)
	}
	if len(plan.PolicyDecisions) != 2 || plan.PolicyDecisions[1].Operation != domain.OperationPlanApply {
		t.Fatalf("plan does not expose create and apply policy decisions: %+v", plan.PolicyDecisions)
	}
}

func TestPlanServiceApplyBindsEvidenceAndRejectsSecondExecution(t *testing.T) {
	store := &memoryPlans{}
	snapshot := planSnapshot("a", "b")
	contextPort := &sequencePlanContext{snapshots: []domain.PlanSnapshot{snapshot}}
	executor := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}
	service := planService(store, contextPort, executor)

	plan, err := service.Create(context.Background(), planRequest())
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != domain.PlanApplied || len(result.Evidence) != 1 {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	evidence := result.Evidence[0]
	if evidence.PlanID != plan.ID || evidence.BaseHead != plan.BaseHead || evidence.ConfigDigest != plan.ConfigDigest {
		t.Fatalf("evidence is not bound to the plan: %+v", evidence)
	}
	if _, err := service.Apply(context.Background(), plan.ID); err == nil {
		t.Fatal("second plan apply was accepted")
	}
	if store.beginRuns != 1 || !executor.called {
		t.Fatalf("apply count=%d executor called=%t", store.beginRuns, executor.called)
	}
}

func TestPlanServiceRejectsDriftBeforeExecution(t *testing.T) {
	tests := []struct {
		name    string
		current domain.PlanSnapshot
		reason  string
	}{
		{name: "head", current: planSnapshot("c", "b"), reason: "HEAD changed"},
		{name: "config", current: planSnapshot("a", "d"), reason: "config changed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryPlans{}
			base := planSnapshot("a", "b")
			contextPort := &sequencePlanContext{snapshots: []domain.PlanSnapshot{base, test.current}}
			executor := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}
			service := planService(store, contextPort, executor)
			plan, err := service.Create(context.Background(), planRequest())
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.Apply(context.Background(), plan.ID)
			if err == nil || !strings.Contains(err.Error(), test.reason) {
				t.Fatalf("drift error=%v", err)
			}
			if store.beginRuns != 0 || executor.called {
				t.Fatalf("drift reached execution: begin=%d called=%t", store.beginRuns, executor.called)
			}
		})
	}
}

func TestPlanServiceRejectsPolicyDenialBeforePersistenceOrExecution(t *testing.T) {
	store := &memoryPlans{}
	snapshot := planSnapshot("a", "b")
	contextPort := &sequencePlanContext{snapshots: []domain.PlanSnapshot{snapshot}}
	executor := &fakeExecutor{identity: domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}}
	service := planService(store, contextPort, executor)
	service.Policy = fixedPolicy{allowed: false}

	if _, err := service.Create(context.Background(), planRequest()); err == nil {
		t.Fatal("policy denial was ignored")
	}
	if store.record.Plan.ID != "" || executor.called {
		t.Fatalf("denied plan changed state: record=%+v called=%t", store.record, executor.called)
	}
}
