package domain

import "testing"

func TestTaskValidateRequiresExecutorForOwnedStates(t *testing.T) {
	task := Task{ID: "3", Title: "Architecture V2", State: TaskClaimed, Risk: RiskMedium, Mode: ExecutionRemote}
	if err := task.Validate(); err == nil {
		t.Fatal("claimed task without executor was accepted")
	}
	task.Executor = &ExecutorIdentity{Name: "github-agent", Mode: ExecutionRemote, Capabilities: []Capability{CapabilityIssue}}
	if err := task.Validate(); err != nil {
		t.Fatalf("valid task rejected: %v", err)
	}
}

func TestTaskValidateRejectsUnknownRisk(t *testing.T) {
	task := Task{ID: "3", Title: "Architecture V2", State: TaskReady, Risk: "critical", Mode: ExecutionRemote}
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
