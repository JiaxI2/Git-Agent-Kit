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
				!reflect.DeepEqual(cfg.Permissions.ApprovalPrincipals, []string{"reviewer"}) ||
				!reflect.DeepEqual(cfg.Permissions.MergePrincipals, []string{"maintainer"}) ||
				!reflect.DeepEqual(cfg.Permissions.ReleasePrincipals, []string{"release-manager"}) {
				t.Fatalf("loaded permissions = %+v", cfg.Permissions)
			}
		})
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
