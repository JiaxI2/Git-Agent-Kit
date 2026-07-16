package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

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
		if !slices.Contains(args, "--draft") || slices.Contains(args, "--body") {
			t.Fatalf("draft/body-file contract violated: %v", args)
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
	for _, expected := range []string{"First line\n\nSecond line", "executor: web-agent", "state: PENDING_USER_APPROVAL"} {
		if !strings.Contains(createdBody, expected) {
			t.Fatalf("body missing %q:\n%s", expected, createdBody)
		}
	}
}

func TestRequestDraftPullRequestRejectsInvalidOrNonDraftResults(t *testing.T) {
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
	if _, err := RequestDraftPullRequest(context.Background(), DraftPullRequestRequest{
		Repo: t.TempDir(), Base: "main", Title: "Title", Executor: "web-agent",
	}); err == nil {
		t.Fatal("missing head must fail before GitHub")
	}

	const createdURL = "https://github.com/owner/repo/pull/17"
	var createdBody string
	runGitHub = func(_ context.Context, _ string, args ...string) (string, error) {
		switch strings.Join(args[:2], " ") {
		case "repo view":
			return "owner/repo", nil
		case "pr view":
			data, err := json.Marshal(draftPullRequestRecord{
				Number: 17, URL: createdURL, IsDraft: false, BaseRefName: "main",
				HeadRefName: "agent/task", Title: "Title", Body: createdBody,
			})
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
		Repo: t.TempDir(), Base: "main", Head: "agent/task", Title: "Title", Executor: "web-agent",
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
