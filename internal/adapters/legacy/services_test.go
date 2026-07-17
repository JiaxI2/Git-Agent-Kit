package legacy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/issue"
)

func configuredRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(filepath.Join(dir, "config.json"), config.Default()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestValidationPlannerMapsProfileToCapabilityEffects(t *testing.T) {
	adapter := &Adapter{Repository: configuredRepository(t)}
	task := domain.Task{ID: "3", Title: "Architecture", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionRemote}
	plan, err := adapter.Plan(context.Background(), task, "full")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Effects) == 0 || len(plan.Requires) == 0 || plan.TaskID != "3" {
		t.Fatalf("plan=%+v", plan)
	}
	for _, effect := range plan.Effects {
		if effect.Command == "" || len(effect.Args) == 0 {
			t.Fatalf("shell boundary missing: %+v", effect)
		}
	}
}

func TestTaskMappingPreservesIssueIdentityAndLabels(t *testing.T) {
	cfg := config.Default()
	task := taskFromItem(issue.Item{Number: 3, Title: "Architecture", URL: "https://example.invalid/3", Labels: []string{cfg.Issue.ClaimedLabel, "executor:web-agent", "risk:high"}}, cfg)
	if task.ID != "3" || task.Issue != 3 || task.State != domain.TaskClaimed || task.Risk != domain.RiskHigh || task.Executor == nil || task.Executor.Name != "web-agent" {
		t.Fatalf("task=%+v", task)
	}
}

func TestNewServicesExposesAllMigrationPorts(t *testing.T) {
	services := NewServices(configuredRepository(t), "")
	if services.Tasks.Tasks == nil || services.Tasks.Issues == nil || services.Tasks.Claimer == nil ||
		services.Validation.Planner == nil || services.Workspace.Workspace == nil ||
		services.PullRequests.PullRequests == nil || services.Repository.Repository == nil {
		t.Fatalf("incomplete services: %+v", services)
	}
}
