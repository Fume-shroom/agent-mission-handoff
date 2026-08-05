package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureIncludesTrackedAndUntrackedChanges(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "amh@example.com")
	runGit(t, repo, "config", "user.name", "AMH Test")
	write(t, filepath.Join(repo, "tracked.txt"), "before\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "base")
	write(t, filepath.Join(repo, "tracked.txt"), "after\n")
	write(t, filepath.Join(repo, "new.txt"), "new\n")

	state, patch, _, err := Capture(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Dirty || !state.Unstaged || !state.PatchIncluded {
		t.Fatalf("unexpected state: %+v", state)
	}
	if len(state.Untracked) != 1 || state.Untracked[0] != "new.txt" {
		t.Fatalf("untracked = %v", state.Untracked)
	}
	if !strings.Contains(string(patch), "tracked.txt") || !strings.Contains(string(patch), "new.txt") {
		t.Fatalf("patch does not contain all changes:\n%s", patch)
	}
}

func TestPatchSafetyLimitIncludesInitialTrackedDiff(t *testing.T) {
	if !patchExceedsLimit(make([]byte, maxPatchSize+1)) {
		t.Fatal("tracked diff above the safety limit was accepted")
	}
	if patchExceedsLimit(make([]byte, maxPatchSize)) {
		t.Fatal("patch at the safety limit was rejected")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, body)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
