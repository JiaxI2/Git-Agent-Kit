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
	Remote       string `json:"remote"`
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
	validatedHead := expected
	if validatedHead == "" {
		validatedHead = head
	}
	canonicalExpected, err := gitx.Run(ctx, repo, "rev-parse", "--verify", validatedHead+"^{commit}")
	if err != nil {
		return Report{}, fmt.Errorf("resolve expected HEAD %q: %w", validatedHead, err)
	}
	if head != canonicalExpected {
		return Report{}, fmt.Errorf("HEAD mismatch: expected %s, got %s", canonicalExpected, head)
	}
	remote := cfg.ValidationRemote()
	if _, err := gitx.Run(ctx, repo, "fetch", "--prune", remote); err != nil {
		return Report{}, fmt.Errorf("fetch validation remote %q: %w", remote, err)
	}
	remoteRefs, err := gitx.Run(ctx, repo, "for-each-ref", "--format=%(objectname) %(refname)", "refs/remotes/"+remote+"/")
	if err != nil {
		return Report{}, fmt.Errorf("inspect validation remote refs: %w", err)
	}
	if !remoteRefAtHead(remoteRefs, canonicalExpected) {
		return Report{}, fmt.Errorf("expected HEAD %s is not the current tip of any fetched %q remote ref; push the commit or refresh the validation worktree", canonicalExpected, remote)
	}
	branch, err := gitx.Branch(ctx, repo)
	if err != nil {
		return Report{}, err
	}
	if strings.TrimSpace(branch) == "" {
		return Report{}, fmt.Errorf("validation refuses detached HEAD %s", head)
	}
	if config.MatchAny(cfg.Protected.Branches, branch) {
		return Report{}, fmt.Errorf("validation branch %q matches protected.branches", branch)
	}
	protected, err := protectedChanges(ctx, repo, remote+"/"+cfg.DefaultBranch, canonicalExpected, cfg.Protected.Paths)
	if err != nil {
		return Report{}, err
	}
	if len(protected) != 0 {
		return Report{}, fmt.Errorf("validation requires manual review for protected paths: %s", strings.Join(protected, ", "))
	}
	cleanBefore, err := gitx.IsClean(ctx, repo)
	if err != nil {
		return Report{}, err
	}
	if cfg.Validation.RequireClean && !cleanBefore {
		return Report{}, fmt.Errorf("repository is dirty before validation")
	}
	report := Report{OK: true, Profile: profile, Head: head, ExpectedHead: expected, Remote: remote, CleanBefore: cleanBefore, OS: runtime.GOOS, Arch: runtime.GOARCH}
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

func remoteRefAtHead(refs, expected string) bool {
	for _, line := range strings.Split(refs, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == expected {
			return true
		}
	}
	return false
}

func protectedChanges(ctx context.Context, repo, base, head string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	if _, err := gitx.Run(ctx, repo, "rev-parse", "--verify", base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("resolve protected-path base %q: %w", base, err)
	}
	out, err := gitx.Run(ctx, repo, "diff", "--name-only", "-z", base+"..."+head)
	if err != nil {
		return nil, fmt.Errorf("inspect changed paths against %q: %w", base, err)
	}
	var protected []string
	for _, name := range strings.Split(out, "\x00") {
		name = strings.TrimSpace(name)
		if name != "" && config.MatchAny(patterns, name) {
			protected = append(protected, name)
		}
	}
	return protected, nil
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
