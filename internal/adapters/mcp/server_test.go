package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type taskPort struct {
	task domain.Task
}

func (p *taskPort) List(context.Context) ([]domain.Task, error) { return []domain.Task{p.task}, nil }
func (p *taskPort) Get(context.Context, domain.TaskID) (domain.Task, error) {
	return p.task, nil
}
func (p *taskPort) Save(_ context.Context, task domain.Task) error {
	p.task = task
	return nil
}

type repoPort struct{}

func (repoPort) Inspect(_ context.Context, request domain.RepositoryRequest) (domain.RepositoryReport, error) {
	return domain.RepositoryReport{OK: true, Repo: request.Repository}, nil
}

type planner struct{}

func (planner) Plan(_ context.Context, task domain.Task, profile string) (domain.ValidationPlan, error) {
	return domain.ValidationPlan{TaskID: task.ID, Profile: profile, Requires: []domain.Capability{domain.CapabilityTest}}, nil
}

func testServer() Server {
	tasks := &taskPort{task: domain.Task{ID: "3", Title: "Architecture V2", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}}
	return Server{Repository: ".", Services: app.Services{
		Tasks:      app.TaskService{Tasks: tasks},
		Validation: app.ValidationService{Tasks: tasks, Planner: planner{}},
		Repository: app.RepositoryService{Repository: repoPort{}},
	}}
}

func TestInitializedNotificationProducesNoResponse(t *testing.T) {
	var output bytes.Buffer
	input := "{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n{\"jsonrpc\":\"2.0\",\"method\":\"tools/list\"}\n"
	if err := testServer().Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("notification produced output %q", output.String())
	}
}

func TestToolsCallApplicationServices(t *testing.T) {
	request := Request{JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "tools/call", Params: json.RawMessage(`{"name":"gia_task_list","arguments":{}}`)}
	response := testServer().Handle(context.Background(), request)
	if response == nil || response.Error != nil {
		t.Fatalf("tool call failed: %+v", response)
	}
	result, ok := response.Result.(toolResult)
	if !ok || result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "Architecture V2") {
		t.Fatalf("unexpected tool result: %#v", response.Result)
	}
}

func TestResourcesAndPromptsAreReadable(t *testing.T) {
	server := testServer()
	resource := server.Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("2"), Method: "resources/read", Params: json.RawMessage(`{"uri":"gia://tasks"}`)})
	if resource.Error != nil {
		t.Fatalf("resource read failed: %+v", resource.Error)
	}
	prompt := server.Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("3"), Method: "prompts/get", Params: json.RawMessage(`{"name":"gia_review"}`)})
	if prompt.Error != nil {
		t.Fatalf("prompt get failed: %+v", prompt.Error)
	}
}

func TestUnknownMethodReturnsJSONRPCError(t *testing.T) {
	response := testServer().Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("4"), Method: "unknown"})
	if response.Error == nil || response.Error.Code != -32601 {
		t.Fatalf("unexpected response: %+v", response)
	}
}
