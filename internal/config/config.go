package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	SchemaVersion int                `json:"schemaVersion"`
	DefaultBranch string             `json:"defaultBranch"`
	Issue         IssueConfig        `json:"issue"`
	Branches      BranchConfig       `json:"branches"`
	Worktrees     WorktreeConfig     `json:"worktrees"`
	Validation    ValidationConfig   `json:"validation"`
	Protected     ProtectedConfig    `json:"protected"`
	Notifications NotificationConfig `json:"notifications"`
}

type IssueConfig struct {
	ReadyLabel     string `json:"readyLabel"`
	ClaimedLabel   string `json:"claimedLabel"`
	CompletedLabel string `json:"completedLabel"`
	AutoCreate     bool   `json:"autoCreate"`
	AutoClaim      bool   `json:"autoClaim"`
}

type BranchConfig struct {
	Pattern      string   `json:"pattern"`
	AllowedTypes []string `json:"allowedTypes"`
}

type WorktreeConfig struct {
	Root               string `json:"root"`
	RefuseDirtyRemoval bool   `json:"refuseDirtyRemoval"`
}

type ValidationConfig struct {
	Profiles            map[string][]string `json:"profiles"`
	RequireClean        bool                `json:"requireClean"`
	RequireExpectedHead bool                `json:"requireExpectedHead"`
}

type ProtectedConfig struct {
	Branches        []string `json:"branches"`
	Paths           []string `json:"paths"`
	RejectForcePush bool     `json:"rejectForcePush"`
}

type NotificationConfig struct {
	Console    bool     `json:"console"`
	WebhookURL string   `json:"webhookUrl"`
	Command    []string `json:"command"`
}

func Default() Config {
	return Config{
		SchemaVersion: 1,
		DefaultBranch: "main",
		Issue:         IssueConfig{ReadyLabel: "agent:ready", ClaimedLabel: "agent:claimed", CompletedLabel: "agent:completed", AutoCreate: true, AutoClaim: false},
		Branches:      BranchConfig{Pattern: "agent/{executor}/{type}/{issue}-{slug}", AllowedTypes: []string{"feat", "fix", "docs", "refactor", "test", "chore", "perf", "ci", "build"}},
		Worktrees:     WorktreeConfig{Root: "../.gia-worktrees", RefuseDirtyRemoval: true},
		Validation: ValidationConfig{Profiles: map[string][]string{
			"smoke":   {"git diff --check", "go test ./..."},
			"full":    {"git diff --check", "go test ./...", "go vet ./..."},
			"release": {"git diff --check", "go test ./...", "go vet ./..."},
		}, RequireClean: true, RequireExpectedHead: true},
		Protected:     ProtectedConfig{Branches: []string{"main", "master", "release/*"}, Paths: []string{".github/workflows/**", ".gitmodules", "**/go.mod", "**/package-lock.json"}, RejectForcePush: true},
		Notifications: NotificationConfig{Console: true},
	}
}

func LoadFromRepo(repo string) (Config, error) {
	path := filepath.Join(repo, ".gia", "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w; run 'gia init --repo %s'", path, err, repo)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
