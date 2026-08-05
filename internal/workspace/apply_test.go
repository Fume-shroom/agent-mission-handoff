package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
)

func TestApplyRestoresPortablePatch(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source)
	write(t, filepath.Join(source, "tracked.txt"), "after\n")
	write(t, filepath.Join(source, "new.txt"), "new\n")
	state, patch, indexPatch, err := Capture(source)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(t.TempDir(), "target")
	cmd := exec.Command("git", "clone", source, target)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, body)
	}
	data := capsule.Data{Workspace: state, WorktreePatch: patch, IndexPatch: indexPatch}
	result, err := Apply(data, ApplyOptions{CWD: target})
	if err != nil {
		t.Fatal(err)
	}
	if result.AlreadyApplied {
		t.Fatal("first apply reported already applied")
	}
	for path, want := range map[string]string{"tracked.txt": "after\n", "new.txt": "new\n"} {
		body, err := os.ReadFile(filepath.Join(target, path))
		if err != nil || normalizeNewlines(string(body)) != want {
			t.Fatalf("%s = %q, %v", path, body, err)
		}
	}
}

func TestApplyRestoresStagedAndUnstagedState(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source)
	write(t, filepath.Join(source, "tracked.txt"), "staged\n")
	runGit(t, source, "add", "tracked.txt")
	write(t, filepath.Join(source, "tracked.txt"), "staged\nunstaged\n")
	state, patch, indexPatch, err := Capture(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexPatch) == 0 || !state.IndexPatchIncluded {
		t.Fatalf("staged patch was not captured: %+v", state)
	}

	target := filepath.Join(t.TempDir(), "target")
	cmd := exec.Command("git", "clone", source, target)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, body)
	}
	result, err := Apply(capsule.Data{Workspace: state, WorktreePatch: patch, IndexPatch: indexPatch}, ApplyOptions{CWD: target})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexRestored {
		t.Fatal("apply did not report restored staged state")
	}
	status := gitText(t, target, "status", "--porcelain=v1")
	if status != "MM tracked.txt\n" {
		t.Fatalf("status = %q, want staged and unstaged changes", status)
	}
	cached := gitText(t, target, "diff", "--cached", "--", "tracked.txt")
	if !strings.Contains(cached, "+staged") || strings.Contains(cached, "+unstaged") {
		t.Fatalf("cached diff does not match source index:\n%s", cached)
	}
	body, err := os.ReadFile(filepath.Join(target, "tracked.txt"))
	if err != nil || normalizeNewlines(string(body)) != "staged\nunstaged\n" {
		t.Fatalf("worktree content = %q, %v", body, err)
	}
}

func TestApplyRestoresIndexOnlyPatchWhenWorktreeMatchesHead(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source)
	write(t, filepath.Join(source, "tracked.txt"), "staged\n")
	runGit(t, source, "add", "tracked.txt")
	write(t, filepath.Join(source, "tracked.txt"), "before\n")

	state, patch, indexPatch, err := Capture(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != 0 || len(indexPatch) == 0 || !state.PatchIncluded || !state.IndexPatchIncluded {
		t.Fatalf("index-only state was not captured: state=%+v worktree=%d index=%d", state, len(patch), len(indexPatch))
	}

	target := filepath.Join(t.TempDir(), "target")
	cmd := exec.Command("git", "clone", source, target)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, body)
	}
	result, err := Apply(capsule.Data{Workspace: state, IndexPatch: indexPatch}, ApplyOptions{CWD: target})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IndexRestored {
		t.Fatal("index-only apply did not report restored staged state")
	}
	if status := gitText(t, target, "status", "--porcelain=v1"); status != "MM tracked.txt\n" {
		t.Fatalf("status = %q, want staged change with worktree reverted to HEAD", status)
	}
	body, err := os.ReadFile(filepath.Join(target, "tracked.txt"))
	if err != nil || normalizeNewlines(string(body)) != "before\n" {
		t.Fatalf("worktree content = %q, %v", body, err)
	}
}

func TestApplyIndexOnlyPatchDoesNotStageDirtyTargetWithoutConsent(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source)
	write(t, filepath.Join(source, "tracked.txt"), "staged\n")
	runGit(t, source, "add", "tracked.txt")
	write(t, filepath.Join(source, "tracked.txt"), "before\n")
	state, patch, indexPatch, err := Capture(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(patch) != 0 || len(indexPatch) == 0 {
		t.Fatalf("expected index-only Capsule, got worktree=%d index=%d", len(patch), len(indexPatch))
	}

	target := filepath.Join(t.TempDir(), "target")
	cmd := exec.Command("git", "clone", source, target)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, body)
	}
	write(t, filepath.Join(target, "tracked.txt"), "staged\n")
	statusBefore := gitText(t, target, "status", "--porcelain=v1")
	contentBefore, err := os.ReadFile(filepath.Join(target, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = Apply(capsule.Data{Workspace: state, IndexPatch: indexPatch}, ApplyOptions{CWD: target})
	if err == nil || !strings.Contains(err.Error(), "destination worktree is not clean") {
		t.Fatalf("dirty index-only apply error = %v", err)
	}
	if statusAfter := gitText(t, target, "status", "--porcelain=v1"); statusAfter != statusBefore {
		t.Fatalf("status changed on rejected apply: before %q, after %q", statusBefore, statusAfter)
	}
	contentAfter, err := os.ReadFile(filepath.Join(target, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contentAfter) != string(contentBefore) {
		t.Fatalf("content changed on rejected apply: before %q, after %q", contentBefore, contentAfter)
	}
}

func gitText(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	body, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, body)
	}
	return string(body)
}

func normalizeNewlines(text string) string {
	return strings.ReplaceAll(text, "\r\n", "\n")
}

func initRepo(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "amh@example.com")
	runGit(t, repo, "config", "user.name", "AMH Test")
	write(t, filepath.Join(repo, "tracked.txt"), "before\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "base")
}
