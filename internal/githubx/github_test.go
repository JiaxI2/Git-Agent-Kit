package githubx

import (
	"context"
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestWithBodyFilePreservesMultilineAndRemovesFile(t *testing.T) {
	body := "first line\n\n- second line\n<!-- marker -->\n"
	var path string

	out, err := WithBodyFile(body, func(bodyPath string) (string, error) {
		path = bodyPath
		data, err := os.ReadFile(bodyPath)
		if err != nil {
			return "", err
		}
		if string(data) != body {
			t.Fatalf("body mismatch:\n%s", data)
		}
		info, err := os.Stat(bodyPath)
		if err != nil {
			return "", err
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("temporary body permissions are too broad: %o", info.Mode().Perm())
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("out=%q", out)
	}
	if path == "" {
		t.Fatal("temporary path was not captured")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temporary body file still exists: %v", err)
	}
}

func TestWithBodyFileRejectsMissingCallback(t *testing.T) {
	_, err := WithBodyFile("body", nil)
	if err == nil || !strings.Contains(err.Error(), "callback") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunWithBodyFileRemovesFileAfterSuccessFailureAndCancellation(t *testing.T) {
	oldRunner := commandRunner
	t.Cleanup(func() {
		commandRunner = oldRunner
	})

	tests := []struct {
		name      string
		cancel    bool
		runErr    error
		wantError bool
	}{
		{name: "success"},
		{name: "failure", runErr: errors.New("simulated gh failure"), wantError: true},
		{name: "cancellation", cancel: true, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			commandRunner = func(ctx context.Context, _ string, args ...string) (string, error) {
				index := slices.Index(args, "--body-file")
				if index < 0 || index+1 >= len(args) {
					t.Fatalf("--body-file missing from args: %v", args)
				}
				path = args[index+1]
				data, err := os.ReadFile(path)
				if err != nil {
					return "", err
				}
				if string(data) != "line one\nline two\n" {
					t.Fatalf("body mismatch: %q", data)
				}
				if err := ctx.Err(); err != nil {
					return "", err
				}
				if tc.runErr != nil {
					return "", tc.runErr
				}
				return "ok", nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancel {
				cancel()
			} else {
				defer cancel()
			}

			out, err := RunWithBodyFile(ctx, t.TempDir(), "line one\nline two\n", "issue", "comment", "7")
			if tc.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !tc.wantError && (err != nil || out != "ok") {
				t.Fatalf("out=%q err=%v", out, err)
			}
			if path == "" {
				t.Fatal("temporary body path was not captured")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("temporary body file still exists: %v", err)
			}
		})
	}
}

func TestResourceNumberFromURL(t *testing.T) {
	number, err := ResourceNumberFromURL("https://github.com/owner/repo/pull/42", "pull")
	if err != nil {
		t.Fatal(err)
	}
	if number != 42 {
		t.Fatalf("number=%d", number)
	}
	for _, test := range []struct {
		url      string
		resource string
	}{
		{url: "", resource: "pull"},
		{url: "owner/repo/pull/42", resource: "pull"},
		{url: "https://github.com/owner/repo/issues/42", resource: "pull"},
		{url: "https://github.com/owner/repo/pull/0", resource: "pull"},
		{url: "https://github.com/owner/repo/pull/42", resource: ""},
	} {
		if _, err := ResourceNumberFromURL(test.url, test.resource); err == nil {
			t.Fatalf("accepted url=%q resource=%q", test.url, test.resource)
		}
	}
}
