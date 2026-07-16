package validate

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/example/git-isolated-agent-kit/internal/config"
	"github.com/example/git-isolated-agent-kit/internal/gitx"
)

type Step struct {
	Command    string `json:"command"`
	OK         bool   `json:"ok"`
	DurationMS int64  `json:"durationMs"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}
type Report struct {
	OK           bool   `json:"ok"`
	Profile      string `json:"profile"`
	Head         string `json:"head"`
	ExpectedHead string `json:"expectedHead,omitempty"`
	CleanBefore  bool   `json:"cleanBefore"`
	CleanAfter   bool   `json:"cleanAfter"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Steps        []Step `json:"steps"`
}

func Run(ctx context.Context, repo, profile, expected string, cfg config.Config) (Report, error) {
	cmds, ok := cfg.Validation.Profiles[profile]
	if !ok {
		return Report{}, fmt.Errorf("unknown validation profile %q", profile)
	}
	head, err := gitx.Head(ctx, repo)
	if err != nil {
		return Report{}, err
	}
	if cfg.Validation.RequireExpectedHead && expected == "" {
		return Report{}, fmt.Errorf("profile requires --expected-head; current HEAD is %s", head)
	}
	if expected != "" && head != expected {
		return Report{}, fmt.Errorf("HEAD mismatch: expected %s, got %s", expected, head)
	}
	cleanBefore, err := gitx.IsClean(ctx, repo)
	if err != nil {
		return Report{}, err
	}
	if cfg.Validation.RequireClean && !cleanBefore {
		return Report{}, fmt.Errorf("repository is dirty before validation")
	}
	report := Report{OK: true, Profile: profile, Head: head, ExpectedHead: expected, CleanBefore: cleanBefore, OS: runtime.GOOS, Arch: runtime.GOARCH}
	for _, c := range cmds {
		start := time.Now()
		out, e := shell(ctx, repo, c)
		s := Step{Command: c, OK: e == nil, DurationMS: time.Since(start).Milliseconds(), Output: trim(out, 4000)}
		if e != nil {
			s.Error = e.Error()
			report.OK = false
		}
		report.Steps = append(report.Steps, s)
		if e != nil {
			break
		}
	}
	report.CleanAfter, _ = gitx.IsClean(ctx, repo)
	if cfg.Validation.RequireClean && !report.CleanAfter {
		report.OK = false
		return report, fmt.Errorf("validation changed the worktree")
	}
	if !report.OK {
		return report, fmt.Errorf("validation failed")
	}
	return report, nil
}

func shell(ctx context.Context, dir, command string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/d", "/s", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-lc", command)
	}
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return out.String() + stderr.String(), fmt.Errorf("%w", err)
	}
	return out.String() + stderr.String(), nil
}
func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "\n...truncated"
	}
	return s
}
