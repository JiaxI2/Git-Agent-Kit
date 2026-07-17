package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

const (
	IdentityModeSharedUser = "shared-user"
	IdentityModeGitHubApp  = "github-app"
	IdentityModeTeam       = "team"

	ApprovalModeOwnerMerge     = "owner-merge"
	ApprovalModeRequiredReview = "required-review"
)

type PermissionConfig struct {
	IdentityMode       string   `json:"identityMode" yaml:"identityMode"`
	ApprovalMode       string   `json:"approvalMode" yaml:"approvalMode"`
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

func (c Config) ExecutorAllowed(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, allowed := range c.Permissions.AllowedExecutors {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || strings.EqualFold(allowed, name) {
			return true
		}
	}
	return false
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
	found, err := ExistingPaths(repo)
	if err != nil {
		return Config{}, err
	}
	if len(found) == 0 {
		candidates := configCandidates(repo)
		return Config{}, fmt.Errorf("no GIA config found; expected exactly one of %s; run 'gia init --repo %s'", strings.Join(candidates, ", "), repo)
	}
	if len(found) != 1 {
		return Config{}, fmt.Errorf("multiple GIA configs found (%s); keep exactly one of config.json, config.yaml, or config.yml", strings.Join(found, ", "))
	}
	return loadPath(found[0])
}

func Load(repo, explicitPath string) (Config, error) {
	if strings.TrimSpace(explicitPath) != "" {
		return LoadExplicit(repo, explicitPath)
	}
	return LoadFromRepo(repo)
}

func LoadExplicit(repo, path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, fmt.Errorf("explicit config path is empty")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, path)
	}
	return loadPath(filepath.Clean(path))
}

func ExistingPaths(repo string) ([]string, error) {
	var found []string
	for _, candidate := range configCandidates(repo) {
		_, err := os.Stat(candidate)
		switch {
		case err == nil:
			found = append(found, candidate)
		case os.IsNotExist(err):
			continue
		default:
			return nil, fmt.Errorf("inspect %s: %w", candidate, err)
		}
	}
	return found, nil
}

func PathForFormat(repo, format string) (string, error) {
	normalized, err := NormalizeFormat(format)
	if err != nil {
		return "", err
	}
	return filepath.Join(repo, ".gia", "config."+normalized), nil
}

func NormalizeFormat(format string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(format, ".")))
	switch normalized {
	case "json", "yaml", "yml":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported config format %q; use json, yaml, or yml", format)
	}
}

func configCandidates(repo string) []string {
	configDir := filepath.Join(repo, ".gia")
	return []string{
		filepath.Join(configDir, "config.json"),
		filepath.Join(configDir, "config.yaml"),
		filepath.Join(configDir, "config.yml"),
	}
}

func loadPath(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	switch filepath.Ext(path) {
	case ".json":
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		err = decodeOne(decoder.Decode, &cfg)
	case ".yaml", ".yml":
		decoder := yaml.NewDecoder(bytes.NewReader(b))
		decoder.KnownFields(true)
		err = decodeOne(decoder.Decode, &cfg)
	default:
		err = fmt.Errorf("unsupported config extension %q", filepath.Ext(path))
	}
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	applyPermissionDefaults(&cfg.Permissions)
	if err := validatePermissionModes(cfg.Permissions); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func decodeOne(decode func(interface{}) error, target interface{}) error {
	if err := decode(target); err != nil {
		return err
	}
	var extra interface{}
	switch err := decode(&extra); {
	case err == io.EOF:
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("multiple configuration documents are not allowed")
	}
}

func defaultPermissions() PermissionConfig {
	return PermissionConfig{
		IdentityMode:       IdentityModeSharedUser,
		ApprovalMode:       ApprovalModeOwnerMerge,
		AllowedExecutors:   []string{"*"},
		ApprovalPrincipals: []string{"user"},
		MergePrincipals:    []string{"user"},
		ReleasePrincipals:  []string{"user"},
	}
}

func applyPermissionDefaults(permissions *PermissionConfig) {
	defaults := defaultPermissions()
	if strings.TrimSpace(permissions.IdentityMode) == "" {
		permissions.IdentityMode = defaults.IdentityMode
	}
	if strings.TrimSpace(permissions.ApprovalMode) == "" {
		permissions.ApprovalMode = defaults.ApprovalMode
	}
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

func validatePermissionModes(permissions PermissionConfig) error {
	switch permissions.IdentityMode {
	case IdentityModeSharedUser, IdentityModeGitHubApp, IdentityModeTeam:
	default:
		return fmt.Errorf("permissions.identityMode %q is invalid; use %s, %s, or %s", permissions.IdentityMode, IdentityModeSharedUser, IdentityModeGitHubApp, IdentityModeTeam)
	}
	switch permissions.ApprovalMode {
	case ApprovalModeOwnerMerge, ApprovalModeRequiredReview:
	default:
		return fmt.Errorf("permissions.approvalMode %q is invalid; use %s or %s", permissions.ApprovalMode, ApprovalModeOwnerMerge, ApprovalModeRequiredReview)
	}
	return nil
}

func Save(path string, cfg Config) error {
	var (
		b   []byte
		err error
	)
	switch filepath.Ext(path) {
	case ".json":
		b, err = json.MarshalIndent(cfg, "", "  ")
	case ".yaml", ".yml":
		b, err = yaml.Marshal(cfg)
	default:
		return fmt.Errorf("unsupported config extension %q", filepath.Ext(path))
	}
	if err != nil {
		return err
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return os.WriteFile(path, b, 0o644)
}
