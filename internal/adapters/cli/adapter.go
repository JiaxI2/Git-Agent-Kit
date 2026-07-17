// Package cli translates command input and application output.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type Adapter struct {
	Services app.Services
}

type Request struct {
	Command      string   `json:"command"`
	TaskID       string   `json:"taskId,omitempty"`
	Title        string   `json:"title,omitempty"`
	Description  string   `json:"description,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Executor     string   `json:"executor,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Profile      string   `json:"profile,omitempty"`
	Repository   string   `json:"repository,omitempty"`
	ConfigPath   string   `json:"configPath,omitempty"`
	Action       string   `json:"action,omitempty"`
	PR           int      `json:"pr,omitempty"`
	Ref          string   `json:"ref,omitempty"`
	Root         string   `json:"root,omitempty"`
	Path         string   `json:"path,omitempty"`
	Force        bool     `json:"force,omitempty"`
	Base         string   `json:"base,omitempty"`
	Head         string   `json:"head,omitempty"`
	Body         string   `json:"body,omitempty"`
}

type Response struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (r Response) ExitCode() int {
	if r.OK {
		return 0
	}
	return 1
}

func (a Adapter) Handle(ctx context.Context, request Request) Response {
	command := strings.TrimSpace(request.Command)
	var data any
	var err error
	switch command {
	case "task.list", "issue.list", "scan":
		data, err = a.Services.Tasks.List(ctx)
	case "task.get":
		data, err = a.Services.Tasks.Get(ctx, domain.TaskID(request.TaskID))
	case "task.create", "issue.create":
		data, err = a.Services.Tasks.Create(ctx, domain.CreateTaskRequest{
			Title: request.Title, Description: request.Description, Risk: domain.Risk(request.Risk),
			PreferredExecutor: request.Executor, Mode: domain.ExecutionMode(request.Mode),
		})
	case "task.claim", "claim":
		capabilities := make([]domain.Capability, 0, len(request.Capabilities))
		for _, capability := range request.Capabilities {
			capabilities = append(capabilities, domain.Capability(capability))
		}
		data, err = a.Services.Tasks.Claim(ctx, domain.TaskID(request.TaskID), domain.ExecutorIdentity{
			Name: request.Executor, Mode: domain.ExecutionMode(request.Mode), Capabilities: capabilities,
		})
	case "validation.plan", "validate.plan":
		data, err = a.Services.Validation.Plan(ctx, domain.TaskID(request.TaskID), request.Profile)
	case "workspace.create", "worktree.create", "workspace.remove", "worktree.remove":
		action := request.Action
		if action == "" {
			if strings.HasSuffix(command, ".create") {
				action = "create"
			} else {
				action = "remove"
			}
		}
		data, err = a.Services.Workspace.Change(ctx, domain.WorkspaceRequest{
			Repository: request.Repository, Action: action, PR: request.PR, Ref: request.Ref,
			Root: request.Root, Path: request.Path, Force: request.Force,
		})
	case "pull_request.request", "pr.request":
		data, err = a.Services.PullRequests.Request(ctx, domain.PullRequestRequest{
			Repository: request.Repository, Base: request.Base, Head: request.Head,
			Title: request.Title, Body: request.Body, Executor: request.Executor,
		})
	case "repository.doctor", "doctor", "repository.inspect", "repo.inspect", "status":
		mode := "inspect"
		if command == "repository.doctor" || command == "doctor" {
			mode = "doctor"
		}
		data, err = a.Services.Repository.Inspect(ctx, domain.RepositoryRequest{
			Repository: request.Repository, ConfigPath: request.ConfigPath, Mode: mode,
		})
	default:
		err = errors.New("unsupported application command")
	}
	return response(command, data, err)
}

func (a Adapter) ServeJSON(ctx context.Context, input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	encoder := json.NewEncoder(output)
	for {
		var request Request
		if err := decoder.Decode(&request); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := encoder.Encode(a.Handle(ctx, request)); err != nil {
			return err
		}
	}
}

func RenderJSON(output io.Writer, response Response) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(response)
}

func response(command string, data any, err error) Response {
	if err != nil {
		return Response{OK: false, Command: command, Error: err.Error()}
	}
	return Response{OK: true, Command: command, Data: data}
}
