package app

import (
	"context"
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
}

func (e *fakeExecutor) Identity() domain.ExecutorIdentity { return e.identity }
func (e *fakeExecutor) Execute(context.Context, domain.Task, []domain.Effect) ([]domain.Evidence, error) {
	e.called = true
	return []domain.Evidence{{Kind: "test", Source: e.identity.Name, Summary: "ok"}}, nil
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
	return domain.Task{ID: "3", Title: "Architecture V2", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}
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
