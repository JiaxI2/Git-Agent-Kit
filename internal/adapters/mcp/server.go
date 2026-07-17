// Package mcp exposes application services over MCP JSON-RPC stdio messages.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

const protocolVersion = "2025-03-26"

type Server struct {
	Services   app.Services
	Repository string
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type toolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type resourceRead struct {
	URI string `json:"uri"`
}

type promptGet struct {
	Name string `json:"name"`
}

func (s Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if err := encoder.Encode(failure(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		response := s.Handle(ctx, request)
		if response == nil {
			continue
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s Server) Handle(ctx context.Context, request Request) *Response {
	if len(request.ID) == 0 {
		return nil
	}
	if request.JSONRPC != "2.0" {
		response := failure(request.ID, -32600, "invalid request")
		return &response
	}
	var response Response
	switch request.Method {
	case "initialize":
		response = success(request.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{}, "resources": map[string]any{}, "prompts": map[string]any{},
			},
			"serverInfo": map[string]string{"name": "gia-mcp", "version": "current"},
		})
	case "tools/list":
		response = success(request.ID, map[string]any{"tools": tools()})
	case "tools/call":
		var call toolCall
		if err := json.Unmarshal(request.Params, &call); err != nil || strings.TrimSpace(call.Name) == "" {
			response = failure(request.ID, -32602, "invalid tool arguments")
			break
		}
		response = success(request.ID, s.callTool(ctx, call))
	case "resources/list":
		response = success(request.ID, map[string]any{"resources": resources()})
	case "resources/templates/list":
		response = success(request.ID, map[string]any{"resourceTemplates": resourceTemplates()})
	case "resources/read":
		var read resourceRead
		if err := json.Unmarshal(request.Params, &read); err != nil || strings.TrimSpace(read.URI) == "" {
			response = failure(request.ID, -32602, "invalid resource arguments")
			break
		}
		result, err := s.readResource(ctx, read.URI)
		if err != nil {
			response = failure(request.ID, -32602, err.Error())
			break
		}
		response = success(request.ID, result)
	case "prompts/list":
		response = success(request.ID, map[string]any{"prompts": prompts()})
	case "prompts/get":
		var get promptGet
		if err := json.Unmarshal(request.Params, &get); err != nil || strings.TrimSpace(get.Name) == "" {
			response = failure(request.ID, -32602, "invalid prompt arguments")
			break
		}
		result, err := getPrompt(get.Name)
		if err != nil {
			response = failure(request.ID, -32602, err.Error())
			break
		}
		response = success(request.ID, result)
	default:
		response = failure(request.ID, -32601, "method not found")
	}
	return &response
}

func (s Server) callTool(ctx context.Context, call toolCall) toolResult {
	var value any
	var err error
	switch call.Name {
	case "gia_task_list":
		value, err = s.Services.Tasks.List(ctx)
	case "gia_task_get":
		value, err = s.Services.Tasks.Get(ctx, domain.TaskID(stringArgument(call.Arguments, "taskId")))
	case "gia_task_claim":
		capabilities := capabilityArguments(call.Arguments["capabilities"])
		if len(capabilities) == 0 {
			capabilities = []domain.Capability{domain.CapabilityIssue, domain.CapabilityBranch}
		}
		mode := domain.ExecutionMode(stringArgument(call.Arguments, "mode"))
		if mode == "" {
			mode = domain.ExecutionRemote
		}
		value, err = s.Services.Tasks.Claim(ctx, domain.TaskID(stringArgument(call.Arguments, "taskId")), domain.ExecutorIdentity{
			Name: stringArgument(call.Arguments, "executor"), Mode: mode, Capabilities: capabilities,
		})
	case "gia_validation_plan":
		profile := stringArgument(call.Arguments, "profile")
		if profile == "" {
			profile = "full"
		}
		value, err = s.Services.Validation.Plan(ctx, domain.TaskID(stringArgument(call.Arguments, "taskId")), profile)
	case "gia_doctor":
		value, err = s.Services.Repository.Inspect(ctx, domain.RepositoryRequest{Repository: s.Repository, Mode: "doctor"})
	case "gia_repo_inspect":
		value, err = s.Services.Repository.Inspect(ctx, domain.RepositoryRequest{Repository: s.Repository, Mode: "inspect"})
	case "gia_plan_create":
		var request domain.CreatePlanRequest
		request, err = planRequest(call.Arguments)
		if err == nil {
			if strings.TrimSpace(request.Repository) == "" {
				request.Repository = s.Repository
			}
			value, err = s.Services.Plans.Create(ctx, request)
		}
	case "gia_plan_show":
		value, err = s.Services.Plans.Show(ctx, domain.PlanID(stringArgument(call.Arguments, "planId")))
	case "gia_plan_diff":
		value, err = s.Services.Plans.Diff(ctx, domain.PlanID(stringArgument(call.Arguments, "planId")))
	case "gia_plan_apply":
		value, err = s.Services.Plans.Apply(ctx, domain.PlanID(stringArgument(call.Arguments, "planId")))
	default:
		err = errors.New("unknown tool")
	}
	if err != nil {
		return toolResult{IsError: true, Content: []content{{Type: "text", Text: err.Error()}}}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return toolResult{IsError: true, Content: []content{{Type: "text", Text: err.Error()}}}
	}
	return toolResult{Content: []content{{Type: "text", Text: string(encoded)}}}
}

func (s Server) readResource(ctx context.Context, uri string) (map[string]any, error) {
	var value any
	switch uri {
	case "gia://tasks":
		tasks, err := s.Services.Tasks.List(ctx)
		if err != nil {
			return nil, err
		}
		value = tasks
	case "gia://repository":
		report, err := s.Services.Repository.Inspect(ctx, domain.RepositoryRequest{Repository: s.Repository, Mode: "inspect"})
		if err != nil {
			return nil, err
		}
		value = report
	case "gia://validation":
		value = map[string]any{"tool": "gia_validation_plan", "profiles": []string{"smoke", "full", "release"}}
	default:
		const planPrefix = "gia://plans/"
		if !strings.HasPrefix(uri, planPrefix) {
			return nil, fmt.Errorf("unknown resource %q", uri)
		}
		id := strings.TrimSpace(strings.TrimPrefix(uri, planPrefix))
		if id == "" || strings.Contains(id, "/") {
			return nil, errors.New("plan resource requires one plan id")
		}
		record, err := s.Services.Plans.Show(ctx, domain.PlanID(id))
		if err != nil {
			return nil, err
		}
		value = record
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return map[string]any{"contents": []map[string]string{{"uri": uri, "mimeType": "application/json", "text": string(encoded)}}}, nil
}

func tools() []map[string]any {
	object := schema()
	return []map[string]any{
		{"name": "gia_task_list", "description": "List governed tasks", "inputSchema": object},
		{"name": "gia_task_get", "description": "Get one governed task", "inputSchema": schema("taskId")},
		{"name": "gia_task_claim", "description": "Claim a governed task", "inputSchema": schema("taskId", "executor")},
		{"name": "gia_validation_plan", "description": "Build a capability-aware validation plan", "inputSchema": schema("taskId")},
		{"name": "gia_doctor", "description": "Inspect governance prerequisites", "inputSchema": object},
		{"name": "gia_repo_inspect", "description": "Inspect repository governance metadata", "inputSchema": object},
		{"name": "gia_plan_create", "description": "Create an immutable execution plan without applying effects", "inputSchema": planCreateSchema()},
		{"name": "gia_plan_show", "description": "Show a persisted execution plan", "inputSchema": schema("planId")},
		{"name": "gia_plan_diff", "description": "Compare a plan with current repository state", "inputSchema": schema("planId")},
		{"name": "gia_plan_apply", "description": "Apply a ready plan exactly once", "inputSchema": schema("planId")},
	}
}

func resources() []map[string]string {
	return []map[string]string{
		{"uri": "gia://tasks", "name": "Governed tasks", "mimeType": "application/json"},
		{"uri": "gia://repository", "name": "Repository governance", "mimeType": "application/json"},
		{"uri": "gia://validation", "name": "Validation entrypoint", "mimeType": "application/json"},
	}
}

func resourceTemplates() []map[string]string {
	return []map[string]string{
		{"uriTemplate": "gia://plans/{planId}", "name": "Persisted execution plan", "mimeType": "application/json"},
	}
}

func prompts() []map[string]any {
	return []map[string]any{
		{"name": "gia_plan_task", "description": "Plan a governed repository task"},
		{"name": "gia_review", "description": "Review changes against repository policy"},
		{"name": "gia_validate", "description": "Build a capability-aware validation plan"},
	}
}

func getPrompt(name string) (map[string]any, error) {
	text := map[string]string{
		"gia_plan_task": "Inspect the governed task, policy, and required capabilities before proposing effects.",
		"gia_review":    "Review the exact diff, policy decisions, evidence, and unresolved validation before approval.",
		"gia_validate":  "Build a validation plan whose evidence is bound to the exact task and commit.",
	}[name]
	if text == "" {
		return nil, fmt.Errorf("unknown prompt %q", name)
	}
	return map[string]any{"description": name, "messages": []map[string]any{{"role": "user", "content": map[string]string{"type": "text", "text": text}}}}, nil
}

func schema(required ...string) map[string]any {
	properties := map[string]any{}
	for _, name := range required {
		properties[name] = map[string]string{"type": "string"}
	}
	value := map[string]any{"type": "object", "properties": properties, "additionalProperties": true}
	if len(required) > 0 {
		value["required"] = required
	}
	return value
}

func planCreateSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"repository":    map[string]string{"type": "string"},
			"task":          map[string]string{"type": "object"},
			"effects":       map[string]string{"type": "array"},
			"preferredMode": map[string]string{"type": "string"},
		},
		"required":             []string{"task", "effects"},
		"additionalProperties": false,
	}
}

func planRequest(arguments map[string]any) (domain.CreatePlanRequest, error) {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return domain.CreatePlanRequest{}, err
	}
	var request domain.CreatePlanRequest
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return domain.CreatePlanRequest{}, fmt.Errorf("invalid plan request: %w", err)
	}
	return request, nil
}

func stringArgument(arguments map[string]any, name string) string {
	value, _ := arguments[name].(string)
	return strings.TrimSpace(value)
}

func capabilityArguments(value any) []domain.Capability {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	capabilities := make([]domain.Capability, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			capabilities = append(capabilities, domain.Capability(text))
		}
	}
	return capabilities
}

func success(id json.RawMessage, result any) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: result}
}

func failure(id json.RawMessage, code int, message string) Response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}}
}
