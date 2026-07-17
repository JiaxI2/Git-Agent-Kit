package execution

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type recordedCommand struct {
	dir  string
	name string
	args []string
}

type recordingRunner struct{ call recordedCommand }

func (r *recordingRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	r.call = recordedCommand{dir: dir, name: name, args: append([]string(nil), args...)}
	return []byte("ok"), nil
}

type remoteClient struct{ called bool }

func (c *remoteClient) Apply(context.Context, domain.Task, domain.Effect) (domain.Evidence, error) {
	c.called = true
	return domain.Evidence{Kind: "ci", Source: "github", Summary: "success"}, nil
}

func executionTask() domain.Task {
	return domain.Task{ID: "3", Title: "Architecture", State: domain.TaskReady, Risk: domain.RiskMedium, Mode: domain.ExecutionLocal}
}

func TestLocalExecutorPreservesArgumentsAndRepositoryDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "path with spaces")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	executor := LocalExecutor{Root: root, Runner: runner}
	effect := domain.Effect{
		Kind: domain.EffectCommand, Workdir: "path with spaces", Command: "go",
		Args: []string{"test", "./package with spaces"}, Requires: []domain.Capability{domain.CapabilityTest},
	}
	if _, err := executor.Execute(context.Background(), executionTask(), []domain.Effect{effect}); err != nil {
		t.Fatal(err)
	}
	expectedDir, err := canonicalExisting(dir)
	if err != nil {
		t.Fatal(err)
	}
	if runner.call.dir != expectedDir || runner.call.name != "go" || !reflect.DeepEqual(runner.call.args, effect.Args) {
		t.Fatalf("command boundary changed: %+v", runner.call)
	}
}

func TestLocalExecutorRejectsRepositoryEscape(t *testing.T) {
	root := t.TempDir()
	effect := domain.Effect{Kind: domain.EffectCommand, Workdir: "..", Command: "go", Args: []string{"test"}, Requires: []domain.Capability{domain.CapabilityTest}}
	_, err := (LocalExecutor{Root: root, Runner: &recordingRunner{}}).Execute(context.Background(), executionTask(), []domain.Effect{effect})
	if err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("escape error=%v", err)
	}
}

func TestLocalExecutorRejectsSymlinkWriteEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	effect := domain.Effect{Kind: domain.EffectFileWrite, Target: filepath.Join("link", "data.txt"), Parameters: map[string]string{"content": "x"}, Requires: []domain.Capability{domain.CapabilityFilesystem}}
	_, err := (LocalExecutor{Root: root}).Execute(context.Background(), executionTask(), []domain.Effect{effect})
	if err == nil || !strings.Contains(err.Error(), "outside repository root") {
		t.Fatalf("symlink escape error=%v", err)
	}
}

func TestRemoteExecutorAppliesOnlySupportedEffects(t *testing.T) {
	client := &remoteClient{}
	executor := RemoteExecutor{Client: client}
	effect := domain.Effect{Kind: domain.EffectRemoteOperation, Requires: []domain.Capability{domain.CapabilityCI}}
	if _, err := executor.Execute(context.Background(), executionTask(), []domain.Effect{effect}); err != nil || !client.called {
		t.Fatalf("remote execution failed called=%t err=%v", client.called, err)
	}
	local := domain.Effect{Kind: domain.EffectCommand, Command: "go", Requires: []domain.Capability{domain.CapabilityCommand}}
	if _, err := executor.Execute(context.Background(), executionTask(), []domain.Effect{local}); err == nil {
		t.Fatal("remote executor accepted local command")
	}
}
