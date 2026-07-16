package validate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
)

func TestTrim(t *testing.T) {
	if trim("abcdef", 3) != "abc\n...truncated" {
		t.Fatal("trim")
	}
}

func TestRunRequiresExpectedHeadReachableFromConfiguredRemote(t *testing.T) {
	_, repo, cfg := newValidationRepo(t, "review")
	writeCommit(t, repo, "feature.txt", "pushed\n", "pushed feature")
	head := validationGit(t, repo, "rev-parse", "HEAD")
	validationGit(t, repo, "push", "-u", "review", "verify/test")

	report, err := Run(context.Background(), repo, "test", head, cfg)
	if err != nil {
		t.Fatalf("Run() pushed commit error = %v", err)
	}
	if !report.OK || report.Remote != "review" {
		t.Fatalf("Run() report = %+v", report)
	}

	writeCommit(t, repo, "local-only.txt", "not pushed\n", "local only")
	unpushed := validationGit(t, repo, "rev-parse", "HEAD")
	if _, err := Run(context.Background(), repo, "test", unpushed, cfg); err == nil || !strings.Contains(err.Error(), "not the current tip") {
		t.Fatalf("Run() unpushed error = %v, want remote reachability failure", err)
	}
}

func TestRunRejectsStaleRemoteHead(t *testing.T) {
	_, repo, cfg := newValidationRepo(t, "origin")
	writeCommit(t, repo, "feature.txt", "first\n", "first feature")
	stale := validationGit(t, repo, "rev-parse", "HEAD")
	validationGit(t, repo, "push", "-u", "origin", "verify/test")
	writeCommit(t, repo, "feature.txt", "second\n", "advance feature")
	validationGit(t, repo, "push", "origin", "verify/test")
	validationGit(t, repo, "branch", "verify/stale", stale)
	validationGit(t, repo, "checkout", "verify/stale")

	if _, err := Run(context.Background(), repo, "test", stale, cfg); err == nil || !strings.Contains(err.Error(), "not the current tip") {
		t.Fatalf("Run() stale remote HEAD error = %v", err)
	}
}

func TestRunRejectsProtectedPath(t *testing.T) {
	_, repo, cfg := newValidationRepo(t, "origin")
	path := filepath.Join(repo, ".github", "workflows", "ci.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCommit(t, repo, ".github/workflows/ci.yml", "name: protected\n", "change workflow")
	head := validationGit(t, repo, "rev-parse", "HEAD")
	validationGit(t, repo, "push", "-u", "origin", "verify/test")

	if _, err := Run(context.Background(), repo, "test", head, cfg); err == nil || !strings.Contains(err.Error(), ".github/workflows/ci.yml") {
		t.Fatalf("Run() protected path error = %v", err)
	}
}

func TestRunRejectsProtectedAndDetachedBranches(t *testing.T) {
	t.Run("protected branch", func(t *testing.T) {
		_, repo, cfg := newValidationRepo(t, "origin")
		validationGit(t, repo, "checkout", "main")
		head := validationGit(t, repo, "rev-parse", "HEAD")
		if _, err := Run(context.Background(), repo, "test", head, cfg); err == nil || !strings.Contains(err.Error(), "protected.branches") {
			t.Fatalf("Run() protected branch error = %v", err)
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		_, repo, cfg := newValidationRepo(t, "origin")
		writeCommit(t, repo, "feature.txt", "pushed\n", "pushed feature")
		head := validationGit(t, repo, "rev-parse", "HEAD")
		validationGit(t, repo, "push", "-u", "origin", "verify/test")
		validationGit(t, repo, "checkout", "--detach", head)
		if _, err := Run(context.Background(), repo, "test", head, cfg); err == nil || !strings.Contains(err.Error(), "detached HEAD") {
			t.Fatalf("Run() detached HEAD error = %v", err)
		}
	})
}

func newValidationRepo(t *testing.T, remoteName string) (string, string, config.Config) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "repo")
	validationGit(t, root, "init", "--bare", remote)
	validationGit(t, root, "init", "-b", "main", repo)
	validationGit(t, repo, "config", "user.name", "GIA Test")
	validationGit(t, repo, "config", "user.email", "gia@example.invalid")
	writeCommit(t, repo, "README.md", "validation test\n", "initial")
	validationGit(t, repo, "remote", "add", remoteName, remote)
	validationGit(t, repo, "push", "-u", remoteName, "main")
	validationGit(t, repo, "checkout", "-b", "verify/test")

	cfg := config.Default()
	cfg.Validation.Remote = remoteName
	cfg.Validation.Profiles["test"] = []string{}
	return remote, repo, cfg
}

func writeCommit(t *testing.T, repo, name, content, message string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	validationGit(t, repo, "add", "--", name)
	validationGit(t, repo, "commit", "-m", message)
}

func validationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
