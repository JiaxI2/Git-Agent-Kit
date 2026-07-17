// Package sdk exposes the stable public Go API for Git Agent Kit.
package sdk

import (
	"context"
	"time"
)

type Capability string
type TaskState string
type ExecutionMode string
type Risk string
type PlanID string
type PlanState string

const (
	ModeLocal    ExecutionMode = "local"
	ModeRemote   ExecutionMode = "remote"
	RiskLow      Risk          = "low"
	RiskMedium   Risk          = "medium"
	RiskHigh     Risk          = "high"
	PlanPlanned  PlanState     = "planned"
	PlanApplying PlanState     = "applying"
	PlanApplied  PlanState     = "applied"
	PlanFailed   PlanState     = "failed"
)

type ExecutorIdentity struct {
	Name         string         `json:"name"`
	Mode         ExecutionMode  `json:"mode"`
	Capabilities []Capability   `json:"capabilities"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Task struct {
	ID          string            `json:"id"`
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

type CreateTaskRequest struct {
	Title             string        `json:"title"`
	Description       string        `json:"description,omitempty"`
	Risk              Risk          `json:"risk"`
	PreferredExecutor string        `json:"preferredExecutor,omitempty"`
	Mode              ExecutionMode `json:"mode"`
}

type Effect struct {
	Kind       string            `json:"kind"`
	Target     string            `json:"target,omitempty"`
	Workdir    string            `json:"workdir,omitempty"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Requires   []Capability      `json:"requires"`
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

type PlanPolicyDecision struct {
	Operation string         `json:"operation"`
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

type PlanStatus struct {
	PlanID     PlanID    `json:"planId"`
	State      PlanState `json:"state"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	Error      string    `json:"error,omitempty"`
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
	TaskID   string       `json:"taskId"`
	Profile  string       `json:"profile"`
	Effects  []Effect     `json:"effects,omitempty"`
	Checks   []string     `json:"checks,omitempty"`
	Requires []Capability `json:"requires"`
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

type PolicyDecision struct {
	Allowed bool     `json:"allowed"`
	Reasons []string `json:"reasons,omitempty"`
}

type TaskStore interface {
	List(context.Context) ([]Task, error)
	Get(context.Context, string) (Task, error)
	Save(context.Context, Task) error
}

type IssueGateway interface {
	Create(context.Context, CreateTaskRequest) (Task, error)
}

type TaskClaimer interface {
	Claim(context.Context, Task, ExecutorIdentity) (Task, error)
}

type ValidationPlanner interface {
	Plan(context.Context, Task, string) (ValidationPlan, error)
}

type WorkspaceGateway interface {
	Change(context.Context, WorkspaceRequest) (WorkspaceResult, error)
}

type PullRequestGateway interface {
	Request(context.Context, PullRequestRequest) (PullRequestResult, error)
}

type RepositoryGateway interface {
	Inspect(context.Context, RepositoryRequest) (RepositoryReport, error)
}

type Policy interface {
	Evaluate(context.Context, Task, string) PolicyDecision
}

type Executor interface {
	Identity() ExecutorIdentity
	Execute(context.Context, Task, []Effect) ([]Evidence, error)
}

type EvidenceStore interface {
	Append(context.Context, string, []Evidence) error
}

type PlanStore interface {
	Create(context.Context, Plan) error
	Get(context.Context, PlanID) (PlanRecord, error)
	BeginApply(context.Context, PlanID, time.Time) error
	FinishApply(context.Context, PlanApplyResult, time.Time) error
}

type PlanContext interface {
	Snapshot(context.Context, string) (PlanSnapshot, error)
}

type Clock interface {
	Now() time.Time
}

type Ports struct {
	Tasks        TaskStore
	Issues       IssueGateway
	Claimer      TaskClaimer
	Validation   ValidationPlanner
	Workspace    WorkspaceGateway
	PullRequests PullRequestGateway
	Repository   RepositoryGateway
	Policy       Policy
	Executors    []Executor
	Evidence     EvidenceStore
	Plans        PlanStore
	PlanContext  PlanContext
	Clock        Clock
}
