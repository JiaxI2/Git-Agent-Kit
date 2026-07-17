package v1_test

import (
	"context"
	"testing"

	sdk "github.com/JiaxI2/git-isolated-agent-kit/pkg/sdk/v1"
)

type store struct{ task sdk.Task }

func (s *store) List(context.Context) ([]sdk.Task, error) { return []sdk.Task{s.task}, nil }
func (s *store) Get(context.Context, string) (sdk.Task, error) {
	return s.task, nil
}
func (s *store) Save(_ context.Context, task sdk.Task) error {
	s.task = task
	return nil
}

func TestExternalConsumerUsesOnlySDKTypes(t *testing.T) {
	tasks := &store{task: sdk.Task{ID: "3", Title: "Architecture V2", State: "ready", Risk: sdk.RiskMedium, Mode: sdk.ModeRemote}}
	client := sdk.New(sdk.Ports{Tasks: tasks})
	items, err := client.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "3" {
		t.Fatalf("items=%+v", items)
	}
	claimed, err := client.ClaimTask(context.Background(), "3", sdk.ExecutorIdentity{
		Name: "agent", Mode: sdk.ModeRemote, Capabilities: []sdk.Capability{"remote.issue"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.State != "claimed" || tasks.task.Executor == nil {
		t.Fatalf("claimed=%+v stored=%+v", claimed, tasks.task)
	}
}
