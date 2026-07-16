package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/git-isolated-agent-kit/internal/config"
	"github.com/example/git-isolated-agent-kit/internal/gitx"
	"github.com/example/git-isolated-agent-kit/internal/issue"
	"github.com/example/git-isolated-agent-kit/internal/notify"
	"github.com/example/git-isolated-agent-kit/internal/validate"
	"github.com/example/git-isolated-agent-kit/internal/workflow"
)

const version = "0.1.0"

type output struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		emit(output{OK: false, Error: err.Error()})
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "version":
		emit(output{OK: true, Command: "version", Data: map[string]string{"version": version}})
		return nil
	case "init":
		return cmdInit(args[1:])
	case "doctor":
		return cmdDoctor(ctx, args[1:])
	case "issue":
		return cmdIssue(ctx, args[1:])
	case "scan":
		return cmdScan(ctx, args[1:])
	case "claim":
		return cmdClaim(ctx, args[1:])
	case "worktree":
		return cmdWorktree(ctx, args[1:])
	case "validate":
		return cmdValidate(ctx, args[1:])
	case "handoff":
		return cmdHandoff(ctx, args[1:])
	case "status":
		return cmdStatus(ctx, args[1:])
	case "feedback":
		return cmdFeedback(ctx, args[1:])
	case "notify":
		return cmdNotify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repo := fs.String("repo", ".", "target repository")
	force := fs.Bool("force", false, "overwrite existing config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	abs, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	if err := workflow.Initialize(abs, *force); err != nil {
		return err
	}
	emit(output{OK: true, Command: "init", Data: map[string]string{"repo": abs, "config": filepath.Join(abs, ".gia", "config.json")}})
	return nil
}

func cmdDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report := workflow.Doctor(ctx, *repo)
	emit(output{OK: report.OK, Command: "doctor", Data: report})
	if !report.OK {
		return errors.New("doctor checks failed")
	}
	return nil
}

func cmdIssue(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("usage: gia issue create --direction <text>")
	}
	fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	direction := fs.String("direction", "", "optimization direction or task request")
	risk := fs.String("risk", "auto", "auto|low|medium|high")
	executor := fs.String("executor", "web-agent", "preferred executor")
	dryRun := fs.Bool("dry-run", false, "render only")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*direction) == "" {
		return errors.New("--direction is required")
	}
	cfg, err := config.LoadFromRepo(*repo)
	if err != nil {
		return err
	}
	spec := issue.FromDirection(*direction, *risk, *executor, cfg)
	if *dryRun {
		emit(output{OK: true, Command: "issue create", Data: spec})
		return nil
	}
	created, err := issue.Create(ctx, *repo, spec, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "issue create", Data: created})
	return nil
}

func cmdScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	limit := fs.Int("limit", 20, "maximum issues")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadFromRepo(*repo)
	if err != nil {
		return err
	}
	items, err := issue.Scan(ctx, *repo, *limit, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "scan", Data: items})
	return nil
}

func cmdClaim(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	number := fs.Int("issue", 0, "issue number")
	executor := fs.String("executor", "web-agent", "executor")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *number <= 0 {
		return errors.New("--issue is required")
	}
	cfg, err := config.LoadFromRepo(*repo)
	if err != nil {
		return err
	}
	result, err := workflow.Claim(ctx, *repo, *number, *executor, cfg)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "claim", Data: result})
	return nil
}

func cmdWorktree(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gia worktree create|remove")
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("worktree create", flag.ContinueOnError)
		repo := fs.String("repo", ".", "repository")
		pr := fs.Int("pr", 0, "pull request number")
		ref := fs.String("ref", "", "remote ref when PR lookup is unavailable")
		root := fs.String("root", "", "worktree root")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *pr <= 0 && *ref == "" {
			return errors.New("--pr or --ref is required")
		}
		result, err := workflow.CreateValidationWorktree(ctx, *repo, *pr, *ref, *root)
		if err != nil {
			return err
		}
		emit(output{OK: true, Command: "worktree create", Data: result})
		return nil
	case "remove":
		fs := flag.NewFlagSet("worktree remove", flag.ContinueOnError)
		repo := fs.String("repo", ".", "repository")
		path := fs.String("path", "", "worktree path")
		force := fs.Bool("force", false, "remove dirty worktree")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *path == "" {
			return errors.New("--path is required")
		}
		if err := gitx.RemoveWorktree(ctx, *repo, *path, *force); err != nil {
			return err
		}
		emit(output{OK: true, Command: "worktree remove", Data: map[string]interface{}{"path": *path, "force": *force}})
		return nil
	default:
		return errors.New("usage: gia worktree create|remove")
	}
}

func cmdValidate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	profile := fs.String("profile", "full", "smoke|full|release")
	expected := fs.String("expected-head", "", "expected commit SHA")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadFromRepo(*repo)
	if err != nil {
		return err
	}
	report, err := validate.Run(ctx, *repo, *profile, *expected, cfg)
	emit(output{OK: err == nil && report.OK, Command: "validate", Data: report})
	return err
}

func cmdHandoff(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	pr := fs.Int("pr", 0, "pull request number")
	to := fs.String("to", "", "next executor")
	state := fs.String("state", "", "workflow state")
	note := fs.String("note", "", "handoff note")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pr <= 0 || *to == "" || *state == "" {
		return errors.New("--pr, --to and --state are required")
	}
	result, err := workflow.Handoff(ctx, *repo, *pr, *to, *state, *note)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "handoff", Data: result})
	return nil
}

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := workflow.Status(ctx, *repo)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "status", Data: result})
	return nil
}

func cmdFeedback(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("feedback", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	category := fs.String("category", "improvement", "bug|improvement|ux|security")
	message := fs.String("message", "", "feedback text")
	pr := fs.Int("pr", 0, "related PR")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*message) == "" {
		return errors.New("--message is required")
	}
	result, err := workflow.RecordFeedback(ctx, *repo, *category, *message, *pr)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "feedback", Data: result})
	return nil
}

func cmdNotify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository")
	event := fs.String("event", "manual", "event name")
	message := fs.String("message", "", "message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.LoadFromRepo(*repo)
	if err != nil {
		return err
	}
	result, err := notify.Send(ctx, *event, *message, cfg.Notifications)
	if err != nil {
		return err
	}
	emit(output{OK: true, Command: "notify", Data: result})
	return nil
}

func emit(v output) {
	v.Command = strings.TrimSpace(v.Command)
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func usage() {
	fmt.Printf(`gia %s - Git Isolated Agent Kit

Commands:
  init       initialize .gia in a repository
  doctor     check dependencies and repository safety
  issue      create a structured GitHub issue from one direction
  scan       find executable agent issues
  claim      atomically claim an issue and prepare a branch
  worktree   create/remove isolated validation worktrees
  validate   run smoke/full/release profile bound to a commit
  handoff    transfer single-writer ownership through PR metadata
  status     inspect branch/worktree/repository state
  feedback   record iterative kit feedback
  notify     send configured notifications
  version    print version
`, version)
	_ = time.Second
}
