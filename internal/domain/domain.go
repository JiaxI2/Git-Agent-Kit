// Package domain defines the governance contracts shared by every adapter.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TaskID string
type TaskState string
type ExecutionMode string
type Risk string
type Capability string
type Operation string
type EffectKind string

const (
	TaskReady      TaskState = "ready"
	TaskClaimed    TaskState = "claimed"
	TaskInProgress TaskState = "in_progress"
	TaskValidating TaskState = "validating"
	TaskReview     TaskState = "review"
	TaskDone       TaskState = "done"
	TaskBlocked    TaskState = "blocked"
)

const (
	ExecutionLocal  ExecutionMode = "local"
	ExecutionRemote ExecutionMode = "remote"
)

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

const (
	CapabilityCommand     Capability = "local.command"
	CapabilityGit         Capability = "local.git"
	CapabilityWorktree    Capability = "local.worktree"
	CapabilityBuild       Capability = "local.build"
	CapabilityTest        Capability = "local.test"
	CapabilityHook        Capability = "local.hook"
	CapabilityFilesystem  Capability = "local.filesystem"
	CapabilityIssue       Capability = "remote.issue"
	CapabilityBranch      Capability = "remote.branch"
	CapabilityCommit      Capability = "remote.commit"
	CapabilityPullRequest Capability = "remote.pull_request"
	CapabilityReview      Capability = "remote.review"
	CapabilityCI          Capability = "remote.ci"
)

const (
	OperationTaskCreate   Operation  = "task.create"
	OperationTaskClaim    Operation  = "task.claim"
	OperationWorkspace    Operation  = "workspace.change"
	OperationValidation   Operation  = "validation.plan"
	OperationPullRequest  Operation  = "pull_request.request"
	OperationRepoInspect  Operation  = "repository.inspect"
	EffectCommand         EffectKind = "command"
	EffectFileWrite       EffectKind = "file.write"
	EffectRemoteOperation EffectKind = "remote.operation"
)

type ExecutorIdentity struct {
	Name         string         `json:"name"`
	Mode         ExecutionMode  `json:"mode"`
	Capabilities []Capability   `json:"capabilities"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

func (e ExecutorIdentity) Validate() error {
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("executor name is required")
	}
	if !validMode(e.Mode) {
		return fmt.Errorf("unsupported execution mode %q", e.Mode)
	}
	if len(e.Capabilities) == 0 {
		return errors.New("executor capabilities are required")
	}
	for _, capability := range e.Capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("unsupported executor capability %q", capability)
		}
	}
	return nil
}

func (e ExecutorIdentity) Supports(required ...Capability) bool {
	available := make(map[Capability]struct{}, len(e.Capabilities))
	for _, capability := range e.Capabilities {
		available[capability] = struct{}{}
	}
	for _, capability := range required {
		if _, ok := available[capability]; !ok {
			return false
		}
	}
	return true
}

type Task struct {
	ID          TaskID            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	State       TaskState         `json:"state"`
	Risk        Risk              `json:"risk"`
	Mode        ExecutionMode     `json:"mode"`
	Executor    *ExecutorIdentity `json:"executor,omitempty"`
	Branch      string            `json:"branch,omitempty"`
	Issue       int               `json:"issue,omitempty"`
	URL         string            `json:"url,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func (t Task) Validate() error {
	if strings.TrimSpace(string(t.ID)) == "" {
		return errors.New("task id is required")
	}
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("task title is required")
	}
	if !validState(t.State) {
		return fmt.Errorf("unsupported task state %q", t.State)
	}
	if !validRisk(t.Risk) {
		return fmt.Errorf("unsupported task risk %q", t.Risk)
	}
	if !validMode(t.Mode) {
		return fmt.Errorf("unsupported execution mode %q", t.Mode)
	}
	if t.Executor != nil {
		if err := t.Executor.Validate(); err != nil {
			return err
		}
	}
	if requiresExecutor(t.State) && t.Executor == nil {
		return fmt.Errorf("task state %s requires an executor", t.State)
	}
	return nil
}

type Effect struct {
	Kind       EffectKind        `json:"kind"`
	Target     string            `json:"target,omitempty"`
	Workdir    string            `json:"workdir,omitempty"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Requires   []Capability      `json:"requires"`
}

func (e Effect) Validate() error {
	if e.Kind != EffectCommand && e.Kind != EffectFileWrite && e.Kind != EffectRemoteOperation {
		return fmt.Errorf("unsupported effect kind %q", e.Kind)
	}
	if len(e.Requires) == 0 {
		return errors.New("effect capabilities are required")
	}
	for _, capability := range e.Requires {
		if !validCapability(capability) {
			return fmt.Errorf("unsupported effect capability %q", capability)
		}
	}
	if e.Kind == EffectCommand && strings.TrimSpace(e.Command) == "" {
		return errors.New("command effect requires a command")
	}
	return nil
}

type Evidence struct {
	Kind      string         `json:"kind"`
	Source    string         `json:"source"`
	Summary   string         `json:"summary"`
	Reference string         `json:"reference,omitempty"`
	Observed  time.Time      `json:"observed"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ValidationPlan struct {
	TaskID   TaskID       `json:"taskId"`
	Profile  string       `json:"profile"`
	Effects  []Effect     `json:"effects,omitempty"`
	Checks   []string     `json:"checks,omitempty"`
	Requires []Capability `json:"requires"`
}

type ValidationResult struct {
	OK       bool       `json:"ok"`
	Evidence []Evidence `json:"evidence,omitempty"`
	Errors   []string   `json:"errors,omitempty"`
}

type PolicyDecision struct {
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons,omitempty"`
}

type CreateTaskRequest struct {
	Title             string        `json:"title"`
	Description       string        `json:"description,omitempty"`
	Risk              Risk          `json:"risk"`
	PreferredExecutor string        `json:"preferredExecutor,omitempty"`
	Mode              ExecutionMode `json:"mode"`
}

type WorkspaceRequest struct {
	Repository string `json:"repository"`
	Action     string `json:"action"`
	PR         int    `json:"pr,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Root       string `json:"root,omitempty"`
	Path       string `json:"path,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

type WorkspaceResult struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Ref    string `json:"ref,omitempty"`
	Head   string `json:"head,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

type PullRequestRequest struct {
	Repository string `json:"repository"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	Executor   string `json:"executor"`
}

type PullRequestResult struct {
	Number   int    `json:"number"`
	URL      string `json:"url"`
	State    string `json:"state"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Title    string `json:"title"`
	Executor string `json:"executor"`
}

type RepositoryRequest struct {
	Repository string `json:"repository"`
	ConfigPath string `json:"configPath,omitempty"`
	Mode       string `json:"mode"`
}

type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type RepositoryReport struct {
	OK        bool    `json:"ok"`
	Repo      string  `json:"repo,omitempty"`
	Branch    string  `json:"branch,omitempty"`
	Head      string  `json:"head,omitempty"`
	Clean     bool    `json:"clean,omitempty"`
	Worktrees string  `json:"worktrees,omitempty"`
	Checks    []Check `json:"checks,omitempty"`
}

func CanTransition(from, to TaskState) bool {
	switch from {
	case TaskReady:
		return to == TaskClaimed || to == TaskBlocked
	case TaskClaimed:
		return to == TaskInProgress || to == TaskReady || to == TaskBlocked
	case TaskInProgress:
		return to == TaskValidating || to == TaskBlocked
	case TaskValidating:
		return to == TaskReview || to == TaskInProgress || to == TaskBlocked
	case TaskReview:
		return to == TaskDone || to == TaskInProgress || to == TaskBlocked
	case TaskBlocked:
		return to == TaskReady || to == TaskInProgress
	default:
		return false
	}
}

func requiresExecutor(state TaskState) bool {
	return state == TaskClaimed || state == TaskInProgress || state == TaskValidating || state == TaskReview
}

func validState(state TaskState) bool {
	return state == TaskReady || state == TaskClaimed || state == TaskInProgress || state == TaskValidating || state == TaskReview || state == TaskDone || state == TaskBlocked
}

func validMode(mode ExecutionMode) bool {
	return mode == ExecutionLocal || mode == ExecutionRemote
}

func validRisk(risk Risk) bool {
	return risk == RiskLow || risk == RiskMedium || risk == RiskHigh
}

func validCapability(capability Capability) bool {
	switch capability {
	case CapabilityCommand, CapabilityGit, CapabilityWorktree, CapabilityBuild,
		CapabilityTest, CapabilityHook, CapabilityFilesystem, CapabilityIssue,
		CapabilityBranch, CapabilityCommit, CapabilityPullRequest, CapabilityReview,
		CapabilityCI:
		return true
	default:
		return false
	}
}
