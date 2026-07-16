package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestHandoffPreservesMultilineComment(t *testing.T) {
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})

	runGitHub = func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Join(args[:2], " ") != "pr view" {
			return "", errors.New("unexpected GitHub call")
		}
		return "0123456789abcdef0123456789abcdef01234567", nil
	}
	var comment string
	runGitHubWithBodyFile = func(_ context.Context, _ string, body string, args ...string) (string, error) {
		if strings.Join(args[:2], " ") != "pr comment" {
			return "", errors.New("unexpected GitHub body call")
		}
		comment = body
		return "", nil
	}

	result, err := Handoff(context.Background(), t.TempDir(), 12, "local-agent", "LOCAL_VALIDATION", "line one\nline two")
	if err != nil {
		t.Fatal(err)
	}
	if result.Head != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("result=%+v", result)
	}
	for _, expected := range []string{
		"<!-- GIA:HANDOFF:START -->",
		"executor: local-agent",
		"state: LOCAL_VALIDATION",
		"note: line one line two",
		"<!-- GIA:HANDOFF:END -->",
	} {
		if !strings.Contains(comment, expected) {
			t.Fatalf("comment missing %q:\n%s", expected, comment)
		}
	}
}

func TestHandoffRejectsEmptyHead(t *testing.T) {
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})
	runGitHub = func(_ context.Context, _ string, _ ...string) (string, error) {
		return "", nil
	}
	runGitHubWithBodyFile = func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
		t.Fatal("comment must not be created without a PR head")
		return "", nil
	}

	if _, err := Handoff(context.Background(), t.TempDir(), 12, "local-agent", "LOCAL_VALIDATION", "note"); err == nil || !strings.Contains(err.Error(), "empty headRefOid") {
		t.Fatalf("err=%v", err)
	}
}

func TestRequestDraftPullRequestUsesDraftBodyFileAndPendingApproval(t *testing.T) {
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})

	var createdBody string
	runGitHub = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args[:2], " ") {
		case "repo view":
			return "owner/repo", nil
		case "pr view":
			record := draftPullRequestRecord{
				Number:      17,
				URL:         "https://github.com/owner/repo/pull/17",
				IsDraft:     true,
				BaseRefName: "main",
				HeadRefName: "agent/web/feat/17-task",
				Title:       "Prepare isolated change",
				Body:        createdBody,
			}
			data, err := json.Marshal(record)
			return string(data), err
		default:
			return "", errors.New("unexpected GitHub call")
		}
	}
	runGitHubWithBodyFile = func(_ context.Context, _ string, body string, args ...string) (string, error) {
		if strings.Join(args[:2], " ") != "pr create" {
			return "", errors.New("unexpected GitHub body call")
		}
		if !slices.Contains(args, "--draft") {
			t.Fatalf("--draft missing from args: %v", args)
		}
		for key, value := range map[string]string{
			"--repo":  "owner/repo",
			"--base":  "main",
			"--head":  "agent/web/feat/17-task",
			"--title": "Prepare isolated change",
		} {
			if !containsPair(args, key, value) {
				t.Fatalf("%s %q missing from args: %v", key, value, args)
			}
		}
		if slices.Contains(args, "--body") {
			t.Fatalf("body was passed as a command argument: %v", args)
		}
		createdBody = body
		return "https://github.com/owner/repo/pull/17", nil
	}

	result, err := RequestDraftPullRequest(context.Background(), DraftPullRequestRequest{
		Repo:     t.TempDir(),
		Base:     "main",
		Head:     "agent/web/feat/17-task",
		Title:    "Prepare isolated change",
		Body:     "First line\n\nSecond line",
		Executor: "web-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Number != 17 || result.State != "PENDING_USER_APPROVAL" || result.Executor != "web-agent" {
		t.Fatalf("result=%+v", result)
	}
	for _, expected := range []string{
		"First line\n\nSecond line",
		"<!-- GIA:DRAFT-PR:START -->",
		"executor: web-agent",
		"state: PENDING_USER_APPROVAL",
		"<!-- GIA:DRAFT-PR:END -->",
	} {
		if !strings.Contains(createdBody, expected) {
			t.Fatalf("body missing %q:\n%s", expected, createdBody)
		}
	}
}

func TestRequestDraftPullRequestRejectsInvalidRequestsBeforeGitHub(t *testing.T) {
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})
	runGitHub = func(_ context.Context, _ string, _ ...string) (string, error) {
		t.Fatal("GitHub must not be called for an invalid request")
		return "", nil
	}
	runGitHubWithBodyFile = func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
		t.Fatal("GitHub must not be called for an invalid request")
		return "", nil
	}

	valid := DraftPullRequestRequest{
		Repo:     t.TempDir(),
		Base:     "main",
		Head:     "agent/task",
		Title:    "Title",
		Executor: "web-agent",
	}
	tests := []struct {
		name   string
		mutate func(*DraftPullRequestRequest)
	}{
		{name: "empty head", mutate: func(r *DraftPullRequestRequest) { r.Head = "" }},
		{name: "empty title", mutate: func(r *DraftPullRequestRequest) { r.Title = "" }},
		{name: "empty executor", mutate: func(r *DraftPullRequestRequest) { r.Executor = "" }},
		{name: "multiline executor", mutate: func(r *DraftPullRequestRequest) { r.Executor = "web\nagent" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if _, err := RequestDraftPullRequest(context.Background(), request); err == nil {
				t.Fatal("expected request validation error")
			}
		})
	}
}

func TestRequestDraftPullRequestRejectsNonDraftRemoteResult(t *testing.T) {
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})
	const createdURL = "https://github.com/owner/repo/pull/17"
	var createdBody string
	runGitHub = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args[:2], " ") {
		case "repo view":
			return "owner/repo", nil
		case "pr view":
			record := draftPullRequestRecord{
				Number:      17,
				URL:         createdURL,
				IsDraft:     false,
				BaseRefName: "main",
				HeadRefName: "agent/task",
				Title:       "Title",
				Body:        createdBody,
			}
			data, err := json.Marshal(record)
			return string(data), err
		default:
			return "", errors.New("unexpected GitHub call")
		}
	}
	runGitHubWithBodyFile = func(_ context.Context, _ string, body string, _ ...string) (string, error) {
		createdBody = body
		return createdURL, nil
	}

	_, err := RequestDraftPullRequest(context.Background(), DraftPullRequestRequest{
		Repo:     t.TempDir(),
		Base:     "main",
		Head:     "agent/task",
		Title:    "Title",
		Executor: "web-agent",
	})
	if err == nil || !strings.Contains(err.Error(), createdURL) || !strings.Contains(err.Error(), "not a draft") {
		t.Fatalf("err=%v", err)
	}
}

func containsPair(args []string, key, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key && args[index+1] == value {
			return true
		}
	}
	return false
}
