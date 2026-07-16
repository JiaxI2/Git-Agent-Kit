package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultHasSafeWorkflowDefaults(t *testing.T) {
	cfg := Default()
	if cfg.DefaultBranch != "main" {
		t.Fatalf("default branch=%q", cfg.DefaultBranch)
	}
	if cfg.Issue.ReadyLabel == "" || cfg.Issue.ClaimedLabel == "" {
		t.Fatal("issue workflow labels must not be empty")
	}
	if !cfg.Validation.RequireClean || !cfg.Validation.RequireExpectedHead {
		t.Fatal("validation must default to clean and SHA-bound")
	}
	if !cfg.Protected.RejectForcePush {
		t.Fatal("force push must be rejected by default")
	}
}

func TestSaveAndLoadFromRepo(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := Default()
	want.DefaultBranch = "trunk"
	if err := Save(filepath.Join(dir, "config.json"), want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFromRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultBranch != want.DefaultBranch {
		t.Fatalf("default branch=%q", got.DefaultBranch)
	}
	if len(got.Validation.Profiles["full"]) == 0 {
		t.Fatal("full validation profile was not preserved")
	}
}

func TestLoadFromRepoExplainsInitialization(t *testing.T) {
	repo := t.TempDir()
	_, err := LoadFromRepo(repo)
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !strings.Contains(err.Error(), "gia init --repo") {
		t.Fatalf("error lacks recovery guidance: %v", err)
	}
}

func TestLoadFromRepoRejectsInvalidJSON(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromRepo(repo); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
