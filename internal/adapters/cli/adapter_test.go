package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/app"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type taskStore struct {
	task domain.Task
}

func (s *taskStore) List(context.Context) ([]domain.Task, error) { return []domain.Task{s.task}, nil }
func (s *taskStore) Get(context.Context, domain.TaskID) (domain.Task, error) {
	return s.task, nil
}
func (s *taskStore) Save(_ context.Context, task domain.Task) error {
	s.task = task
	return nil
}

type repositoryPort struct{ mode string }

func (p *repositoryPort) Inspect(_ context.Context, request domain.RepositoryRequest) (domain.RepositoryReport, error) {
	p.mode = request.Mode
	return domain.RepositoryReport{OK: true, Repo: request.Repository}, nil
}

func adapterForTest() (Adapter, *taskStore, *repositoryPort) {
	store := &taskStore{task: domain.Task{ID: "3", Title: "Architecture V2", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}}
	repository := &repositoryPort{}
	return Adapter{Services: app.Services{
		Tasks:      app.TaskService{Tasks: store},
		Repository: app.RepositoryService{Repository: repository},
	}}, store, repository
}

func TestAliasesDelegateToTaskService(t *testing.T) {
	adapter, store, _ := adapterForTest()
	for _, command := range []string{"task.list", "issue.list", "scan"} {
		result := adapter.Handle(context.Background(), Request{Command: command})
		if !result.OK || result.ExitCode() != 0 {
			t.Fatalf("%s failed: %+v", command, result)
		}
	}
	claim := adapter.Handle(context.Background(), Request{
		Command: "claim", TaskID: "3", Executor: "remote-agent", Mode: "remote",
		Capabilities: []string{"remote.issue"},
	})
	if !claim.OK || store.task.State != domain.TaskClaimed || store.task.Executor == nil {
		t.Fatalf("claim alias did not delegate: %+v task=%+v", claim, store.task)
	}
}

func TestDoctorAndStatusSelectRepositoryMode(t *testing.T) {
	adapter, _, repository := adapterForTest()
	if result := adapter.Handle(context.Background(), Request{Command: "doctor", Repository: "."}); !result.OK || repository.mode != "doctor" {
		t.Fatalf("doctor result=%+v mode=%q", result, repository.mode)
	}
	if result := adapter.Handle(context.Background(), Request{Command: "status", Repository: "."}); !result.OK || repository.mode != "inspect" {
		t.Fatalf("status result=%+v mode=%q", result, repository.mode)
	}
}

func TestServeJSONPreservesMachineOutputAndErrors(t *testing.T) {
	adapter, _, _ := adapterForTest()
	input := bytes.NewBufferString("{\"command\":\"issue.list\"}\n{\"command\":\"unknown\"}\n")
	var output bytes.Buffer
	if err := adapter.ServeJSON(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var first, second Response
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if !first.OK || second.OK || second.ExitCode() != 1 {
		t.Fatalf("responses first=%+v second=%+v", first, second)
	}
}
