package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

func TestInitializeWithFormatCreatesOneConfigAndFailsClosedOnAmbiguity(t *testing.T) {
	for _, format := range []string{"json", "yaml", "yml"} {
		t.Run(format, func(t *testing.T) {
			repo := t.TempDir()
			path, err := InitializeWithFormat(repo, format, false)
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(repo, ".gia", "config."+format)
			if path != want {
				t.Fatalf("path=%q want=%q", path, want)
			}
			found, err := config.ExistingPaths(repo)
			if err != nil {
				t.Fatal(err)
			}
			if len(found) != 1 || found[0] != want {
				t.Fatalf("found configs=%v", found)
			}
			if _, err := config.LoadFromRepo(repo); err != nil {
				t.Fatalf("load initialized config: %v", err)
			}
			if _, err := InitializeWithFormat(repo, format, true); err != nil {
				t.Fatalf("force same format: %v", err)
			}
			other := "yaml"
			if format == "yaml" {
				other = "json"
			}
			if _, err := InitializeWithFormat(repo, other, true); err == nil || !strings.Contains(err.Error(), "will not delete or choose") {
				t.Fatalf("cross-format force error=%v", err)
			}
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("existing config was removed: %v", err)
			}
		})
	}
}

func TestInitializeWithFormatRejectsMultipleExistingConfigsEvenWithForce(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "config.yml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InitializeWithFormat(repo, "json", true); err == nil || !strings.Contains(err.Error(), "will not delete or choose") {
		t.Fatalf("ambiguous force error=%v", err)
	}
	for _, name := range []string{"config.json", "config.yml"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was removed: %v", name, err)
		}
	}
}

func TestDoctorIncludesPermissionBoundaryGuidance(t *testing.T) {
	report := Doctor(context.Background(), t.TempDir())
	for _, check := range report.Checks {
		if check.Name == "permission-boundary" {
			if !check.OK || !strings.Contains(check.Detail, "GitHub App") || !strings.Contains(check.Detail, "ruleset") {
				t.Fatalf("permission boundary check=%+v", check)
			}
			return
		}
	}
	t.Fatal("permission-boundary guidance missing")
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

func TestCreatedRemoteRefRequiresNewBranchPorcelainStatus(t *testing.T) {
	ref := "refs/heads/gia/claims/42"
	if !createdRemoteRef("*\tHEAD:"+ref+"\t[new branch]\nDone", ref) {
		t.Fatal("new branch porcelain status was not accepted")
	}
	for _, output := range []string{
		"=\tHEAD:" + ref + "\t[up to date]\nDone",
		" \tHEAD:" + ref + "\t[fast-forward]\nDone",
		"",
	} {
		if createdRemoteRef(output, ref) {
			t.Fatalf("non-creation status was accepted: %q", output)
		}
	}
}

func TestClaimConcurrentSingleWriterAndRepeatedClaimFails(t *testing.T) {
	remote, firstRepo := newClaimRemote(t)
	secondRepo := cloneClaimRepo(t, remote, "second")
	thirdRepo := cloneClaimRepo(t, remote, "third")
	fake := newFakeClaimGH(true)
	withClaimRunner(t, fake.run)
	cfg := config.Default()

	type result struct {
		claim ClaimResult
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, request := range []struct {
		repo     string
		executor string
	}{
		{repo: firstRepo, executor: "web-agent"},
		{repo: secondRepo, executor: "local-agent"},
	} {
		request := request
		go func() {
			<-start
			got, err := Claim(context.Background(), request.repo, 42, request.executor, cfg)
			results <- result{claim: got, err: err}
		}()
	}
	close(start)

	var successes, failures int
	for range 2 {
		got := <-results
		if got.err == nil {
			successes++
			if got.claim.State != "CLAIMED" {
				t.Errorf("successful claim state = %q", got.claim.State)
			}
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("concurrent claims: successes=%d failures=%d, want one each", successes, failures)
	}

	refs := gitOutput(t, remote, "--git-dir", remote, "for-each-ref", "--format=%(refname)", "refs/heads/gia/claims/42", "refs/heads/agent/")
	lines := nonemptyLines(refs)
	if len(lines) != 2 {
		t.Fatalf("remote claim refs = %q, want one lease and one task branch", lines)
	}
	if !containsLine(lines, "refs/heads/gia/claims/42") {
		t.Fatalf("missing claim lease in %q", lines)
	}

	fake.resetToReady()
	if _, err := Claim(context.Background(), thirdRepo, 42, "review-agent", cfg); err == nil || !strings.Contains(err.Error(), "ownership was not acquired") {
		t.Fatalf("repeated claim error = %v, want remote lease conflict", err)
	}
}

func TestClaimMetadataFailuresRollbackArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		failEdit    bool
		failComment bool
	}{
		{name: "issue edit", failEdit: true},
		{name: "issue comment", failComment: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote, repo := newClaimRemote(t)
			fake := newFakeClaimGH(false)
			fake.failEdit = tt.failEdit
			fake.failComment = tt.failComment
			withClaimRunner(t, fake.run)

			if got, err := Claim(context.Background(), repo, 7, "web-agent", config.Default()); err == nil {
				t.Fatalf("Claim() = %+v, want metadata error", got)
			} else if !strings.Contains(err.Error(), "failed before CLAIMED") {
				t.Fatalf("Claim() error = %v, want fail-closed context", err)
			}

			refs := gitOutput(t, remote, "--git-dir", remote, "for-each-ref", "--format=%(refname)", "refs/heads/gia/claims/7", "refs/heads/agent/")
			if strings.TrimSpace(refs) != "" {
				t.Fatalf("claim refs survived rollback: %q", refs)
			}
			if branch := gitOutput(t, repo, "branch", "--list", "agent/web-agent/feat/7-issue-7"); strings.TrimSpace(branch) != "" {
				t.Fatalf("local task branch survived rollback: %q", branch)
			}
			if !fake.hasLabel(config.Default().Issue.ReadyLabel) || fake.hasLabel(config.Default().Issue.ClaimedLabel) || fake.hasLabel("executor:web-agent") {
				t.Fatalf("labels not restored after failure: %v", fake.labelNames())
			}
		})
	}
}

func TestClaimRejectsUnsafePolicyAndProtectedBranch(t *testing.T) {
	cfg := config.Default()
	cfg.Protected.RejectForcePush = false
	if _, err := Claim(context.Background(), t.TempDir(), 1, "web-agent", cfg); err == nil || !strings.Contains(err.Error(), "rejectForcePush=true") {
		t.Fatalf("unsafe policy error = %v", err)
	}

	cfg = config.Default()
	cfg.Permissions.AllowedExecutors = []string{"local-agent"}
	if _, err := Claim(context.Background(), t.TempDir(), 1, "web-agent", cfg); err == nil || !strings.Contains(err.Error(), "permissions.allowedExecutors") {
		t.Fatalf("executor permission error = %v", err)
	}

	_, repo := newClaimRemote(t)
	fake := newFakeClaimGH(false)
	withClaimRunner(t, fake.run)
	cfg = config.Default()
	cfg.Branches.Pattern = "main"
	if _, err := Claim(context.Background(), repo, 1, "web-agent", cfg); err == nil || !strings.Contains(err.Error(), "protected.branches") {
		t.Fatalf("protected branch error = %v", err)
	}
}

func TestClaimFailureNamesArtifactsWhenRecoveryIsIncomplete(t *testing.T) {
	artifacts := claimArtifacts{
		localBranch: "agent/web/feat/9-issue-9",
		leaseBranch: "gia/claims/9",
		taskBranch:  "agent/web/feat/9-issue-9",
	}
	err := claimFailure(errors.New("metadata failed"), errors.New("delete failed"), artifacts)
	for _, want := range []string{artifacts.localBranch, artifacts.leaseBranch, artifacts.taskBranch, "automatic recovery was incomplete"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("claimFailure() = %q, missing %q", err, want)
		}
	}
}

type fakeClaimGH struct {
	mu            sync.Mutex
	labels        map[string]bool
	failEdit      bool
	failComment   bool
	barrier       chan struct{}
	barrierTarget int
	barrierCount  int
}

func newFakeClaimGH(barrier bool) *fakeClaimGH {
	fake := &fakeClaimGH{labels: map[string]bool{config.Default().Issue.ReadyLabel: true}}
	if barrier {
		fake.barrier = make(chan struct{})
		fake.barrierTarget = 2
	}
	return fake
}

func (f *fakeClaimGH) run(_ context.Context, _ string, name string, args ...string) (string, error) {
	if name != "gh" {
		return "", fmt.Errorf("unexpected command %q", name)
	}
	if len(args) >= 2 && args[0] == "repo" && args[1] == "view" {
		return "owner/repo", nil
	}
	if len(args) >= 3 && args[0] == "issue" && args[1] == "view" {
		f.waitForConcurrentViews()
		f.mu.Lock()
		defer f.mu.Unlock()
		state := issueState{State: "OPEN"}
		for _, name := range f.labelNamesLocked() {
			state.Labels = append(state.Labels, struct {
				Name string `json:"name"`
			}{Name: name})
		}
		data, err := json.Marshal(state)
		return string(data), err
	}
	if len(args) >= 3 && args[0] == "issue" && args[1] == "edit" {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.applyLabelArguments(args)
		if f.failEdit {
			f.failEdit = false
			return "", fmt.Errorf("injected issue edit failure after partial label update")
		}
		return "", nil
	}
	if len(args) >= 3 && args[0] == "issue" && args[1] == "comment" {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failComment {
			return "", fmt.Errorf("injected issue comment failure")
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected gh arguments: %v", args)
}

func (f *fakeClaimGH) waitForConcurrentViews() {
	f.mu.Lock()
	if f.barrier == nil {
		f.mu.Unlock()
		return
	}
	f.barrierCount++
	barrier := f.barrier
	if f.barrierCount == f.barrierTarget {
		close(f.barrier)
		f.barrier = nil
	}
	f.mu.Unlock()
	<-barrier
}

func (f *fakeClaimGH) applyLabelArguments(args []string) {
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--add-label":
			f.labels[args[i+1]] = true
			i++
		case "--remove-label":
			delete(f.labels, args[i+1])
			i++
		}
	}
}

func (f *fakeClaimGH) hasLabel(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.labels[name]
}

func (f *fakeClaimGH) labelNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.labelNamesLocked()
}

func (f *fakeClaimGH) labelNamesLocked() []string {
	var names []string
	for name, present := range f.labels {
		if present {
			names = append(names, name)
		}
	}
	return names
}

func (f *fakeClaimGH) resetToReady() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.labels = map[string]bool{config.Default().Issue.ReadyLabel: true}
}

func withClaimRunner(t *testing.T, runner func(context.Context, string, string, ...string) (string, error)) {
	t.Helper()
	original := claimRun
	originalBody := claimRunWithBodyFile
	claimRun = runner
	claimRunWithBodyFile = func(ctx context.Context, repo, _ string, args ...string) (string, error) {
		return runner(ctx, repo, "gh", args...)
	}
	t.Cleanup(func() {
		claimRun = original
		claimRunWithBodyFile = originalBody
	})
}

func newClaimRemote(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	repo := filepath.Join(root, "first")
	gitOutput(t, root, "init", "--bare", remote)
	gitOutput(t, root, "init", "-b", "main", repo)
	configureTestGit(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("claim test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOutput(t, repo, "add", "README.md")
	gitOutput(t, repo, "commit", "-m", "initial")
	gitOutput(t, repo, "remote", "add", "origin", remote)
	gitOutput(t, repo, "push", "-u", "origin", "main")
	gitOutput(t, remote, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return remote, repo
}

func cloneClaimRepo(t *testing.T, remote, name string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), name)
	gitOutput(t, filepath.Dir(repo), "clone", remote, repo)
	configureTestGit(t, repo)
	return repo
}

func configureTestGit(t *testing.T, repo string) {
	t.Helper()
	gitOutput(t, repo, "config", "user.name", "GIA Test")
	gitOutput(t, repo, "config", "user.email", "gia@example.invalid")
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func nonemptyLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
