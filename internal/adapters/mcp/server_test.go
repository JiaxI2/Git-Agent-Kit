package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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

type planPort struct{ record domain.PlanRecord }

func (p *planPort) Create(_ context.Context, plan domain.Plan) error {
	p.record = domain.PlanRecord{Plan: plan, Status: domain.PlanStatus{PlanID: plan.ID, State: domain.PlanPlanned}}
	return nil
}
func (p *planPort) Get(_ context.Context, id domain.PlanID) (domain.PlanRecord, error) {
	if p.record.Plan.ID != id {
		return domain.PlanRecord{}, errors.New("plan not found")
	}
	return p.record, nil
}
func (p *planPort) BeginApply(_ context.Context, id domain.PlanID, started time.Time) error {
	if p.record.Plan.ID != id || p.record.Status.State != domain.PlanPlanned {
		return errors.New("plan already attempted")
	}
	p.record.Status.State = domain.PlanApplying
	p.record.Status.StartedAt = started
	return nil
}
func (p *planPort) FinishApply(_ context.Context, result domain.PlanApplyResult, finished time.Time) error {
	p.record.Status.State = result.State
	p.record.Status.FinishedAt = finished
	return nil
}

type planContext struct{}

func (planContext) Snapshot(context.Context, string) (domain.PlanSnapshot, error) {
	return domain.PlanSnapshot{Repository: "C:/repo", Head: strings.Repeat("a", 40), ConfigDigest: "sha256:" + strings.Repeat("b", 64)}, nil
}

type planPolicy struct{}

func (planPolicy) Evaluate(context.Context, domain.Task, domain.Operation) domain.PolicyDecision {
	return domain.PolicyDecision{Allowed: true}
}

type planExecutor struct{}

func (planExecutor) Identity() domain.ExecutorIdentity {
	return domain.ExecutorIdentity{Name: "local", Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{domain.CapabilityCommand}}
}
func (planExecutor) Execute(context.Context, domain.Task, []domain.Effect) ([]domain.Evidence, error) {
	return []domain.Evidence{{Kind: "command", Source: "local", Summary: "ok"}}, nil
}

func testServer() Server {
	tasks := &taskPort{task: domain.Task{ID: "3", Title: "Architecture", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}}
	plans := &planPort{}
	execution := app.ExecutionService{Selector: app.CapabilitySelector{Executors: []app.Executor{planExecutor{}}}}
	return Server{Repository: ".", Services: app.Services{
		Tasks:      app.TaskService{Tasks: tasks},
		Validation: app.ValidationService{Tasks: tasks, Planner: planner{}},
		Repository: app.RepositoryService{Repository: repoPort{}},
		Execution:  execution,
		Plans: app.PlanService{
			Plans: plans, Context: planContext{}, Policy: planPolicy{}, Execution: execution,
		},
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
	if !ok || result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, "Architecture") {
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

func TestPlanToolsCreateDiffApplyAndRejectReplay(t *testing.T) {
	server := testServer()
	create := server.Handle(context.Background(), Request{
		JSONRPC: "2.0", ID: json.RawMessage("5"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"gia_plan_create","arguments":{"repository":"C:/repo","task":{"id":"5","title":"Inspectable Plan","state":"ready","risk":"medium","mode":"local"},"effects":[{"kind":"command","command":"go","args":["test","./..."],"requires":["local.command"]}]}}`),
	})
	created, ok := create.Result.(toolResult)
	if !ok || created.IsError || len(created.Content) != 1 {
		t.Fatalf("create result=%+v", create)
	}
	var plan domain.Plan
	if err := json.Unmarshal([]byte(created.Content[0].Text), &plan); err != nil || plan.ID == "" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	templates := server.Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("7"), Method: "resources/templates/list"})
	if templates.Error != nil || !strings.Contains(mustJSON(t, templates.Result), "gia://plans/{planId}") {
		t.Fatalf("templates=%+v", templates)
	}
	resourceParams, err := json.Marshal(map[string]string{"uri": "gia://plans/" + string(plan.ID)})
	if err != nil {
		t.Fatal(err)
	}
	resource := server.Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("8"), Method: "resources/read", Params: resourceParams})
	if resource.Error != nil || !strings.Contains(mustJSON(t, resource.Result), string(plan.ID)) {
		t.Fatalf("plan resource=%+v", resource)
	}
	call := func(name string) toolResult {
		t.Helper()
		params, err := json.Marshal(map[string]any{"name": name, "arguments": map[string]any{"planId": plan.ID}})
		if err != nil {
			t.Fatal(err)
		}
		response := server.Handle(context.Background(), Request{JSONRPC: "2.0", ID: json.RawMessage("6"), Method: "tools/call", Params: params})
		result, ok := response.Result.(toolResult)
		if !ok {
			t.Fatalf("%s result=%+v", name, response)
		}
		return result
	}
	if diff := call("gia_plan_diff"); diff.IsError || !strings.Contains(diff.Content[0].Text, `"ready":true`) {
		t.Fatalf("diff=%+v", diff)
	}
	if applied := call("gia_plan_apply"); applied.IsError || !strings.Contains(applied.Content[0].Text, `"state":"applied"`) {
		t.Fatalf("apply=%+v", applied)
	}
	if replay := call("gia_plan_apply"); !replay.IsError {
		t.Fatalf("replay=%+v", replay)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
