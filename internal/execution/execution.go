// Package execution implements capability-bearing local and remote executors.
package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

type CommandRunner interface {
	Run(context.Context, string, string, ...string) ([]byte, error)
}

type FileWriter interface {
	WriteFile(string, []byte, os.FileMode) error
}

type OSCommandRunner struct{}

func (OSCommandRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	return command.CombinedOutput()
}

type OSFileWriter struct{}

func (OSFileWriter) WriteFile(path string, content []byte, mode os.FileMode) error {
	return os.WriteFile(path, content, mode)
}

type LocalExecutor struct {
	Name   string
	Root   string
	Runner CommandRunner
	Writer FileWriter
	Now    func() time.Time
}

func (e LocalExecutor) Identity() domain.ExecutorIdentity {
	name := strings.TrimSpace(e.Name)
	if name == "" {
		name = "local"
	}
	return domain.ExecutorIdentity{Name: name, Mode: domain.ExecutionLocal, Capabilities: []domain.Capability{
		domain.CapabilityCommand, domain.CapabilityGit, domain.CapabilityWorktree,
		domain.CapabilityBuild, domain.CapabilityTest, domain.CapabilityHook,
		domain.CapabilityFilesystem,
	}}
}

func (e LocalExecutor) Execute(ctx context.Context, task domain.Task, effects []domain.Effect) ([]domain.Evidence, error) {
	if strings.TrimSpace(e.Root) == "" {
		return nil, errors.New("local executor repository root is required")
	}
	runner := e.Runner
	if runner == nil {
		runner = OSCommandRunner{}
	}
	writer := e.Writer
	if writer == nil {
		writer = OSFileWriter{}
	}
	now := e.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	identity := e.Identity()
	evidence := make([]domain.Evidence, 0, len(effects))
	for _, effect := range effects {
		if err := effect.Validate(); err != nil {
			return evidence, err
		}
		if !identity.Supports(effect.Requires...) {
			return evidence, fmt.Errorf("local executor lacks capabilities %v", effect.Requires)
		}
		switch effect.Kind {
		case domain.EffectCommand:
			dir, err := resolveExistingWithinRoot(e.Root, effect.Workdir)
			if err != nil {
				return evidence, err
			}
			output, runErr := runner.Run(ctx, dir, effect.Command, effect.Args...)
			item := domain.Evidence{
				Kind: "command", Source: identity.Name, Summary: strings.TrimSpace(string(output)),
				Observed: now(), Metadata: map[string]any{"taskId": task.ID, "command": effect.Command, "workdir": dir},
			}
			evidence = append(evidence, item)
			if runErr != nil {
				return evidence, fmt.Errorf("%s failed: %w", effect.Command, runErr)
			}
		case domain.EffectFileWrite:
			path, err := resolveWriteWithinRoot(e.Root, effect.Target)
			if err != nil {
				return evidence, err
			}
			if err := writer.WriteFile(path, []byte(effect.Parameters["content"]), 0o644); err != nil {
				return evidence, err
			}
			evidence = append(evidence, domain.Evidence{
				Kind: "file.write", Source: identity.Name, Summary: filepath.ToSlash(effect.Target),
				Observed: now(), Metadata: map[string]any{"taskId": task.ID, "path": path},
			})
		default:
			return evidence, fmt.Errorf("local executor does not support effect %q", effect.Kind)
		}
	}
	return evidence, nil
}

type RemoteClient interface {
	Apply(context.Context, domain.Task, domain.Effect) (domain.Evidence, error)
}

type RemoteExecutor struct {
	Name   string
	Client RemoteClient
	Now    func() time.Time
}

func (e RemoteExecutor) Identity() domain.ExecutorIdentity {
	name := strings.TrimSpace(e.Name)
	if name == "" {
		name = "remote"
	}
	return domain.ExecutorIdentity{Name: name, Mode: domain.ExecutionRemote, Capabilities: []domain.Capability{
		domain.CapabilityIssue, domain.CapabilityBranch, domain.CapabilityCommit,
		domain.CapabilityPullRequest, domain.CapabilityReview, domain.CapabilityCI,
	}}
}

func (e RemoteExecutor) Execute(ctx context.Context, task domain.Task, effects []domain.Effect) ([]domain.Evidence, error) {
	if e.Client == nil {
		return nil, errors.New("remote client is required")
	}
	now := e.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	identity := e.Identity()
	evidence := make([]domain.Evidence, 0, len(effects))
	for _, effect := range effects {
		if err := effect.Validate(); err != nil {
			return evidence, err
		}
		if effect.Kind != domain.EffectRemoteOperation {
			return evidence, fmt.Errorf("remote executor does not support effect %q", effect.Kind)
		}
		if !identity.Supports(effect.Requires...) {
			return evidence, fmt.Errorf("remote executor lacks capabilities %v", effect.Requires)
		}
		item, err := e.Client.Apply(ctx, task, effect)
		if err != nil {
			return evidence, err
		}
		if item.Observed.IsZero() {
			item.Observed = now()
		}
		evidence = append(evidence, item)
	}
	return evidence, nil
}

func resolveExistingWithinRoot(root, candidate string) (string, error) {
	rootPath, err := canonicalExisting(root)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(candidate) == "" {
		return rootPath, nil
	}
	path := candidate
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, path)
	}
	path, err = canonicalExisting(path)
	if err != nil {
		return "", err
	}
	if !within(rootPath, path) {
		return "", fmt.Errorf("path resolves outside repository root: %s", path)
	}
	return path, nil
}

func resolveWriteWithinRoot(root, candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		return "", errors.New("file effect target is required")
	}
	rootPath, err := canonicalExisting(root)
	if err != nil {
		return "", err
	}
	path := candidate
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := canonicalExisting(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	path = filepath.Join(parent, filepath.Base(path))
	if !within(rootPath, path) {
		return "", fmt.Errorf("path resolves outside repository root: %s", path)
	}
	return path, nil
}

func canonicalExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}
