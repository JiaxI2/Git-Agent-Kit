package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryStateHelpers(t *testing.T) {
	ctx := context.Background()
	repo := initTestRepository(t, ctx)

	branch, err := Branch(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("branch=%q", branch)
	}
	head, err := Head(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) < 7 {
		t.Fatalf("head=%q", head)
	}
	clean, err := IsClean(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !clean {
		t.Fatal("newly committed repository should be clean")
	}

	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean, err = IsClean(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if clean {
		t.Fatal("untracked file should make repository dirty")
	}
}

func TestRunIncludesGitDiagnostics(t *testing.T) {
	_, err := Run(context.Background(), t.TempDir(), "rev-parse", "missing-ref")
	if err == nil {
		t.Fatal("expected git failure")
	}
	if !strings.Contains(err.Error(), "rev-parse missing-ref") {
		t.Fatalf("error lacks command context: %v", err)
	}
}

func initTestRepository(t *testing.T, ctx context.Context) string {
	t.Helper()
	repo := t.TempDir()
	if _, err := Run(ctx, repo, "init", "--initial-branch=main"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, repo, "config", "user.email", "gia-test@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, repo, "config", "user.name", "GIA Test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, repo, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(ctx, repo, "commit", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	return repo
}
