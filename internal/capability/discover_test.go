package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

func TestDiscoverSkillsAndMCP(t *testing.T) {
	skill := filepath.Join(t.TempDir(), ".codex", "skills", "incident-debug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]string{"cmd": "sed -n 1,80p " + skill})
	raw := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"mcp__logs__search","arguments":"{}"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":` + string(mustJSON(string(arguments))) + `}}` + "\n")
	session := handoff.AgentSession{Conversation: []handoff.Turn{{Role: handoff.RoleTool, Tool: "mcp__logs__search", Text: "searched"}}}
	got := Discover(raw, session)
	want := map[string]bool{"mcp:logs": false, "skill:incident-debug": false}
	for _, c := range got {
		key := c.Kind + ":" + c.Name
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing %s in %+v", key, got)
		}
	}
}

func TestDiscoverIgnoresSkillPathsInsideHeredoc(t *testing.T) {
	raw := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"apply_patch <<'PATCH'\\n+/fake/incident-debug/SKILL.md\\nPATCH\"}"}}` + "\n")
	got := Discover(raw, handoff.AgentSession{})
	if len(got) != 0 {
		t.Fatalf("heredoc produced capabilities: %+v", got)
	}
}

func TestCommandExecutableSkipsShellAssignmentsAndControlSyntax(t *testing.T) {
	tests := []struct {
		command string
		want    []string
	}{
		{"GOOS=darwin GOARCH=arm64 go build ./cmd/amh", []string{"go"}},
		{"tmpdir=$(mktemp -d /tmp/amh.XXXXXX)\ngo test ./...", []string{"go"}},
		{"sid=$(cat session-id)\nclaude --resume $sid", []string{"claude"}},
		{"for file in *.jsonl; do printf '%s\\n' $file; done", nil},
		{"capsule=$(cat /tmp/path)", nil},
		{filepath.Join(os.TempDir(), "amh-spike-bin") + " inspect mission.amh", nil},
		{"git status && custom-cli verify | tee result", []string{"git", "custom-cli"}},
		{"cd repo && go test ./...", []string{"go"}},
		{"rg 'foo|bar' .", []string{"rg"}},
		{"apply_patch <<'PATCH'\n*** Begin Patch\n+go test ./...\n+if !ok; then echo bad; fi\n*** End Patch\nPATCH", nil},
		{"cat <<EOF > config.txt\nsecret=value\ngh auth status\nEOF\ngit status", []string{"git"}},
		{"!invalid && +markdown", nil},
	}
	for _, tt := range tests {
		got := commandExecutables("exec_command", map[string]any{"cmd": tt.command})
		if !slices.Equal(got, tt.want) {
			t.Errorf("commandExecutables(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func mustJSON(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

func TestDiscoverCLIsAreAdvisory(t *testing.T) {
	dir := t.TempDir()
	goPath := filepath.Join(dir, "go")
	if err := os.WriteFile(goPath, []byte("fake"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	raw := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"go test ./...\"}"}}` + "\n")
	got := Discover(raw, handoff.AgentSession{})
	if len(got) != 1 || got[0].Name != "go" || got[0].Required {
		t.Fatalf("unexpected CLI capability: %+v", got)
	}
}

func TestDescribeExecutableRecognizesNPMGlobalPackage(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "lib", "node_modules", "@openai", "codex")
	executable := filepath.Join(packageDir, "bin", "codex.js")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "package.json"), []byte(`{"version":"9.8.7"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source, version, digest := describeExecutable(executable)
	if source != "npm:@openai/codex" || version != "9.8.7" || digest == "" {
		t.Fatalf("unexpected executable metadata: %q %q %q", source, version, digest)
	}
}

func TestEnrichFindsDirectlyInvokedSkillSource(t *testing.T) {
	home := t.TempDir()
	skill := filepath.Join(home, "skills", "incident-debug", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("# skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := enrich(capsule.Capability{Kind: "skill", Name: "incident-debug"}, Options{Agent: "codex", Home: home})
	if got.Source == "" || got.Digest == "" {
		t.Fatalf("skill metadata was not enriched: %+v", got)
	}
}
