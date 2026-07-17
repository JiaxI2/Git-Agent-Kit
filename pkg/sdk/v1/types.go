// Package v1 exposes the stable public Go API for Git Agent Kit.
package v1

import (
	"context"
	"time"
)

type Capability string
type TaskState string
type ExecutionMode string
type Risk string

const (
	ModeLocal  ExecutionMode = "local"
	ModeRemote ExecutionMode = "remote"
	RiskLow    Risk          = "low"
	RiskMedium Risk          = "medium"
	RiskHigh   Risk          = "high"
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
	Kind      string         `json:"kind"`
	Source    string         `json:"source"`
	Summary   string         `json:"summary"`
	Reference string         `json:"reference,omitempty"`
	Observed  time.Time      `json:"observed"`
	Metadata  map[string]any `json:"metadata,omitempty"`
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
	Clock        Clock
}
