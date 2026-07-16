package issue

import (
	"github.com/example/git-isolated-agent-kit/internal/config"
	"strings"
	"testing"
)

func TestFromDirection(t *testing.T) {
	s := FromDirection("优化 release workflow 权限和安全", "auto", "web-agent", config.Default())
	if s.Risk != "high" {
		t.Fatalf("risk=%s", s.Risk)
	}
	if !strings.Contains(s.Body, "单分支单写者") {
		t.Fatal("missing safety contract")
	}
}
