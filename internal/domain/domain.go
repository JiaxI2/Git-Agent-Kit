// Package domain defines the governance contracts shared by every adapter.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
type PlanID string
type PlanState string

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
	OperationPlanCreate   Operation  = "plan.create"
	OperationPlanApply    Operation  = "plan.apply"
	EffectCommand         EffectKind = "command"
	EffectFileWrite       EffectKind = "file.write"
	EffectRemoteOperation EffectKind = "remote.operation"
)

const (
	PlanPlanned  PlanState = "planned"
	PlanApplying PlanState = "applying"
	PlanApplied  PlanState = "applied"
	PlanFailed   PlanState = "failed"
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
	Kind         string         `json:"kind"`
	Source       string         `json:"source"`
	Summary      string         `json:"summary"`
	Reference    string         `json:"reference,omitempty"`
	Observed     time.Time      `json:"observed"`
	PlanID       PlanID         `json:"planId,omitempty"`
	BaseHead     string         `json:"baseHead,omitempty"`
	ConfigDigest string         `json:"configDigest,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type PlanSnapshot struct {
	Repository   string `json:"repository"`
	Head         string `json:"head"`
	ConfigDigest string `json:"configDigest"`
}

func (s PlanSnapshot) Validate() error {
	if strings.TrimSpace(s.Repository) == "" {
		return errors.New("plan repository is required")
	}
	if !validObjectID(s.Head) {
		return fmt.Errorf("plan head %q is not a Git object id", s.Head)
	}
	if !validDigest(s.ConfigDigest) {
		return fmt.Errorf("plan config digest %q is invalid", s.ConfigDigest)
	}
	return nil
}

type PlanPolicyDecision struct {
	Operation Operation      `json:"operation"`
	Decision  PolicyDecision `json:"decision"`
}

type CreatePlanRequest struct {
	Repository    string        `json:"repository"`
	Task          Task          `json:"task"`
	Effects       []Effect      `json:"effects"`
	PreferredMode ExecutionMode `json:"preferredMode"`
}

type Plan struct {
	ID              PlanID               `json:"id"`
	Repository      string               `json:"repository"`
	BaseHead        string               `json:"baseHead"`
	Task            Task                 `json:"task"`
	Effects         []Effect             `json:"effects"`
	PolicyDecisions []PlanPolicyDecision `json:"policyDecisions"`
	Requires        []Capability         `json:"requires"`
	ConfigDigest    string               `json:"configDigest"`
	PreferredMode   ExecutionMode        `json:"preferredMode"`
	CreatedAt       time.Time            `json:"createdAt"`
}

func (p Plan) Seal() (Plan, error) {
	p.ID = ""
	p.Requires = requiredCapabilities(p.Effects)
	if err := p.validateContent(); err != nil {
		return Plan{}, err
	}
	id, err := p.ComputeID()
	if err != nil {
		return Plan{}, err
	}
	p.ID = id
	return p, nil
}

func (p Plan) ComputeID() (PlanID, error) {
	p.ID = ""
	encoded, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encode plan: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return PlanID(hex.EncodeToString(digest[:])), nil
}

func (p Plan) Validate() error {
	if strings.TrimSpace(string(p.ID)) == "" {
		return errors.New("plan id is required")
	}
	if err := p.validateContent(); err != nil {
		return err
	}
	expected, err := p.ComputeID()
	if err != nil {
		return err
	}
	if p.ID != expected {
		return fmt.Errorf("plan id mismatch: expected %s", expected)
	}
	return nil
}

func (p Plan) validateContent() error {
	snapshot := PlanSnapshot{Repository: p.Repository, Head: p.BaseHead, ConfigDigest: p.ConfigDigest}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := p.Task.Validate(); err != nil {
		return fmt.Errorf("invalid plan task: %w", err)
	}
	if len(p.Effects) == 0 {
		return errors.New("plan effects are required")
	}
	for _, effect := range p.Effects {
		if err := effect.Validate(); err != nil {
			return fmt.Errorf("invalid plan effect: %w", err)
		}
	}
	if len(p.PolicyDecisions) == 0 {
		return errors.New("plan policy decisions are required")
	}
	for _, decision := range p.PolicyDecisions {
		if decision.Operation == "" {
			return errors.New("plan policy decision operation is required")
		}
	}
	expectedCapabilities := requiredCapabilities(p.Effects)
	if !slices.Equal(p.Requires, expectedCapabilities) {
		return fmt.Errorf("plan capabilities do not match effects: expected %v", expectedCapabilities)
	}
	if !validMode(p.PreferredMode) {
		return fmt.Errorf("unsupported preferred execution mode %q", p.PreferredMode)
	}
	if p.CreatedAt.IsZero() {
		return errors.New("plan creation time is required")
	}
	return nil
}

type PlanStatus struct {
	PlanID     PlanID    `json:"planId"`
	State      PlanState `json:"state"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func (s PlanStatus) Validate() error {
	if strings.TrimSpace(string(s.PlanID)) == "" {
		return errors.New("plan status id is required")
	}
	switch s.State {
	case PlanPlanned:
		if !s.StartedAt.IsZero() || !s.FinishedAt.IsZero() || s.Error != "" {
			return errors.New("planned status cannot contain execution data")
		}
	case PlanApplying:
		if s.StartedAt.IsZero() || !s.FinishedAt.IsZero() {
			return errors.New("applying status requires only a start time")
		}
	case PlanApplied:
		if s.StartedAt.IsZero() || s.FinishedAt.IsZero() || s.Error != "" {
			return errors.New("applied status requires start and finish times without an error")
		}
	case PlanFailed:
		if s.StartedAt.IsZero() || s.FinishedAt.IsZero() || strings.TrimSpace(s.Error) == "" {
			return errors.New("failed status requires start and finish times with an error")
		}
	default:
		return fmt.Errorf("unsupported plan state %q", s.State)
	}
	if !s.StartedAt.IsZero() && !s.FinishedAt.IsZero() && s.FinishedAt.Before(s.StartedAt) {
		return errors.New("plan finish time precedes start time")
	}
	return nil
}

type PlanRecord struct {
	Plan   Plan       `json:"plan"`
	Status PlanStatus `json:"status"`
}

type PlanDiff struct {
	PlanID        PlanID       `json:"planId"`
	Expected      PlanSnapshot `json:"expected"`
	Actual        PlanSnapshot `json:"actual"`
	Status        PlanState    `json:"status"`
	HeadMatches   bool         `json:"headMatches"`
	ConfigMatches bool         `json:"configMatches"`
	Ready         bool         `json:"ready"`
	Reasons       []string     `json:"reasons,omitempty"`
}

type PlanApplyResult struct {
	PlanID   PlanID             `json:"planId"`
	State    PlanState          `json:"state"`
	Decision PlanPolicyDecision `json:"decision"`
	Evidence []Evidence         `json:"evidence,omitempty"`
	Error    string             `json:"error,omitempty"`
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

func requiredCapabilities(effects []Effect) []Capability {
	seen := map[Capability]bool{}
	var required []Capability
	for _, effect := range effects {
		for _, capability := range effect.Requires {
			if !seen[capability] {
				seen[capability] = true
				required = append(required, capability)
			}
		}
	}
	return required
}

func validObjectID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validDigest(value string) bool {
	const prefix = "sha256:"
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	encoded := strings.TrimPrefix(value, prefix)
	if len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}
