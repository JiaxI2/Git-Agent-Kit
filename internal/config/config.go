package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	SchemaVersion int                `json:"schemaVersion" yaml:"schemaVersion"`
	DefaultBranch string             `json:"defaultBranch" yaml:"defaultBranch"`
	Issue         IssueConfig        `json:"issue" yaml:"issue"`
	Branches      BranchConfig       `json:"branches" yaml:"branches"`
	Worktrees     WorktreeConfig     `json:"worktrees" yaml:"worktrees"`
	Validation    ValidationConfig   `json:"validation" yaml:"validation"`
	Protected     ProtectedConfig    `json:"protected" yaml:"protected"`
	Permissions   PermissionConfig   `json:"permissions" yaml:"permissions"`
	Notifications NotificationConfig `json:"notifications" yaml:"notifications"`
}

type IssueConfig struct {
	ReadyLabel     string `json:"readyLabel" yaml:"readyLabel"`
	ClaimedLabel   string `json:"claimedLabel" yaml:"claimedLabel"`
	CompletedLabel string `json:"completedLabel" yaml:"completedLabel"`
	AutoCreate     bool   `json:"autoCreate" yaml:"autoCreate"`
	AutoClaim      bool   `json:"autoClaim" yaml:"autoClaim"`
}

type BranchConfig struct {
	Pattern      string   `json:"pattern" yaml:"pattern"`
	AllowedTypes []string `json:"allowedTypes" yaml:"allowedTypes"`
}

type WorktreeConfig struct {
	Root               string `json:"root" yaml:"root"`
	RefuseDirtyRemoval bool   `json:"refuseDirtyRemoval" yaml:"refuseDirtyRemoval"`
}

type ValidationConfig struct {
	Profiles            map[string][]string `json:"profiles" yaml:"profiles"`
	Remote              string              `json:"remote" yaml:"remote"`
	RequireClean        bool                `json:"requireClean" yaml:"requireClean"`
	RequireExpectedHead bool                `json:"requireExpectedHead" yaml:"requireExpectedHead"`
}

type ProtectedConfig struct {
	Branches        []string `json:"branches" yaml:"branches"`
	Paths           []string `json:"paths" yaml:"paths"`
	RejectForcePush bool     `json:"rejectForcePush" yaml:"rejectForcePush"`
}

type PermissionConfig struct {
	AllowedExecutors   []string `json:"allowedExecutors" yaml:"allowedExecutors"`
	ApprovalPrincipals []string `json:"approvalPrincipals" yaml:"approvalPrincipals"`
	MergePrincipals    []string `json:"mergePrincipals" yaml:"mergePrincipals"`
	ReleasePrincipals  []string `json:"releasePrincipals" yaml:"releasePrincipals"`
}

type NotificationConfig struct {
	Console    bool     `json:"console" yaml:"console"`
	WebhookURL string   `json:"webhookUrl" yaml:"webhookUrl"`
	Command    []string `json:"command" yaml:"command"`
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
		}, Remote: "origin", RequireClean: true, RequireExpectedHead: true},
		Protected:     ProtectedConfig{Branches: []string{"main", "master", "release/*"}, Paths: []string{".github/workflows/**", ".gitmodules", "**/go.mod", "**/package-lock.json"}, RejectForcePush: true},
		Permissions:   defaultPermissions(),
		Notifications: NotificationConfig{Console: true},
	}
}

func (c Config) ValidationRemote() string {
	remote := strings.TrimSpace(c.Validation.Remote)
	if remote == "" {
		return "origin"
	}
	return remote
}

func MatchAny(patterns []string, value string) bool {
	value = normalizePolicyPath(value)
	for _, pattern := range patterns {
		if matchPolicyPattern(pattern, value) {
			return true
		}
	}
	return false
}

func matchPolicyPattern(pattern, value string) bool {
	pattern = normalizePolicyPath(pattern)
	if pattern == "" {
		return false
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for i := 0; i < len(pattern); {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i += 2
				if i < len(pattern) && pattern[i] == '/' {
					expression.WriteString("(?:.*/)?")
					i++
				} else {
					expression.WriteString(".*")
				}
				continue
			}
			expression.WriteString("[^/]*")
		case '?':
			expression.WriteString("[^/]")
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
		i++
	}
	expression.WriteByte('$')
	matched, err := regexp.MatchString(expression.String(), value)
	return err == nil && matched
}

func normalizePolicyPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	return strings.Trim(value, "/")
}

func LoadFromRepo(repo string) (Config, error) {
	configDir := filepath.Join(repo, ".gia")
	candidates := []string{
		filepath.Join(configDir, "config.json"),
		filepath.Join(configDir, "config.yaml"),
		filepath.Join(configDir, "config.yml"),
	}
	var found []string
	for _, candidate := range candidates {
		_, err := os.Stat(candidate)
		switch {
		case err == nil:
			found = append(found, candidate)
		case os.IsNotExist(err):
			continue
		default:
			return Config{}, fmt.Errorf("inspect %s: %w", candidate, err)
		}
	}
	if len(found) == 0 {
		return Config{}, fmt.Errorf("no GIA config found; expected exactly one of %s; run 'gia init --repo %s'", strings.Join(candidates, ", "), repo)
	}
	if len(found) != 1 {
		return Config{}, fmt.Errorf("multiple GIA configs found (%s); keep exactly one of config.json, config.yaml, or config.yml", strings.Join(found, ", "))
	}
	path := found[0]
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	switch filepath.Ext(path) {
	case ".json":
		err = json.Unmarshal(b, &cfg)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(b, &cfg)
	default:
		err = fmt.Errorf("unsupported config extension %q", filepath.Ext(path))
	}
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	applyPermissionDefaults(&cfg.Permissions)
	return cfg, nil
}

func defaultPermissions() PermissionConfig {
	return PermissionConfig{
		AllowedExecutors:   []string{"*"},
		ApprovalPrincipals: []string{"user"},
		MergePrincipals:    []string{"user"},
		ReleasePrincipals:  []string{"user"},
	}
}

func applyPermissionDefaults(permissions *PermissionConfig) {
	defaults := defaultPermissions()
	if permissions.AllowedExecutors == nil {
		permissions.AllowedExecutors = defaults.AllowedExecutors
	}
	if permissions.ApprovalPrincipals == nil {
		permissions.ApprovalPrincipals = defaults.ApprovalPrincipals
	}
	if permissions.MergePrincipals == nil {
		permissions.MergePrincipals = defaults.MergePrincipals
	}
	if permissions.ReleasePrincipals == nil {
		permissions.ReleasePrincipals = defaults.ReleasePrincipals
	}
}

func Save(path string, cfg Config) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}
