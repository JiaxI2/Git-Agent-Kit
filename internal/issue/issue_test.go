package issue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/githubx"
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

func TestCreateBootstrapsLabelsPreservesBodyAndVerifiesIssue(t *testing.T) {
	cfg := config.Default()
	spec := FromDirection("收敛安装\n并验证完整正文", "low", "web-agent", cfg)
	labels := map[string]bool{}
	var bodyPath string
	var createdLabels []string

	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch command := strings.Join(args[:2], " "); command {
			case "repo view":
				return "owner/repo", nil
			case "label list":
				raw := "["
				first := true
				for label := range labels {
					if !first {
						raw += ","
					}
					raw += fmt.Sprintf(`{"name":%q}`, label)
					first = false
				}
				return raw + "]", nil
			case "label create":
				labels[args[2]] = true
				return "", nil
			case "issue view":
				return `{"number":42,"title":"[Agent] 收敛安装","url":"https://github.com/owner/repo/issues/42","labels":[{"name":"agent:ready"},{"name":"risk:low"},{"name":"executor:web-agent"}]}`, nil
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, body string, args ...string) (string, error) {
			if slices.Contains(args, "--body") {
				t.Fatalf("multiline body was passed as an argument: %v", args)
			}
			if !strings.Contains(body, "收敛安装\n并验证完整正文") || !strings.Contains(body, "## 验收标准") {
				t.Fatalf("body was truncated:\n%s", body)
			}
			for i, arg := range args {
				if arg == "--label" && i+1 < len(args) {
					createdLabels = append(createdLabels, args[i+1])
				}
			}
			return githubBodyFileProbe(t, body, &bodyPath)
		},
	)

	created, err := Create(context.Background(), t.TempDir(), spec, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if created.Number != 42 || created.URL != "https://github.com/owner/repo/issues/42" {
		t.Fatalf("created=%+v", created)
	}
	for _, required := range []string{"agent:ready", "agent:claimed", "agent:completed", "risk:low", "executor:web-agent"} {
		if !labels[required] {
			t.Fatalf("label %q was not bootstrapped: %v", required, labels)
		}
	}
	if !slices.Equal(createdLabels, spec.Labels) {
		t.Fatalf("created labels=%v want=%v", createdLabels, spec.Labels)
	}
	if _, err := os.Stat(bodyPath); !os.IsNotExist(err) {
		t.Fatalf("temporary body file still exists: %v", err)
	}
}

func TestCreateFailsClosedWhenLabelBootstrapIsForbidden(t *testing.T) {
	cfg := config.Default()
	spec := FromDirection("测试权限失败", "low", "web-agent", cfg)
	issueCreateCalled := false

	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch command := strings.Join(args[:2], " "); command {
			case "repo view":
				return "owner/repo", nil
			case "label list":
				return "[]", nil
			case "label create":
				return "", errors.New("HTTP 403: forbidden")
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			issueCreateCalled = true
			return "", nil
		},
	)

	_, err := Create(context.Background(), t.TempDir(), spec, cfg)
	if err == nil || !strings.Contains(err.Error(), "bootstrap GitHub label") {
		t.Fatalf("err=%v", err)
	}
	if issueCreateCalled {
		t.Fatal("issue creation continued after label bootstrap failure")
	}
}

func TestCreateRejectsEmptyGitHubOutput(t *testing.T) {
	cfg := config.Default()
	spec := FromDirection("测试空输出", "low", "web-agent", cfg)
	allLabels := labelListJSON(append([]string{
		cfg.Issue.ReadyLabel,
		cfg.Issue.ClaimedLabel,
		cfg.Issue.CompletedLabel,
	}, spec.Labels...))

	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch strings.Join(args[:2], " ") {
			case "repo view":
				return "owner/repo", nil
			case "label list":
				return allLabels, nil
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "", nil
		},
	)

	if _, err := Create(context.Background(), t.TempDir(), spec, cfg); err == nil || !strings.Contains(err.Error(), "invalid issue URL") {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateVerificationFailureReportsCreatedURL(t *testing.T) {
	cfg := config.Default()
	spec := FromDirection("测试创建后校验", "low", "web-agent", cfg)
	allLabels := labelListJSON(append([]string{
		cfg.Issue.ReadyLabel,
		cfg.Issue.ClaimedLabel,
		cfg.Issue.CompletedLabel,
	}, spec.Labels...))
	const issueURL = "https://github.com/owner/repo/issues/42"

	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch strings.Join(args[:2], " ") {
			case "repo view":
				return "owner/repo", nil
			case "label list":
				return allLabels, nil
			case "issue view":
				return `{"number":42,"title":"Task","url":"https://github.com/owner/repo/issues/42","labels":[{"name":"agent:ready"},{"name":"risk:low"}]}`, nil
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return issueURL, nil
		},
	)

	_, err := Create(context.Background(), t.TempDir(), spec, cfg)
	if err == nil || !strings.Contains(err.Error(), issueURL) || !strings.Contains(err.Error(), "may have been created") || !strings.Contains(err.Error(), "executor:web-agent") {
		t.Fatalf("err=%v", err)
	}
}

func TestListReadyProvidesReusableIssueListAPI(t *testing.T) {
	cfg := config.Default()
	var listArgs []string
	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch strings.Join(args[:2], " ") {
			case "repo view":
				return "owner/repo", nil
			case "issue list":
				listArgs = append([]string(nil), args...)
				return `[{"number":7,"title":"Task","url":"https://github.com/owner/repo/issues/7","labels":[{"name":"agent:ready"}]}]`, nil
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "", errors.New("unexpected body call")
		},
	)

	items, err := ListReady(context.Background(), t.TempDir(), 10, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Number != 7 {
		t.Fatalf("items=%+v", items)
	}
	if !containsArgumentPair(listArgs, "--label", cfg.Issue.ReadyLabel) {
		t.Fatalf("ready label missing from args: %v", listArgs)
	}
}

func TestGetReturnsIssueTitleBodyAndLabels(t *testing.T) {
	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			switch strings.Join(args[:2], " ") {
			case "repo view":
				return "owner/repo", nil
			case "issue view":
				if !slices.Contains(args, "number,title,body,url,labels") {
					t.Fatalf("issue body was not requested: %v", args)
				}
				return `{"number":7,"title":"Task title","body":"line one\nline two","url":"https://github.com/owner/repo/issues/7","labels":[{"name":"agent:claimed"}]}`, nil
			default:
				return "", fmt.Errorf("unexpected gh args: %v", args)
			}
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "", errors.New("unexpected body call")
		},
	)

	got, err := Get(context.Background(), t.TempDir(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != 7 || got.Title != "Task title" || got.Body != "line one\nline two" || !slices.Equal(got.Labels, []string{"agent:claimed"}) {
		t.Fatalf("detail=%+v", got)
	}
}

func TestListRejectsEmptyOutput(t *testing.T) {
	withGitHubRunners(t,
		func(_ context.Context, _ string, args ...string) (string, error) {
			if strings.Join(args[:2], " ") == "repo view" {
				return "owner/repo", nil
			}
			return "", nil
		},
		func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "", errors.New("unexpected body call")
		},
	)

	if _, err := List(context.Background(), t.TempDir(), ListOptions{}); err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("err=%v", err)
	}
}

func TestIssueNumberFromAbsoluteGitHubURL(t *testing.T) {
	number, err := issueNumberFromURL("https://github.com/owner/repo/issues/42")
	if err != nil {
		t.Fatal(err)
	}
	if number != 42 {
		t.Fatalf("number=%d", number)
	}
	for _, invalid := range []string{"", "owner/repo/issues/42", "https://github.com/owner/repo/pull/42"} {
		if _, err := issueNumberFromURL(invalid); err == nil {
			t.Fatalf("accepted invalid URL %q", invalid)
		}
	}
}

func withGitHubRunners(t *testing.T, run func(context.Context, string, ...string) (string, error), runBody func(context.Context, string, string, ...string) (string, error)) {
	t.Helper()
	oldRun := runGitHub
	oldRunBody := runGitHubWithBodyFile
	runGitHub = run
	runGitHubWithBodyFile = runBody
	t.Cleanup(func() {
		runGitHub = oldRun
		runGitHubWithBodyFile = oldRunBody
	})
}

func githubBodyFileProbe(t *testing.T, body string, path *string) (string, error) {
	t.Helper()
	return githubx.WithBodyFile(body, func(bodyPath string) (string, error) {
		*path = bodyPath
		data, err := os.ReadFile(bodyPath)
		if err != nil {
			return "", err
		}
		if string(data) != body {
			return "", fmt.Errorf("body mismatch")
		}
		return "https://github.com/owner/repo/issues/42", nil
	})
}

func labelListJSON(labels []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, label := range labels {
		if seen[label] {
			continue
		}
		seen[label] = true
		parts = append(parts, fmt.Sprintf(`{"name":%q}`, label))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func containsArgumentPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
