package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func Run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func Head(ctx context.Context, repo string) (string, error) {
	return Run(ctx, repo, "rev-parse", "HEAD")
}
func Branch(ctx context.Context, repo string) (string, error) {
	return Run(ctx, repo, "branch", "--show-current")
}
func StatusPorcelain(ctx context.Context, repo string) (string, error) {
	return Run(ctx, repo, "status", "--porcelain=v1")
}
func IsClean(ctx context.Context, repo string) (bool, error) {
	s, err := StatusPorcelain(ctx, repo)
	return s == "", err
}
func Fetch(ctx context.Context, repo string) error {
	_, err := Run(ctx, repo, "fetch", "--prune", "origin")
	return err
}

func CreateWorktree(ctx context.Context, repo, path, branch, ref string) error {
	if _, err := Run(ctx, repo, "worktree", "add", "-b", branch, path, ref); err != nil {
		return err
	}
	return nil
}

func RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	clean, err := IsClean(ctx, path)
	if err != nil {
		return err
	}
	if !clean && !force {
		return fmt.Errorf("worktree %s is dirty; commit/stash changes or pass --force explicitly", path)
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, filepath.Clean(path))
	_, err = Run(ctx, repo, args...)
	return err
}
