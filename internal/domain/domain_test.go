package domain

import (
	"strings"
	"testing"
	"time"
)

func TestTaskValidateRequiresExecutorForOwnedStates(t *testing.T) {
	task := Task{ID: "3", Title: "Architecture", State: TaskClaimed, Risk: RiskMedium, Mode: ExecutionRemote}
	if err := task.Validate(); err == nil {
		t.Fatal("claimed task without executor was accepted")
	}
	task.Executor = &ExecutorIdentity{Name: "github-agent", Mode: ExecutionRemote, Capabilities: []Capability{CapabilityIssue}}
	if err := task.Validate(); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
}

func TestTaskValidateRejectsUnknownRisk(t *testing.T) {
	task := Task{ID: "3", Title: "Architecture", State: TaskReady, Risk: "critical", Mode: ExecutionRemote}
	if err := task.Validate(); err == nil {
		t.Fatal("unknown risk was accepted")
	}
}

func TestExecutorSupportsAllRequiredCapabilities(t *testing.T) {
	executor := ExecutorIdentity{Name: "local", Mode: ExecutionLocal, Capabilities: []Capability{CapabilityCommand, CapabilityGit}}
	if !executor.Supports(CapabilityCommand, CapabilityGit) {
		t.Fatal("available capabilities were not recognized")
	}
	if executor.Supports(CapabilityCommand, CapabilityCI) {
		t.Fatal("missing capability was accepted")
	}
}

func TestCanTransitionUsesLegalWorkflowEdges(t *testing.T) {
	allowed := [][2]TaskState{
		{TaskReady, TaskClaimed},
		{TaskClaimed, TaskInProgress},
		{TaskInProgress, TaskValidating},
		{TaskValidating, TaskReview},
		{TaskReview, TaskDone},
		{TaskBlocked, TaskReady},
	}
	for _, transition := range allowed {
		if !CanTransition(transition[0], transition[1]) {
			t.Errorf("legal transition %s -> %s was rejected", transition[0], transition[1])
		}
	}
	denied := [][2]TaskState{{TaskReady, TaskDone}, {TaskDone, TaskReady}, {TaskInProgress, TaskDone}}
	for _, transition := range denied {
		if CanTransition(transition[0], transition[1]) {
			t.Errorf("illegal transition %s -> %s was accepted", transition[0], transition[1])
		}
	}
}

func TestEffectValidatePreservesArgumentArray(t *testing.T) {
	effect := Effect{Kind: EffectCommand, Command: "go", Args: []string{"test", "./path with spaces"}, Requires: []Capability{CapabilityTest}}
	if err := effect.Validate(); err != nil {
		t.Fatalf("valid effect rejected: %v", err)
	}
	if got := effect.Args[1]; got != "./path with spaces" {
		t.Fatalf("argument boundary changed: %q", got)
	}
}

func TestPlanSealBindsStableContentAndRejectsTampering(t *testing.T) {
	plan := Plan{
		Repository: "C:/repo", BaseHead: strings.Repeat("a", 40), ConfigDigest: "sha256:" + strings.Repeat("b", 64),
		Task:            Task{ID: "5", Title: "Inspectable Plan", State: TaskReady, Risk: RiskMedium, Mode: ExecutionLocal},
		Effects:         []Effect{{Kind: EffectCommand, Command: "go", Args: []string{"test", "./path with spaces"}, Requires: []Capability{CapabilityCommand, CapabilityTest}}},
		PolicyDecisions: []PlanPolicyDecision{{Operation: OperationPlanCreate, Decision: PolicyDecision{Allowed: true}}},
		PreferredMode:   ExecutionLocal, CreatedAt: time.Unix(1, 0).UTC(),
	}
	sealed, err := plan.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ID == "" || len(sealed.Requires) != 2 {
		t.Fatalf("sealed plan=%+v", sealed)
	}
	again, err := plan.Seal()
	if err != nil || again.ID != sealed.ID {
		t.Fatalf("plan id is not stable: first=%s second=%s err=%v", sealed.ID, again.ID, err)
	}
	sealed.Task.Title = "tampered"
	if err := sealed.Validate(); err == nil || !strings.Contains(err.Error(), "plan id mismatch") {
		t.Fatalf("tampered plan error=%v", err)
	}
}

func TestPlanRejectsInvalidSnapshotAndCapabilitySummary(t *testing.T) {
	plan := Plan{
		ID: "invalid", Repository: ".", BaseHead: "deadbeef", ConfigDigest: "sha256:bad",
		Task:            Task{ID: "5", Title: "Inspectable Plan", State: TaskReady, Risk: RiskMedium, Mode: ExecutionLocal},
		Effects:         []Effect{{Kind: EffectCommand, Command: "go", Requires: []Capability{CapabilityTest}}},
		PolicyDecisions: []PlanPolicyDecision{{Operation: OperationPlanCreate, Decision: PolicyDecision{Allowed: true}}},
		PreferredMode:   ExecutionLocal, CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "Git object id") {
		t.Fatalf("invalid snapshot error=%v", err)
	}
	plan.BaseHead = strings.Repeat("a", 40)
	plan.ConfigDigest = "sha256:" + strings.Repeat("b", 64)
	if err := plan.Validate(); err == nil || !strings.Contains(err.Error(), "capabilities do not match") {
		t.Fatalf("invalid capability summary error=%v", err)
	}
}
