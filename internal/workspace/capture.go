package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

const maxPatchSize = 16 << 20

func Capture(cwd string) (capsule.Workspace, []byte, []byte, error) {
	absolute, err := filepath.Abs(cwd)
	if err != nil {
		return capsule.Workspace{}, nil, nil, err
	}
	state := capsule.Workspace{CWD: absolute, PathOnly: true}
	root, err := gitOutput(absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return state, nil, nil, nil
	}
	root = strings.TrimSpace(root)
	state.SourceGitRoot = root
	state.Git = captureGitInfo(root)

	status, err := gitBytes(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return state, nil, nil, fmt.Errorf("git status: %w", err)
	}
	parseStatus(status, &state)
	if !state.Dirty {
		return state, nil, nil, nil
	}

	patch, err := gitBytes(root, "diff", "--binary", "--unified=0", "HEAD", "--")
	if err != nil {
		return state, nil, nil, fmt.Errorf("git diff: %w", err)
	}
	if patchExceedsLimit(patch) {
		state.PatchOmission = "worktree patch exceeds the 16 MiB safety limit"
		return state, nil, nil, nil
	}
	indexPatch, err := gitBytes(root, "diff", "--cached", "--binary", "--unified=0", "HEAD", "--")
	if err != nil {
		return state, nil, nil, fmt.Errorf("git staged diff: %w", err)
	}
	if patchExceedsLimit(indexPatch) {
		state.PatchOmission = "staged patch exceeds the 16 MiB safety limit"
		return state, nil, nil, nil
	}
	for _, path := range state.Untracked {
		body, diffErr := gitNoIndex(root, path)
		if diffErr != nil {
			state.PatchOmission = "an untracked file could not be represented as a Git patch"
			return state, nil, nil, nil
		}
		patch = append(patch, body...)
		if patchExceedsLimit(patch) {
			state.PatchOmission = "worktree patch exceeds the 16 MiB safety limit"
			return state, nil, nil, nil
		}
	}
	if len(patch) == 0 && len(indexPatch) == 0 {
		state.PatchOmission = "Git reported changes but produced no portable patch"
		return state, nil, nil, nil
	}
	state.PatchIncluded = true
	state.PathOnly = false
	state.PatchBytes = len(patch)
	if len(indexPatch) > 0 {
		state.IndexPatchIncluded = true
		state.IndexPatchBytes = len(indexPatch)
	}
	return state, patch, indexPatch, nil
}

func patchExceedsLimit(patch []byte) bool {
	return len(patch) > maxPatchSize
}

func captureGitInfo(root string) *handoff.GitInfo {
	info := &handoff.GitInfo{}
	info.Branch, _ = gitOutput(root, "branch", "--show-current")
	info.Commit, _ = gitOutput(root, "rev-parse", "HEAD")
	info.Remote, _ = gitOutput(root, "remote", "get-url", "origin")
	info.Branch = strings.TrimSpace(info.Branch)
	info.Commit = strings.TrimSpace(info.Commit)
	info.Remote = strings.TrimSpace(info.Remote)
	return info
}

func parseStatus(body []byte, state *capsule.Workspace) {
	for _, record := range bytes.Split(body, []byte{0}) {
		if len(record) < 4 {
			continue
		}
		state.Dirty = true
		status := string(record[:2])
		path := string(record[3:])
		if status == "??" {
			state.Untracked = append(state.Untracked, filepath.ToSlash(path))
			continue
		}
		if status[0] != ' ' && status[0] != '?' {
			state.Staged = true
		}
		if status[1] != ' ' && status[1] != '?' {
			state.Unstaged = true
		}
	}
}

func gitNoIndex(root, path string) ([]byte, error) {
	cmd := exec.Command("git", "diff", "--no-index", "--binary", "--unified=0", "--", os.DevNull, path)
	cmd.Dir = root
	body, err := cmd.Output()
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		return nil, err
	}
	return body, nil
}

func gitOutput(root string, args ...string) (string, error) {
	body, err := gitBytes(root, args...)
	return string(body), err
}

func gitBytes(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	return cmd.Output()
}
