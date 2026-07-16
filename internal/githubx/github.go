package githubx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var commandRunner = Run

// Run executes gh in repo and returns trimmed standard output.
func Run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = repo
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// RunWithBodyFile passes body through gh's --body-file option. This avoids
// multiline argument truncation by Windows command shims.
func RunWithBodyFile(ctx context.Context, repo, body string, args ...string) (string, error) {
	return WithBodyFile(body, func(path string) (string, error) {
		bodyArgs := append([]string(nil), args...)
		bodyArgs = append(bodyArgs, "--body-file", path)
		return commandRunner(ctx, repo, bodyArgs...)
	})
}

// WithBodyFile writes body to a private temporary file for the duration of fn.
func WithBodyFile(body string, fn func(path string) (string, error)) (out string, err error) {
	if fn == nil {
		return "", errors.New("body file callback is required")
	}
	file, err := os.CreateTemp("", "gia-github-body-*.md")
	if err != nil {
		return "", fmt.Errorf("create temporary GitHub body file: %w", err)
	}
	path := file.Name()
	defer func() {
		removeErr := os.Remove(path)
		if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
			return
		}
		cleanupErr := fmt.Errorf("remove temporary GitHub body file %s: %w", path, removeErr)
		if err == nil {
			err = cleanupErr
			return
		}
		err = errors.Join(err, cleanupErr)
	}()

	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("restrict temporary GitHub body file permissions: %w", err)
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write temporary GitHub body file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temporary GitHub body file: %w", err)
	}
	return fn(path)
}

// ResourceNumberFromURL extracts the numeric identifier from a GitHub resource
// URL such as https://github.com/owner/repo/pull/42.
func ResourceNumberFromURL(raw, resource string) (int, error) {
	value := strings.TrimSpace(raw)
	kind := strings.Trim(strings.TrimSpace(resource), "/")
	if kind == "" || strings.Contains(kind, "/") {
		return 0, fmt.Errorf("invalid GitHub resource kind %q", resource)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return 0, fmt.Errorf("invalid GitHub resource URL %q", value)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 4 || parts[len(parts)-2] != kind {
		return 0, fmt.Errorf("invalid GitHub %s URL %q", kind, value)
	}
	number, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("invalid GitHub %s URL %q", kind, value)
	}
	return number, nil
}
