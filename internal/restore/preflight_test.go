package restore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestFormatChecksStripsTerminalControlCharacters(t *testing.T) {
	got := FormatChecks([]Check{{Kind: "skill", Name: "bad\x1b[2Jname", Status: "missing", Detail: "line\nrewrite"}})
	if strings.ContainsAny(strings.TrimSuffix(got, "\n"), "\x1b\n\r") {
		t.Fatalf("unsafe control characters in output: %q", got)
	}
}
