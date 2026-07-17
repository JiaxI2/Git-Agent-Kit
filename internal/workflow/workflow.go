package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/githubx"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/gitx"
)

var claimRun = run

type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}
type DoctorReport struct {
	OK     bool          `json:"ok"`
	Checks []DoctorCheck `json:"checks"`
}
type ClaimResult struct {
	Issue    int    `json:"issue"`
	Branch   string `json:"branch"`
	Executor string `json:"executor"`
	State    string `json:"state"`
	Head     string `json:"head"`
}
type WorktreeResult struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Ref    string `json:"ref"`
	Head   string `json:"head"`
}
type HandoffResult struct {
	PR    int    `json:"pr"`
	To    string `json:"to"`
	State string `json:"state"`
	Head  string `json:"head"`
}
type DraftPullRequestRequest struct {
	Repo     string `json:"repo"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Executor string `json:"executor"`
}
type DraftPullRequestResult struct {
	Number   int    `json:"number"`
	URL      string `json:"url"`
	State    string `json:"state"`
	Base     string `json:"base"`
	Head     string `json:"head"`
	Title    string `json:"title"`
	Executor string `json:"executor"`
}
type draftPullRequestRecord struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	IsDraft     bool   `json:"isDraft"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	Body        string `json:"body"`
}
type StatusResult struct {
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Clean     bool   `json:"clean"`
	Worktrees string `json:"worktrees"`
}
type FeedbackResult struct {
	Path     string `json:"path"`
	Category string `json:"category"`
}

var runGitHub = githubx.Run
var runGitHubWithBodyFile = githubx.RunWithBodyFile
var claimRunWithBodyFile = githubx.RunWithBodyFile
var doctorRun = run

type githubIdentity struct {
	Kind       string
	Actor      string
	Owner      string
	Repository string
}

type effectiveRule struct {
	Type       string `json:"type"`
	Parameters struct {
		RequiredApprovingReviewCount int  `json:"required_approving_review_count"`
		RequireCodeOwnerReview       bool `json:"require_code_owner_review"`
		RequireLastPushApproval      bool `json:"require_last_push_approval"`
	} `json:"parameters"`
}

func Initialize(repo string, force bool) error {
	_, err := InitializeWithFormat(repo, "json", force)
	return err
}

func InitializeWithFormat(repo, format string, force bool) (string, error) {
	cfgPath, err := config.PathForFormat(repo, format)
	if err != nil {
		return "", err
	}
	existing, err := config.ExistingPaths(repo)
	if err != nil {
		return "", err
	}
	if len(existing) > 0 {
		if !force {
			return "", fmt.Errorf("GIA config already exists (%s); pass --force only to overwrite the same single format", strings.Join(existing, ", "))
		}
		if len(existing) != 1 || filepath.Clean(existing[0]) != filepath.Clean(cfgPath) {
			return "", fmt.Errorf("refusing --force for %s while config files exist (%s); GIA will not delete or choose between formats, so keep only %s and retry", cfgPath, strings.Join(existing, ", "), cfgPath)
		}
	}
	dir := filepath.Join(repo, ".gia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := config.Save(cfgPath, config.Default()); err != nil {
		return "", err
	}
	templates := map[string]string{
		"issue.md":   "# Agent Task\n\nDescribe one optimization direction. GIA converts it into a controlled issue.\n",
		"handoff.md": "<!-- GIA:HANDOFF:START -->\nexecutor: web-agent\nstate: WEB_OWNED\nhead_sha: <sha>\nnext_executor: local-agent\n<!-- GIA:HANDOFF:END -->\n",
	}
	tdir := filepath.Join(dir, "templates")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		return "", err
	}
	for n, c := range templates {
		if err := os.WriteFile(filepath.Join(tdir, n), []byte(c), 0o644); err != nil {
			return "", err
		}
	}
	return cfgPath, nil
}

func Doctor(ctx context.Context, repo string) DoctorReport {
	return DoctorWithConfig(ctx, repo, "")
}

func DoctorWithConfig(ctx context.Context, repo, configPath string) DoctorReport {
	checks := []DoctorCheck{}
	add := func(name string, err error, detail string) {
		checks = append(checks, DoctorCheck{name, err == nil, detail})
	}
	_, err := exec.LookPath("git")
	add("git", err, "Git CLI")
	_, err = exec.LookPath("gh")
	add("gh", err, "GitHub CLI; required for issue/PR automation")
	_, err = exec.LookPath("go")
	add("go", err, "Go toolchain")
	_, err = gitx.Run(ctx, repo, "rev-parse", "--is-inside-work-tree")
	add("repository", err, repo)
	cfg, configErr := config.Load(repo, configPath)
	configDetail := "auto-discover exactly one .gia/config.json, config.yaml, or config.yml"
	if strings.TrimSpace(configPath) != "" {
		configDetail = "explicit config: " + configPath
	}
	add("config", configErr, configDetail)
	add("permission-boundary", nil, "permissions.identityMode and approvalMode declare workflow intent; a separate GitHub App/token plus GitHub ruleset remains the hard approval, merge, and release boundary")
	_, authErr := doctorRun(ctx, repo, "gh", "auth", "status")
	add("github-auth", authErr, "gh auth status")
	if configErr != nil || authErr != nil {
		detail := "requires a valid GIA config and authenticated gh session"
		add("github-identity", errors.New(detail), detail)
		add("approval-policy", errors.New(detail), detail)
	} else {
		identity, detail, identityErr := inspectGitHubIdentity(ctx, repo, cfg)
		add("github-identity", identityErr, detail)
		if identityErr != nil {
			detail = "requires a GitHub actor matching permissions.identityMode"
			add("approval-policy", errors.New(detail), detail)
		} else {
			detail, approvalErr := inspectApprovalPolicy(ctx, repo, cfg, identity)
			add("approval-policy", approvalErr, detail)
		}
	}
	ok := true
	for _, c := range checks {
		if !c.OK {
			ok = false
		}
	}
	return DoctorReport{ok, checks}
}

func inspectGitHubIdentity(ctx context.Context, repo string, cfg config.Config) (githubIdentity, string, error) {
	full, err := doctorRun(ctx, repo, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return githubIdentity{}, "cannot resolve the target GitHub repository", err
	}
	parts := strings.SplitN(strings.TrimSpace(full), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return githubIdentity{}, fmt.Sprintf("gh repo view returned invalid nameWithOwner %q", full), fmt.Errorf("invalid GitHub repository identity")
	}
	identity := githubIdentity{Owner: parts[0], Repository: strings.TrimSpace(full)}
	switch cfg.Permissions.IdentityMode {
	case config.IdentityModeSharedUser, config.IdentityModeTeam:
		actor, actorErr := doctorRun(ctx, repo, "gh", "api", "user", "--jq", ".login")
		if actorErr != nil {
			detail := fmt.Sprintf("permissions.identityMode=%s requires a GitHub user token, but gh did not resolve a user actor", cfg.Permissions.IdentityMode)
			return githubIdentity{}, detail, actorErr
		}
		identity.Kind = "user"
		identity.Actor = strings.TrimSpace(actor)
		if identity.Actor == "" {
			return githubIdentity{}, "gh api user returned an empty login", fmt.Errorf("empty GitHub user actor")
		}
	case config.IdentityModeGitHubApp:
		if _, appErr := doctorRun(ctx, repo, "gh", "api", "installation/repositories", "--jq", ".total_count"); appErr != nil {
			actor, userErr := doctorRun(ctx, repo, "gh", "api", "user", "--jq", ".login")
			if userErr == nil && strings.TrimSpace(actor) != "" {
				detail := fmt.Sprintf("permissions.identityMode=%s, but gh is authenticated as user %s", cfg.Permissions.IdentityMode, strings.TrimSpace(actor))
				return githubIdentity{}, detail, fmt.Errorf("GitHub identity mode mismatch")
			}
			return githubIdentity{}, "permissions.identityMode=github-app requires a GitHub App installation token", appErr
		}
		identity.Kind = "github-app"
		identity.Actor = "github-app-installation"
	default:
		return githubIdentity{}, "unsupported permissions.identityMode", fmt.Errorf("unsupported identity mode %q", cfg.Permissions.IdentityMode)
	}
	detail := fmt.Sprintf("mode=%s actor=%s owner=%s repository=%s", cfg.Permissions.IdentityMode, identity.Actor, identity.Owner, identity.Repository)
	return identity, detail, nil
}

func inspectApprovalPolicy(ctx context.Context, repo string, cfg config.Config, identity githubIdentity) (string, error) {
	branch := strings.TrimSpace(cfg.DefaultBranch)
	if branch == "" {
		return "defaultBranch is empty", fmt.Errorf("cannot inspect approval rules without defaultBranch")
	}
	endpoint := fmt.Sprintf("repos/%s/rules/branches/%s", identity.Repository, url.PathEscape(branch))
	out, err := doctorRun(ctx, repo, "gh", "api", endpoint)
	if err != nil {
		return fmt.Sprintf("cannot inspect effective GitHub rules for %s", branch), err
	}
	var rules []effectiveRule
	if err := json.Unmarshal([]byte(out), &rules); err != nil {
		return "cannot parse effective GitHub rules", err
	}
	var pullRequest *effectiveRule
	for i := range rules {
		if rules[i].Type == "pull_request" {
			pullRequest = &rules[i]
			break
		}
	}
	if pullRequest == nil {
		detail := fmt.Sprintf("approvalMode=%s requires an effective pull_request rule on %s", cfg.Permissions.ApprovalMode, branch)
		return detail, fmt.Errorf("missing pull request rule")
	}
	parameters := pullRequest.Parameters
	detail := fmt.Sprintf("mode=%s approvals=%d codeOwner=%t lastPush=%t branch=%s", cfg.Permissions.ApprovalMode, parameters.RequiredApprovingReviewCount, parameters.RequireCodeOwnerReview, parameters.RequireLastPushApproval, branch)
	switch cfg.Permissions.ApprovalMode {
	case config.ApprovalModeOwnerMerge:
		if parameters.RequiredApprovingReviewCount > 0 || parameters.RequireCodeOwnerReview || parameters.RequireLastPushApproval {
			return detail + "; owner-merge requires zero formal approvals and no code-owner/last-push approval gate", fmt.Errorf("GitHub approval rules conflict with owner-merge")
		}
	case config.ApprovalModeRequiredReview:
		if parameters.RequiredApprovingReviewCount < 1 && !parameters.RequireCodeOwnerReview {
			return detail + "; required-review is not enforced by GitHub", fmt.Errorf("GitHub approval rules do not require a review")
		}
		if cfg.Permissions.IdentityMode == config.IdentityModeSharedUser && strings.EqualFold(identity.Actor, identity.Owner) {
			return detail + "; shared-user actor is the repository owner and cannot approve its own PR; use owner-merge, team, or github-app", fmt.Errorf("shared user identity cannot satisfy owner review")
		}
	default:
		return detail, fmt.Errorf("unsupported approval mode %q", cfg.Permissions.ApprovalMode)
	}
	return detail, nil
}

func Claim(ctx context.Context, repo string, issue int, executor string, cfg config.Config) (ClaimResult, error) {
	if !cfg.Protected.RejectForcePush {
		return ClaimResult{}, fmt.Errorf("claim requires protected.rejectForcePush=true; GIA never updates an existing remote claim ref")
	}
	if !cfg.ExecutorAllowed(executor) {
		return ClaimResult{}, fmt.Errorf("executor %q is not allowed by permissions.allowedExecutors", strings.TrimSpace(executor))
	}
	executorID := sanitize(executor)
	if executorID == "" {
		return ClaimResult{}, fmt.Errorf("executor must contain at least one letter or number")
	}
	if strings.TrimSpace(cfg.Issue.ReadyLabel) == "" || strings.TrimSpace(cfg.Issue.ClaimedLabel) == "" {
		return ClaimResult{}, fmt.Errorf("issue readyLabel and claimedLabel must be configured")
	}
	remote := cfg.ValidationRemote()
	if _, err := gitx.Run(ctx, repo, "fetch", "--prune", remote); err != nil {
		return ClaimResult{}, err
	}
	base := remote + "/" + cfg.DefaultBranch
	head, err := gitx.Run(ctx, repo, "rev-parse", base)
	if err != nil {
		return ClaimResult{}, err
	}
	slug := fmt.Sprintf("issue-%d", issue)
	branch := strings.NewReplacer("{executor}", executorID, "{type}", "feat", "{issue}", fmt.Sprint(issue), "{slug}", slug).Replace(cfg.Branches.Pattern)
	leaseBranch := fmt.Sprintf("gia/claims/%d", issue)
	for _, candidate := range []string{branch, leaseBranch} {
		if config.MatchAny(cfg.Protected.Branches, candidate) {
			return ClaimResult{}, fmt.Errorf("refuse claim branch %q: matches protected.branches", candidate)
		}
	}
	full, err := claimRun(ctx, repo, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return ClaimResult{}, err
	}
	full = strings.TrimSpace(full)
	if full == "" {
		return ClaimResult{}, fmt.Errorf("gh repo view returned an empty nameWithOwner")
	}
	if err := claimableIssue(ctx, repo, full, issue, cfg); err != nil {
		return ClaimResult{}, err
	}
	if _, err := gitx.Run(ctx, repo, "branch", branch, head); err != nil {
		return ClaimResult{}, err
	}
	artifacts := claimArtifacts{localBranch: branch}
	if err := pushNewRemoteBranch(ctx, repo, remote, head, leaseBranch, false); err != nil {
		return ClaimResult{}, claimFailure(err, cleanupClaim(ctx, repo, remote, full, issue, executorID, cfg, artifacts), artifacts)
	}
	artifacts.leaseBranch = leaseBranch
	if err := pushNewRemoteBranch(ctx, repo, remote, branch, branch, true); err != nil {
		return ClaimResult{}, claimFailure(err, cleanupClaim(ctx, repo, remote, full, issue, executorID, cfg, artifacts), artifacts)
	}
	artifacts.taskBranch = branch

	artifacts.labelsChanged = true
	if _, err := claimRun(ctx, repo, "gh", "issue", "edit", fmt.Sprint(issue), "--repo", full, "--add-label", cfg.Issue.ClaimedLabel, "--remove-label", cfg.Issue.ReadyLabel, "--add-label", "executor:"+executorID); err != nil {
		return ClaimResult{}, claimFailure(fmt.Errorf("update issue claim labels: %w", err), cleanupClaim(ctx, repo, remote, full, issue, executorID, cfg, artifacts), artifacts)
	}
	if err := writeClaimAudit(ctx, repo, full, issue, executor, branch, head); err != nil {
		return ClaimResult{}, claimFailure(fmt.Errorf("write issue claim audit comment: %w", err), cleanupClaim(ctx, repo, remote, full, issue, executorID, cfg, artifacts), artifacts)
	}
	return ClaimResult{issue, branch, executor, "CLAIMED", head}, nil
}

type claimArtifacts struct {
	localBranch   string
	leaseBranch   string
	taskBranch    string
	labelsChanged bool
}

type issueState struct {
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func claimableIssue(ctx context.Context, repo, full string, issue int, cfg config.Config) error {
	out, err := claimRun(ctx, repo, "gh", "issue", "view", fmt.Sprint(issue), "--repo", full, "--json", "state,labels")
	if err != nil {
		return fmt.Errorf("inspect issue before claim: %w", err)
	}
	var state issueState
	if err := json.Unmarshal([]byte(out), &state); err != nil {
		return fmt.Errorf("parse issue state before claim: %w", err)
	}
	if !strings.EqualFold(state.State, "OPEN") {
		return fmt.Errorf("issue #%d is not open", issue)
	}
	ready := false
	for _, label := range state.Labels {
		if label.Name == cfg.Issue.ClaimedLabel {
			return fmt.Errorf("issue #%d is already claimed", issue)
		}
		if label.Name == cfg.Issue.ReadyLabel {
			ready = true
		}
	}
	if !ready {
		return fmt.Errorf("issue #%d is not claimable: missing label %q", issue, cfg.Issue.ReadyLabel)
	}
	return nil
}

func pushNewRemoteBranch(ctx context.Context, repo, remote, source, branch string, setUpstream bool) error {
	args := []string{"push", "--porcelain"}
	if setUpstream {
		args = append(args, "-u")
	}
	ref := "refs/heads/" + branch
	args = append(args, remote, source+":"+ref)
	out, err := gitx.Run(ctx, repo, args...)
	if err != nil {
		return fmt.Errorf("create remote branch %q: %w", branch, err)
	}
	if !createdRemoteRef(out, ref) {
		return fmt.Errorf("remote branch %q already exists; ownership was not acquired", branch)
	}
	return nil
}

func createdRemoteRef(output, ref string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 && fields[0] == "*" && strings.HasSuffix(fields[1], ":"+ref) && fields[2] == "[new branch]" {
			return true
		}
	}
	return false
}

func writeClaimAudit(ctx context.Context, repo, full string, issue int, executor, branch, head string) error {
	body := fmt.Sprintf("GIA claimed this task.\n\n- executor: `%s`\n- branch: `%s`\n- base head: `%s`\n- state: `CLAIMED`", executor, branch, head)
	_, err := claimRunWithBodyFile(ctx, repo, body, "issue", "comment", fmt.Sprint(issue), "--repo", full)
	return err
}

func cleanupClaim(ctx context.Context, repo, remote, full string, issue int, executor string, cfg config.Config, artifacts claimArtifacts) error {
	var cleanupErrors []error
	if artifacts.labelsChanged {
		if _, err := claimRun(ctx, repo, "gh", "issue", "edit", fmt.Sprint(issue), "--repo", full, "--add-label", cfg.Issue.ReadyLabel, "--remove-label", cfg.Issue.ClaimedLabel, "--remove-label", "executor:"+executor); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("restore issue labels: %w", err))
		}
	}
	for _, branch := range []string{artifacts.taskBranch, artifacts.leaseBranch} {
		if branch == "" {
			continue
		}
		if _, err := gitx.Run(ctx, repo, "push", "--porcelain", remote, "--delete", branch); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete remote branch %q: %w", branch, err))
		}
	}
	if artifacts.localBranch != "" {
		if _, err := gitx.Run(ctx, repo, "branch", "-d", artifacts.localBranch); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete local branch %q: %w", artifacts.localBranch, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func claimFailure(cause, cleanupErr error, artifacts claimArtifacts) error {
	if cleanupErr != nil {
		return fmt.Errorf("claim failed before CLAIMED: %w; automatic recovery was incomplete (lease=%q, task=%q, local=%q): %v", cause, artifacts.leaseBranch, artifacts.taskBranch, artifacts.localBranch, cleanupErr)
	}
	return fmt.Errorf("claim failed before CLAIMED: %w; created claim artifacts were rolled back", cause)
}

func CreateValidationWorktree(ctx context.Context, repo string, pr int, ref, root string) (WorktreeResult, error) {
	if err := gitx.Fetch(ctx, repo); err != nil {
		return WorktreeResult{}, err
	}
	if ref == "" {
		out, err := run(ctx, repo, "gh", "pr", "view", fmt.Sprint(pr), "--json", "headRefName", "--jq", ".headRefName")
		if err != nil {
			return WorktreeResult{}, err
		}
		ref = "origin/" + strings.TrimSpace(out)
	}
	if root == "" {
		cfg, err := config.LoadFromRepo(repo)
		if err != nil {
			return WorktreeResult{}, err
		}
		root = cfg.Worktrees.Root
		if !filepath.IsAbs(root) {
			root = filepath.Join(repo, root)
		}
	}
	name := fmt.Sprintf("pr-%d", pr)
	if pr == 0 {
		name = sanitize(strings.TrimPrefix(ref, "origin/"))
	}
	path := filepath.Clean(filepath.Join(root, name))
	branch := "verify/" + name
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WorktreeResult{}, err
	}
	if err := gitx.CreateWorktree(ctx, repo, path, branch, ref); err != nil {
		return WorktreeResult{}, err
	}
	head, err := gitx.Head(ctx, path)
	if err != nil {
		return WorktreeResult{}, err
	}
	return WorktreeResult{path, branch, ref, head}, nil
}

func Handoff(ctx context.Context, repo string, pr int, to, state, note string) (HandoffResult, error) {
	head, err := runGitHub(ctx, repo, "pr", "view", fmt.Sprint(pr), "--json", "headRefOid", "--jq", ".headRefOid")
	if err != nil {
		return HandoffResult{}, err
	}
	head = strings.TrimSpace(head)
	if head == "" {
		return HandoffResult{}, fmt.Errorf("gh pr view returned empty headRefOid for PR #%d", pr)
	}
	body := fmt.Sprintf("<!-- GIA:HANDOFF:START -->\nexecutor: %s\nstate: %s\nhead_sha: %s\nnext_executor: %s\nnote: %s\n<!-- GIA:HANDOFF:END -->", to, state, head, to, escapeLine(note))
	if _, err := runGitHubWithBodyFile(ctx, repo, body, "pr", "comment", fmt.Sprint(pr)); err != nil {
		return HandoffResult{}, err
	}
	return HandoffResult{pr, to, state, head}, nil
}

func RequestDraftPullRequest(ctx context.Context, request DraftPullRequestRequest) (DraftPullRequestResult, error) {
	repo := strings.TrimSpace(request.Repo)
	base := strings.TrimSpace(request.Base)
	head := strings.TrimSpace(request.Head)
	title := strings.TrimSpace(request.Title)
	executor := strings.TrimSpace(request.Executor)
	fields := []struct {
		name  string
		value string
	}{
		{name: "repo", value: repo},
		{name: "base", value: base},
		{name: "head", value: head},
		{name: "title", value: title},
		{name: "executor", value: executor},
	}
	for _, field := range fields {
		if field.value == "" {
			return DraftPullRequestResult{}, fmt.Errorf("draft PR %s is required", field.name)
		}
	}
	for _, field := range fields[1:] {
		if strings.ContainsAny(field.value, "\r\n") {
			return DraftPullRequestResult{}, fmt.Errorf("draft PR %s must be a single line", field.name)
		}
	}

	full, err := githubRepositoryName(ctx, repo)
	if err != nil {
		return DraftPullRequestResult{}, err
	}
	body := draftPullRequestBody(request.Body, executor)
	out, err := runGitHubWithBodyFile(ctx, repo, body,
		"pr", "create",
		"--repo", full,
		"--draft",
		"--base", base,
		"--head", head,
		"--title", title,
	)
	if err != nil {
		return DraftPullRequestResult{}, err
	}
	number, err := githubx.ResourceNumberFromURL(out, "pull")
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("parse created draft PR: %w", err)
	}
	createdURL := strings.TrimSpace(out)
	record, err := viewDraftPullRequest(ctx, repo, full, number)
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("draft PR may have been created at %s, but verification failed: %w", createdURL, err)
	}
	expectedHead := head
	if index := strings.LastIndex(expectedHead, ":"); index >= 0 {
		expectedHead = expectedHead[index+1:]
	}
	switch {
	case !record.IsDraft:
		err = fmt.Errorf("PR #%d is not a draft", number)
	case record.BaseRefName != base:
		err = fmt.Errorf("PR #%d base is %q, expected %q", number, record.BaseRefName, base)
	case record.HeadRefName != expectedHead:
		err = fmt.Errorf("PR #%d head is %q, expected %q", number, record.HeadRefName, expectedHead)
	case record.Title != title:
		err = fmt.Errorf("PR #%d title does not match the request", number)
	case normalizeBodyForComparison(record.Body) != normalizeBodyForComparison(body):
		err = fmt.Errorf("PR #%d body does not match the request", number)
	}
	if err != nil {
		return DraftPullRequestResult{}, fmt.Errorf("draft PR may have been created at %s, but verification failed: %w", createdURL, err)
	}
	return DraftPullRequestResult{
		Number:   record.Number,
		URL:      record.URL,
		State:    "PENDING_USER_APPROVAL",
		Base:     record.BaseRefName,
		Head:     record.HeadRefName,
		Title:    record.Title,
		Executor: executor,
	}, nil
}

func githubRepositoryName(ctx context.Context, repo string) (string, error) {
	out, err := runGitHub(ctx, repo, "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return "", err
	}
	full := strings.TrimSpace(out)
	parts := strings.Split(full, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("gh repo view returned invalid nameWithOwner %q", full)
	}
	return full, nil
}

func viewDraftPullRequest(ctx context.Context, repo, full string, number int) (draftPullRequestRecord, error) {
	out, err := runGitHub(ctx, repo,
		"pr", "view", fmt.Sprint(number),
		"--repo", full,
		"--json", "number,url,isDraft,baseRefName,headRefName,title,body",
	)
	if err != nil {
		return draftPullRequestRecord{}, err
	}
	if strings.TrimSpace(out) == "" {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned empty output")
	}
	var record draftPullRequestRecord
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		return draftPullRequestRecord{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	if record.Number != number || record.Number <= 0 {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned PR #%d, expected #%d", record.Number, number)
	}
	urlNumber, err := githubx.ResourceNumberFromURL(record.URL, "pull")
	if err != nil || urlNumber != number {
		return draftPullRequestRecord{}, fmt.Errorf("gh pr view returned invalid URL for PR #%d", number)
	}
	return record, nil
}

func draftPullRequestBody(body, executor string) string {
	content := strings.TrimRight(normalizeNewlines(body), "\n")
	if content != "" {
		content += "\n\n"
	}
	return fmt.Sprintf("%s<!-- GIA:DRAFT-PR:START -->\nexecutor: %s\nstate: PENDING_USER_APPROVAL\n<!-- GIA:DRAFT-PR:END -->\n", content, executor)
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

func normalizeBodyForComparison(value string) string {
	return strings.TrimRight(normalizeNewlines(value), "\n")
}

func Status(ctx context.Context, repo string) (StatusResult, error) {
	abs, err := filepath.Abs(repo)
	if err != nil {
		return StatusResult{}, err
	}
	b, err := gitx.Branch(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	h, err := gitx.Head(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	clean, err := gitx.IsClean(ctx, repo)
	if err != nil {
		return StatusResult{}, err
	}
	wt, err := gitx.Run(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{abs, b, h, clean, wt}, nil
}

func RecordFeedback(ctx context.Context, repo, category, message string, pr int) (FeedbackResult, error) {
	dir := filepath.Join(repo, ".gia", "feedback")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FeedbackResult{}, err
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(dir, name)
	data := map[string]interface{}{"time": time.Now().UTC().Format(time.RFC3339Nano), "category": category, "message": message, "pr": pr, "os": runtime.GOOS, "arch": runtime.GOARCH}
	b, _ := json.MarshalIndent(data, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return FeedbackResult{}, err
	}
	return FeedbackResult{path, category}, nil
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
func sanitize(s string) string {
	r := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	return strings.Trim(r.ReplaceAllString(strings.ToLower(s), "-"), "-")
}
func escapeLine(s string) string { return strings.ReplaceAll(strings.TrimSpace(s), "\n", " ") }
