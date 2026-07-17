package sdk_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/JiaxI2/git-isolated-agent-kit/pkg/sdk"
)

type store struct{ task sdk.Task }

func (s *store) List(context.Context) ([]sdk.Task, error) { return []sdk.Task{s.task}, nil }
func (s *store) Get(context.Context, string) (sdk.Task, error) {
	return s.task, nil
}

type plans struct{ record sdk.PlanRecord }

func (p *plans) Create(_ context.Context, plan sdk.Plan) error {
	p.record = sdk.PlanRecord{Plan: plan, Status: sdk.PlanStatus{PlanID: plan.ID, State: sdk.PlanPlanned}}
	return nil
}
func (p *plans) Get(_ context.Context, id sdk.PlanID) (sdk.PlanRecord, error) {
	if p.record.Plan.ID != id {
		return sdk.PlanRecord{}, errors.New("plan not found")
	}
	return p.record, nil
}
func (p *plans) BeginApply(_ context.Context, id sdk.PlanID, started time.Time) error {
	if p.record.Plan.ID != id || p.record.Status.State != sdk.PlanPlanned {
		return errors.New("plan already attempted")
	}
	p.record.Status.State = sdk.PlanApplying
	p.record.Status.StartedAt = started
	return nil
}
func (p *plans) FinishApply(_ context.Context, result sdk.PlanApplyResult, finished time.Time) error {
	p.record.Status.State = result.State
	p.record.Status.FinishedAt = finished
	p.record.Status.Error = result.Error
	return nil
}

type snapshot struct{ value sdk.PlanSnapshot }

func (s snapshot) Snapshot(context.Context, string) (sdk.PlanSnapshot, error) { return s.value, nil }

type allowPolicy struct{}

func (allowPolicy) Evaluate(context.Context, sdk.Task, string) sdk.PolicyDecision {
	return sdk.PolicyDecision{Allowed: true}
}

type localExecutor struct{}

func (localExecutor) Identity() sdk.ExecutorIdentity {
	return sdk.ExecutorIdentity{Name: "local", Mode: sdk.ModeLocal, Capabilities: []sdk.Capability{"local.command"}}
}
func (localExecutor) Execute(context.Context, sdk.Task, []sdk.Effect) ([]sdk.Evidence, error) {
	return []sdk.Evidence{{Kind: "command", Source: "local", Summary: "ok"}}, nil
}

type clock struct{}

func (clock) Now() time.Time { return time.Unix(1, 0).UTC() }
func (s *store) Save(_ context.Context, task sdk.Task) error {
	s.task = task
	return nil
}

func TestExternalConsumerUsesOnlySDKTypes(t *testing.T) {
	tasks := &store{task: sdk.Task{ID: "3", Title: "Architecture", State: "ready", Risk: sdk.RiskMedium, Mode: sdk.ModeRemote}}
	client := sdk.New(sdk.Ports{Tasks: tasks})
	items, err := client.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "3" {
		t.Fatalf("items=%+v", items)
	}
	claimed, err := client.ClaimTask(context.Background(), "3", sdk.ExecutorIdentity{
		Name: "agent", Mode: sdk.ModeRemote, Capabilities: []sdk.Capability{"remote.issue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.State != "claimed" || tasks.task.Executor == nil {
		t.Fatalf("claimed=%+v stored=%+v", claimed, tasks.task)
	}
}

func TestExternalConsumerCreatesInspectsAndAppliesPlan(t *testing.T) {
	store := &plans{}
	current := sdk.PlanSnapshot{
		Repository: "C:/repo", Head: strings.Repeat("a", 40), ConfigDigest: "sha256:" + strings.Repeat("b", 64),
	}
	client := sdk.New(sdk.Ports{
		Plans: store, PlanContext: snapshot{value: current}, Policy: allowPolicy{},
		Executors: []sdk.Executor{localExecutor{}}, Clock: clock{},
	})
	plan, err := client.CreatePlan(context.Background(), sdk.CreatePlanRequest{
		Repository: current.Repository,
		Task:       sdk.Task{ID: "5", Title: "Inspectable Plan", State: "ready", Risk: sdk.RiskMedium, Mode: sdk.ModeLocal},
		Effects:    []sdk.Effect{{Kind: "command", Command: "go", Args: []string{"test", "./..."}, Requires: []sdk.Capability{"local.command"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	diff, err := client.DiffPlan(context.Background(), plan.ID)
	if err != nil || !diff.Ready {
		t.Fatalf("diff=%+v err=%v", diff, err)
	}
	result, err := client.ApplyPlan(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != sdk.PlanApplied || len(result.Evidence) != 1 || result.Evidence[0].PlanID != plan.ID {
		t.Fatalf("result=%+v", result)
	}
	if _, err := client.ApplyPlan(context.Background(), plan.ID); err == nil {
		t.Fatal("second apply was accepted")
	}
}
