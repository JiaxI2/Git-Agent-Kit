package planstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/gitx"
)

func TestStorePersistsOutsideWorktreeAndAcrossInstances(t *testing.T) {
	repo, configPath := testRepository(t)
	store := New(repo, configPath)
	snapshot, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	plan := testPlan(t, snapshot)
	before, err := gitx.StatusPorcelain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	after, err := gitx.StatusPorcelain(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("plan persistence polluted worktree: before=%q after=%q", before, after)
	}
	record, err := New(repo, configPath).Get(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Plan.ID != plan.ID || record.Status.State != domain.PlanPlanned {
		t.Fatalf("unexpected stored plan: %+v", record)
	}
}

func TestStoreApplyLeaseAllowsExactlyOneAttempt(t *testing.T) {
	repo, configPath := testRepository(t)
	store := New(repo, configPath)
	snapshot, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	plan := testPlan(t, snapshot)
	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var successes atomic.Int32
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			if store.BeginApply(context.Background(), plan.ID, time.Unix(2, 0).UTC()) == nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	group.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful apply leases=%d", successes.Load())
	}
	record, err := store.Get(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status.State != domain.PlanApplying {
		t.Fatalf("status=%s", record.Status.State)
	}
	result := domain.PlanApplyResult{PlanID: plan.ID, State: domain.PlanApplied}
	if err := store.FinishApply(context.Background(), result, time.Unix(3, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishApply(context.Background(), result, time.Unix(4, 0).UTC()); err == nil {
		t.Fatal("second plan result was accepted")
	}
	record, err = New(repo, configPath).Get(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status.State != domain.PlanApplied || record.Status.StartedAt.IsZero() || record.Status.FinishedAt.IsZero() {
		t.Fatalf("completed status=%+v", record.Status)
	}
}

func TestSnapshotDetectsConfigAndHeadChanges(t *testing.T) {
	repo, configPath := testRepository(t)
	store := New(repo, configPath)
	initial, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	configChanged, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if configChanged.Head != initial.Head || configChanged.ConfigDigest == initial.ConfigDigest {
		t.Fatalf("config change snapshot initial=%+v changed=%+v", initial, configChanged)
	}
	if _, err := gitx.Run(context.Background(), repo, "add", ".gia/config.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(context.Background(), repo, "commit", "-m", "test: change config"); err != nil {
		t.Fatal(err)
	}
	headChanged, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if headChanged.Head == initial.Head {
		t.Fatalf("HEAD did not change: %+v", headChanged)
	}
}

func TestStoreRejectsTamperedPlan(t *testing.T) {
	repo, configPath := testRepository(t)
	store := New(repo, configPath)
	snapshot, err := store.Snapshot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	plan := testPlan(t, snapshot)
	if err := store.Create(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	paths, err := store.paths(context.Background(), plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(paths.plan)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "Inspectable Plan", "Tampered Plan", 1))
	if err := os.WriteFile(paths.plan, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), plan.ID); err == nil || !strings.Contains(err.Error(), "plan id mismatch") {
		t.Fatalf("tampered plan error=%v", err)
	}
}

func testRepository(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	if _, err := gitx.Run(context.Background(), repo, "init"); err != nil {
		t.Fatal(err)
	}
	for _, setting := range [][2]string{{"user.name", "GIA Test"}, {"user.email", "gia@example.invalid"}} {
		if _, err := gitx.Run(context.Background(), repo, "config", setting[0], setting[1]); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(repo, ".gia", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(configPath, config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(context.Background(), repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.Run(context.Background(), repo, "commit", "-m", "test: initialize repository"); err != nil {
		t.Fatal(err)
	}
	return repo, configPath
}

func testPlan(t *testing.T, snapshot domain.PlanSnapshot) domain.Plan {
	t.Helper()
	plan, err := (domain.Plan{
		Repository: snapshot.Repository, BaseHead: snapshot.Head, ConfigDigest: snapshot.ConfigDigest,
		Task:            domain.Task{ID: "5", Title: "Inspectable Plan", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionLocal},
		Effects:         []domain.Effect{{Kind: domain.EffectCommand, Command: "go", Args: []string{"test", "./..."}, Requires: []domain.Capability{domain.CapabilityCommand}}},
		PolicyDecisions: []domain.PlanPolicyDecision{{Operation: domain.OperationPlanCreate, Decision: domain.PolicyDecision{Allowed: true}}},
		PreferredMode:   domain.ExecutionLocal, CreatedAt: time.Unix(1, 0).UTC(),
	}).Seal()
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
