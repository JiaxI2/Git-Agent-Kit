// Package planstore persists immutable plans and their single-use apply lease.
package planstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/config"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
	"github.com/JiaxI2/git-isolated-agent-kit/internal/gitx"
)

type Store struct {
	Repository string
	ConfigPath string
}

type applyAttempt struct {
	PlanID    domain.PlanID `json:"planId"`
	StartedAt time.Time     `json:"startedAt"`
}

type applyResult struct {
	Result     domain.PlanApplyResult `json:"result"`
	FinishedAt time.Time              `json:"finishedAt"`
}

func New(repository, configPath string) *Store {
	return &Store{Repository: repository, ConfigPath: configPath}
}

func (s *Store) Snapshot(ctx context.Context, repository string) (domain.PlanSnapshot, error) {
	repo, err := canonicalPath(repository)
	if err != nil {
		return domain.PlanSnapshot{}, fmt.Errorf("resolve plan repository: %w", err)
	}
	configured, err := canonicalPath(s.Repository)
	if err != nil {
		return domain.PlanSnapshot{}, fmt.Errorf("resolve configured repository: %w", err)
	}
	if !samePath(repo, configured) {
		return domain.PlanSnapshot{}, fmt.Errorf("plan repository %s does not match configured repository %s", repo, configured)
	}
	head, err := gitx.Head(ctx, repo)
	if err != nil {
		return domain.PlanSnapshot{}, err
	}
	path, err := s.configFile(repo)
	if err != nil {
		return domain.PlanSnapshot{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return domain.PlanSnapshot{}, fmt.Errorf("read plan config %s: %w", path, err)
	}
	digest := sha256.Sum256(content)
	snapshot := domain.PlanSnapshot{Repository: repo, Head: head, ConfigDigest: "sha256:" + hex.EncodeToString(digest[:])}
	if err := snapshot.Validate(); err != nil {
		return domain.PlanSnapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) Create(ctx context.Context, plan domain.Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	repo, err := canonicalPath(s.Repository)
	if err != nil {
		return fmt.Errorf("resolve configured repository: %w", err)
	}
	if !samePath(plan.Repository, repo) {
		return fmt.Errorf("plan repository %s does not match configured repository %s", plan.Repository, repo)
	}
	paths, err := s.paths(ctx, plan.ID)
	if err != nil {
		return err
	}
	if err := publishJSON(paths.plan, plan); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("persist plan: %w", err)
		}
		existing, readErr := readPlan(paths.plan)
		if readErr != nil {
			return readErr
		}
		existingJSON, _ := json.Marshal(existing)
		planJSON, _ := json.Marshal(plan)
		if !bytes.Equal(existingJSON, planJSON) {
			return errors.New("content-addressed plan already exists with different content")
		}
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id domain.PlanID) (domain.PlanRecord, error) {
	if err := validatePlanID(id); err != nil {
		return domain.PlanRecord{}, err
	}
	paths, err := s.paths(ctx, id)
	if err != nil {
		return domain.PlanRecord{}, err
	}
	plan, err := readPlan(paths.plan)
	if err != nil {
		return domain.PlanRecord{}, err
	}
	status := domain.PlanStatus{PlanID: id, State: domain.PlanPlanned}
	var completed applyResult
	switch err := readJSON(paths.result, &completed); {
	case err == nil:
		if completed.Result.PlanID != id || (completed.Result.State != domain.PlanApplied && completed.Result.State != domain.PlanFailed) {
			return domain.PlanRecord{}, errors.New("stored plan result is invalid")
		}
		status.State = completed.Result.State
		status.FinishedAt = completed.FinishedAt
		status.Error = completed.Result.Error
	case !errors.Is(err, os.ErrNotExist):
		return domain.PlanRecord{}, fmt.Errorf("read plan result: %w", err)
	}
	var attempt applyAttempt
	switch err := readJSON(paths.attempt, &attempt); {
	case err == nil:
		if attempt.PlanID != id || attempt.StartedAt.IsZero() {
			return domain.PlanRecord{}, errors.New("stored plan attempt is invalid")
		}
		status.StartedAt = attempt.StartedAt
		if status.State == domain.PlanPlanned {
			status.State = domain.PlanApplying
		}
	case !errors.Is(err, os.ErrNotExist):
		return domain.PlanRecord{}, fmt.Errorf("read plan attempt: %w", err)
	}
	return domain.PlanRecord{Plan: plan, Status: status}, nil
}

func (s *Store) BeginApply(ctx context.Context, id domain.PlanID, started time.Time) error {
	if started.IsZero() {
		return errors.New("plan apply start time is required")
	}
	record, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if record.Status.State != domain.PlanPlanned {
		return fmt.Errorf("plan %s has already been attempted", id)
	}
	paths, err := s.paths(ctx, id)
	if err != nil {
		return err
	}
	if err := publishJSON(paths.attempt, applyAttempt{PlanID: id, StartedAt: started}); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("plan %s has already been attempted", id)
		}
		return fmt.Errorf("acquire plan apply lease: %w", err)
	}
	return nil
}

func (s *Store) FinishApply(ctx context.Context, result domain.PlanApplyResult, finished time.Time) error {
	if err := validatePlanID(result.PlanID); err != nil {
		return err
	}
	if result.State != domain.PlanApplied && result.State != domain.PlanFailed {
		return fmt.Errorf("unsupported plan result state %q", result.State)
	}
	if finished.IsZero() {
		return errors.New("plan apply finish time is required")
	}
	paths, err := s.paths(ctx, result.PlanID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(paths.attempt); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("plan apply lease is missing")
		}
		return fmt.Errorf("inspect plan apply lease: %w", err)
	}
	if err := publishJSON(paths.result, applyResult{Result: result, FinishedAt: finished}); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("plan %s already has a result", result.PlanID)
		}
		return fmt.Errorf("persist plan result: %w", err)
	}
	return nil
}

type storePaths struct {
	plan    string
	attempt string
	result  string
}

func (s *Store) paths(ctx context.Context, id domain.PlanID) (storePaths, error) {
	if err := validatePlanID(id); err != nil {
		return storePaths{}, err
	}
	common, err := gitx.Run(ctx, s.Repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return storePaths{}, err
	}
	root := filepath.Join(filepath.Clean(common), "gia")
	return storePaths{
		plan:    filepath.Join(root, "plans", string(id)+".json"),
		attempt: filepath.Join(root, "attempts", string(id)+".json"),
		result:  filepath.Join(root, "results", string(id)+".json"),
	}, nil
}

func (s *Store) configFile(repo string) (string, error) {
	if _, err := config.Load(repo, s.ConfigPath); err != nil {
		return "", err
	}
	if strings.TrimSpace(s.ConfigPath) != "" {
		path := s.ConfigPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(repo, path)
		}
		return filepath.Clean(path), nil
	}
	found, err := config.ExistingPaths(repo)
	if err != nil {
		return "", err
	}
	if len(found) != 1 {
		return "", fmt.Errorf("expected one validated GIA config, found %d", len(found))
	}
	return found[0], nil
}

func readPlan(path string) (domain.Plan, error) {
	var plan domain.Plan
	if err := readJSON(path, &plan); err != nil {
		return domain.Plan{}, fmt.Errorf("read plan: %w", err)
	}
	if err := plan.Validate(); err != nil {
		return domain.Plan{}, fmt.Errorf("stored plan is invalid: %w", err)
	}
	return plan, nil
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	switch err := decoder.Decode(&extra); {
	case errors.Is(err, io.EOF):
		return nil
	case err == nil:
		return errors.New("multiple JSON values are not allowed")
	default:
		return err
	}
}

func publishJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func validatePlanID(id domain.PlanID) error {
	value := string(id)
	if len(value) != sha256.Size*2 {
		return errors.New("plan id must be a SHA-256 digest")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return errors.New("plan id must be a lowercase SHA-256 digest")
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("repository path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
