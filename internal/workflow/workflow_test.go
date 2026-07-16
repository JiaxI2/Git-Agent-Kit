package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/example/git-isolated-agent-kit/internal/config"
)

func TestInitialize(t *testing.T) {
	d := t.TempDir()
	if err := Initialize(d, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, ".gia", "config.json")); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(d, false); err == nil {
		t.Fatal("expected overwrite refusal")
	}
}
func TestDefaultConfigHasProfiles(t *testing.T) {
	c := config.Default()
	for _, p := range []string{"smoke", "full", "release"} {
		if len(c.Validation.Profiles[p]) == 0 {
			t.Fatalf("missing %s", p)
		}
	}
}
func TestSanitize(t *testing.T) {
	if got := sanitize("Web GPT / Test"); got != "web-gpt-test" {
		t.Fatalf("got %q", got)
	}
}
