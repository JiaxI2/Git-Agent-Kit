package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidationRemoteDefaultsToOrigin(t *testing.T) {
	cfg := Default()
	cfg.Validation.Remote = ""
	if got := cfg.ValidationRemote(); got != "origin" {
		t.Fatalf("ValidationRemote() = %q, want origin", got)
	}
}

func TestMatchAnySupportsProtectedPolicyGlobs(t *testing.T) {
	patterns := []string{".github/workflows/**", ".gitmodules", "**/go.mod", "release/*"}
	tests := []struct {
		value string
		want  bool
	}{
		{value: ".github/workflows/ci.yml", want: true},
		{value: ".github/workflows/nested/release.yml", want: true},
		{value: ".gitmodules", want: true},
		{value: "go.mod", want: true},
		{value: "tools/agent/go.mod", want: true},
		{value: "release/1.x", want: true},
		{value: "docs/guide.md", want: false},
	}
	for _, tt := range tests {
		if got := MatchAny(patterns, tt.value); got != tt.want {
			t.Errorf("MatchAny(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestDefaultPermissionsAreUserControlled(t *testing.T) {
	got := Default().Permissions
	if got.IdentityMode != IdentityModeSharedUser || got.ApprovalMode != ApprovalModeOwnerMerge {
		t.Fatalf("permission modes = %s/%s", got.IdentityMode, got.ApprovalMode)
	}
	if !reflect.DeepEqual(got.AllowedExecutors, []string{"*"}) {
		t.Fatalf("AllowedExecutors = %v", got.AllowedExecutors)
	}
	for name, principals := range map[string][]string{
		"approval": got.ApprovalPrincipals,
		"merge":    got.MergePrincipals,
		"release":  got.ReleasePrincipals,
	} {
		if !reflect.DeepEqual(principals, []string{"user"}) {
			t.Errorf("%s principals = %v, want [user]", name, principals)
		}
	}
}

func TestExecutorAllowedSupportsWildcardAndDenyAll(t *testing.T) {
	cfg := Default()
	if !cfg.ExecutorAllowed("web-agent") {
		t.Fatal("wildcard executor policy rejected web-agent")
	}
	cfg.Permissions.AllowedExecutors = []string{"Local-Agent"}
	if !cfg.ExecutorAllowed("local-agent") {
		t.Fatal("executor comparison should be case-insensitive")
	}
	if cfg.ExecutorAllowed("web-agent") {
		t.Fatal("unlisted executor was accepted")
	}
	cfg.Permissions.AllowedExecutors = []string{}
	if cfg.ExecutorAllowed("local-agent") || cfg.ExecutorAllowed("") {
		t.Fatal("deny-all or empty executor was accepted")
	}
}

func TestLoadFromRepoSupportsExactlyOneJSONOrYAMLConfig(t *testing.T) {
	for _, extension := range []string{".json", ".yaml", ".yml"} {
		t.Run(extension, func(t *testing.T) {
			repo := t.TempDir()
			dir := filepath.Join(repo, ".gia")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "config"+extension)
			var content string
			if extension == ".json" {
				content = `{
  "schemaVersion": 1,
  "defaultBranch": "trunk",
  "validation": {"remote": "review"},
  "permissions": {
    "identityMode": "github-app",
    "approvalMode": "required-review",
    "allowedExecutors": ["agent-a"],
    "approvalPrincipals": ["reviewer"],
    "mergePrincipals": ["maintainer"],
    "releasePrincipals": ["release-manager"]
  }
}`
			} else {
				content = `schemaVersion: 1
defaultBranch: trunk
validation:
  remote: review
permissions:
  identityMode: github-app
  approvalMode: required-review
  allowedExecutors: [agent-a]
  approvalPrincipals: [reviewer]
  mergePrincipals: [maintainer]
  releasePrincipals: [release-manager]
`
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadFromRepo(repo)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DefaultBranch != "trunk" || cfg.ValidationRemote() != "review" {
				t.Fatalf("loaded config = %+v", cfg)
			}
			if !reflect.DeepEqual(cfg.Permissions.AllowedExecutors, []string{"agent-a"}) ||
				cfg.Permissions.IdentityMode != IdentityModeGitHubApp ||
				cfg.Permissions.ApprovalMode != ApprovalModeRequiredReview ||
				!reflect.DeepEqual(cfg.Permissions.ApprovalPrincipals, []string{"reviewer"}) ||
				!reflect.DeepEqual(cfg.Permissions.MergePrincipals, []string{"maintainer"}) ||
				!reflect.DeepEqual(cfg.Permissions.ReleasePrincipals, []string{"release-manager"}) {
				t.Fatalf("loaded permissions = %+v", cfg.Permissions)
			}
		})
	}
}

func TestLoadExplicitOverridesRepositoryDiscovery(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "config.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	explicit := filepath.Join(repo, "review-config.yml")
	if err := os.WriteFile(explicit, []byte("defaultBranch: review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(repo, "review-config.yml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultBranch != "review" {
		t.Fatalf("explicit default branch=%q", cfg.DefaultBranch)
	}
}

func TestLoadFromRepoFailsClosedForMissingOrMultipleConfigs(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		repo := t.TempDir()
		if _, err := LoadFromRepo(repo); err == nil || !strings.Contains(err.Error(), "expected exactly one") {
			t.Fatalf("LoadFromRepo() error = %v", err)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		repo := t.TempDir()
		dir := filepath.Join(repo, ".gia")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"config.json", "config.yaml"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := LoadFromRepo(repo); err == nil || !strings.Contains(err.Error(), "multiple GIA configs") {
			t.Fatalf("LoadFromRepo() error = %v", err)
		}
	})
}

func TestLoadRejectsUnknownFieldsAndMultipleDocuments(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{name: "json unknown", file: "config.json", content: `{"schemaVersion":1,"defualtBranch":"main"}`, want: "unknown field"},
		{name: "yaml unknown", file: "config.yaml", content: "schemaVersion: 1\ndefualtBranch: main\n", want: "field defualtBranch not found"},
		{name: "yml nested unknown", file: "config.yml", content: "validation:\n  requireCleam: true\n", want: "field requireCleam not found"},
		{name: "json multiple", file: "config.json", content: "{}\n{}\n", want: "multiple configuration documents"},
		{name: "yaml multiple", file: "config.yaml", content: "{}\n---\n{}\n", want: "multiple configuration documents"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			dir := filepath.Join(repo, ".gia")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadFromRepo(repo)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadFromRepo() error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsInvalidPermissionModes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		want    string
	}{
		{name: "identity", content: "permissions:\n  identityMode: robot-user\n", want: "permissions.identityMode"},
		{name: "approval", content: "permissions:\n  approvalMode: self-approve\n", want: "permissions.approvalMode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			dir := filepath.Join(repo, ".gia")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadFromRepo(repo)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadFromRepo() error=%v, want %q", err, tt.want)
			}
		})
	}
}

func TestSaveUsesRequestedJSONOrYAMLFormat(t *testing.T) {
	for _, extension := range []string{".json", ".yaml", ".yml"} {
		t.Run(extension, func(t *testing.T) {
			repo := t.TempDir()
			dir := filepath.Join(repo, ".gia")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "config"+extension)
			want := Default()
			want.DefaultBranch = "review"
			if err := Save(path, want); err != nil {
				t.Fatal(err)
			}
			got, err := LoadFromRepo(repo)
			if err != nil {
				t.Fatal(err)
			}
			if got.DefaultBranch != "review" {
				t.Fatalf("loaded default branch=%q", got.DefaultBranch)
			}
		})
	}
}

func TestPathForFormatRejectsUnsupportedFormat(t *testing.T) {
	path, err := PathForFormat("repo", "YML")
	if err != nil || path != filepath.Join("repo", ".gia", "config.yml") {
		t.Fatalf("PathForFormat() path=%q err=%v", path, err)
	}
	if _, err := PathForFormat("repo", "toml"); err == nil {
		t.Fatal("unsupported format was accepted")
	}
}

func TestLoadFromRepoAppliesPermissionDefaultsOnlyWhenOmitted(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `permissions:
  allowedExecutors: []
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Permissions.AllowedExecutors == nil || len(cfg.Permissions.AllowedExecutors) != 0 {
		t.Fatalf("explicit executor deny-all was not preserved: %v", cfg.Permissions.AllowedExecutors)
	}
	if !reflect.DeepEqual(cfg.Permissions.ApprovalPrincipals, []string{"user"}) {
		t.Fatalf("omitted approval defaults = %v", cfg.Permissions.ApprovalPrincipals)
	}
	if cfg.Permissions.IdentityMode != IdentityModeSharedUser || cfg.Permissions.ApprovalMode != ApprovalModeOwnerMerge {
		t.Fatalf("omitted permission mode defaults = %s/%s", cfg.Permissions.IdentityMode, cfg.Permissions.ApprovalMode)
	}
}

func TestConfigFieldsHaveJSONAndYAMLTags(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(Config{}),
		reflect.TypeOf(IssueConfig{}),
		reflect.TypeOf(BranchConfig{}),
		reflect.TypeOf(WorktreeConfig{}),
		reflect.TypeOf(ValidationConfig{}),
		reflect.TypeOf(ProtectedConfig{}),
		reflect.TypeOf(PermissionConfig{}),
		reflect.TypeOf(NotificationConfig{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Tag.Get("json") == "" {
				t.Errorf("%s.%s missing json tag", typ.Name(), field.Name)
			}
			if field.Tag.Get("yaml") == "" {
				t.Errorf("%s.%s missing yaml tag", typ.Name(), field.Name)
			}
		}
	}
}
