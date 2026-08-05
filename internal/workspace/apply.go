package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
)

type ApplyOptions struct {
	CWD                string
	AllowDirty         bool
	AllowGitDifference bool
}

type ApplyResult struct {
	CWD            string
	AlreadyApplied bool
	IndexRestored  bool
}

func Apply(data capsule.Data, opts ApplyOptions) (ApplyResult, error) {
	if len(data.WorktreePatch) == 0 && len(data.IndexPatch) == 0 {
		return ApplyResult{}, errors.New("capsule does not contain a portable worktree patch")
	}
	cwd, err := filepath.Abs(opts.CWD)
	if err != nil {
		return ApplyResult{}, err
	}
	if _, err := gitBytes(cwd, "rev-parse", "--git-dir"); err != nil {
		return ApplyResult{}, errors.New("destination is not a Git workspace")
	}
	if data.Workspace.Git != nil && data.Workspace.Git.Commit != "" && !opts.AllowGitDifference {
		head, err := gitOutput(cwd, "rev-parse", "HEAD")
		if err != nil || strings.TrimSpace(head) != data.Workspace.Git.Commit {
			return ApplyResult{}, errors.New("destination HEAD differs from the source; align Git or use --allow-git-difference after review")
		}
	}
	if !opts.AllowDirty {
		status, err := gitBytes(cwd, "status", "--porcelain=v1")
		if err != nil {
			return ApplyResult{}, err
		}
		if len(bytes.TrimSpace(status)) > 0 {
			worktreeApplied := len(data.WorktreePatch) == 0 || patchCommand(cwd, data.WorktreePatch, "apply", "--reverse", "--check", "--binary", "-") == nil
			if worktreeApplied {
				if len(data.IndexPatch) == 0 || patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--reverse", "--check", "--binary", "-") == nil {
					return ApplyResult{CWD: cwd, AlreadyApplied: true}, nil
				}
				// An index-only Capsule cannot distinguish the source worktree state
				// from unrelated destination edits. Never stage those edits without the
				// receiver explicitly opting into dirty-worktree application.
				if len(data.WorktreePatch) == 0 {
					return ApplyResult{}, errors.New("destination worktree is not clean; review it or use --allow-dirty")
				}
				if patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--check", "--binary", "-") == nil {
					if err := patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--binary", "-"); err != nil {
						return ApplyResult{}, fmt.Errorf("restore staged state: %w", err)
					}
					return ApplyResult{CWD: cwd, AlreadyApplied: true, IndexRestored: true}, nil
				}
			}
			return ApplyResult{}, errors.New("destination worktree is not clean; review it or use --allow-dirty")
		}
	}
	if len(data.WorktreePatch) == 0 {
		if err := patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--check", "--binary", "-"); err != nil {
			return ApplyResult{}, fmt.Errorf("staged patch does not apply cleanly: %w", err)
		}
		if err := patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--binary", "-"); err != nil {
			return ApplyResult{}, fmt.Errorf("apply staged patch: %w", err)
		}
		return ApplyResult{CWD: cwd, IndexRestored: true}, nil
	}
	if err := patchCommand(cwd, data.WorktreePatch, "apply", "--check", "--binary", "-"); err != nil {
		return ApplyResult{}, fmt.Errorf("worktree patch does not apply cleanly: %w", err)
	}
	if len(data.IndexPatch) > 0 {
		if err := patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--check", "--binary", "-"); err != nil {
			return ApplyResult{}, fmt.Errorf("staged patch does not apply cleanly: %w", err)
		}
	}
	if err := patchCommand(cwd, data.WorktreePatch, "apply", "--binary", "-"); err != nil {
		return ApplyResult{}, fmt.Errorf("apply worktree patch: %w", err)
	}
	if len(data.IndexPatch) > 0 {
		if err := patchCommand(cwd, data.IndexPatch, "apply", "--cached", "--binary", "-"); err != nil {
			return ApplyResult{}, fmt.Errorf("restore staged state: %w", err)
		}
	}
	return ApplyResult{CWD: cwd, IndexRestored: len(data.IndexPatch) > 0}, nil
}

func patchCommand(cwd string, patch []byte, args ...string) error {
	if len(args) > 0 && args[0] == "apply" {
		args = append([]string{"apply", "--unidiff-zero"}, args[1:]...)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Stdin = bytes.NewReader(patch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
