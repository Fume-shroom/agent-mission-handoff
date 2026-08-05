package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capability"
	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
)

func TestPreflightFindsProjectLocalSkill(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, ".agents", "skills", "incident-debug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := capsule.Data{Capabilities: []capsule.Capability{{Kind: "skill", Name: "incident-debug"}}}
	checks := Preflight(data, cwd)
	for _, check := range checks {
		if check.Kind == "skill" && check.Name == "incident-debug" {
			if check.Status != "ready" {
				t.Fatalf("project skill not found: %+v", check)
			}
			return
		}
	}
	t.Fatal("skill check missing")
}

func TestPreflightUsesTargetAgentHome(t *testing.T) {
	cwd := t.TempDir()
	codexHome := filepath.Join(t.TempDir(), ".codex")
	claudeHome := filepath.Join(t.TempDir(), ".claude")
	skill := filepath.Join(codexHome, "skills", "codex-only", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("# skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := capsule.Data{Capabilities: []capsule.Capability{{Kind: "skill", Name: "codex-only", Required: true}}}
	checks := PreflightFor(data, Options{CWD: cwd, Target: "claude", Home: claudeHome})
	for _, check := range checks {
		if check.Kind == "skill" && check.Name == "codex-only" && check.Status != "missing" {
			t.Fatalf("Claude preflight used Codex home: %+v", check)
		}
	}
}

func TestPreflightReportsDifferentSkillDigest(t *testing.T) {
	cwd := t.TempDir()
	destination := filepath.Join(cwd, ".agents", "skills", "incident-debug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("# destination skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(source, []byte("# source skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, version, digest := capability.DescribeFile(source)
	data := capsule.Data{Capabilities: []capsule.Capability{{Kind: "skill", Name: "incident-debug", Version: version, Digest: digest}}}
	checks := Preflight(data, cwd)
	for _, check := range checks {
		if check.Kind == "skill" && check.Name == "incident-debug" {
			if check.Status != "different" || !strings.Contains(check.Detail, "digest differs") {
				t.Fatalf("different Skill identity was not reported: %+v", check)
			}
			return
		}
	}
	t.Fatal("skill check missing")
}

func TestCLIIdentityPrefersPortableVersionOverBinaryDigest(t *testing.T) {
	exact := Check{Kind: "cli", Name: "example", Status: "ready"}
	applyIdentityCheck(&exact, capsule.Capability{Kind: "cli", Name: "example", Version: "1.2.3", Digest: "sha256:exact"}, "", "sha256:exact")
	if exact.Status != "ready" {
		t.Fatalf("exact CLI digest was rejected without a parseable version: %+v", exact)
	}

	check := Check{Kind: "cli", Name: "example", Status: "ready"}
	applyIdentityCheck(&check, capsule.Capability{Kind: "cli", Name: "example", Version: "1.2.3", Digest: "sha256:source"}, "1.2.3", "sha256:different-architecture")
	if check.Status != "ready" {
		t.Fatalf("same CLI version was rejected because the binary digest differed: %+v", check)
	}

	applyIdentityCheck(&check, capsule.Capability{Kind: "cli", Name: "example", Version: "1.2.3", Digest: "sha256:source"}, "1.2.4", "sha256:destination")
	if check.Status != "different" || !strings.Contains(check.Detail, "version differs") {
		t.Fatalf("different CLI version was not reported: %+v", check)
	}
}

func TestFormatChecksStripsTerminalControlCharacters(t *testing.T) {
	got := FormatChecks([]Check{{Kind: "skill", Name: "bad\x1b[2Jname", Status: "missing", Detail: "line\nrewrite"}})
	if strings.ContainsAny(strings.TrimSuffix(got, "\n"), "\x1b\n\r") {
		t.Fatalf("unsafe control characters in output: %q", got)
	}
}

func TestPreflightReportsPortableWorktreePatch(t *testing.T) {
	cwd := t.TempDir()
	checks := PreflightFor(capsule.Data{
		Workspace:     capsule.Workspace{Dirty: true, PatchIncluded: true},
		WorktreePatch: []byte("patch"),
	}, Options{CWD: cwd})
	for _, check := range checks {
		if check.Kind == "workspace" && check.Name == "workspace-patch" {
			if check.Status != "deferred" || check.Required {
				t.Fatalf("unexpected patch check: %+v", check)
			}
			return
		}
	}
	t.Fatal("worktree patch check missing")
}
