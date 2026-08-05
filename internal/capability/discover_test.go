package capability

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

func TestDiscoverSkillsAndMCP(t *testing.T) {
	raw := []byte(`{"type":"response_item","payload":{"type":"function_call","name":"mcp__logs__search","arguments":"{}"}}
{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"sed -n 1,80p /home/me/.codex/skills/incident-debug/SKILL.md\"}"}}
`)
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
		{"git status && custom-cli verify | tee result", []string{"git", "custom-cli", "tee"}},
		{"cd repo && go test ./...", []string{"go"}},
		{"rg 'foo|bar' .", []string{"rg"}},
	}
	for _, tt := range tests {
		got := commandExecutables("exec_command", map[string]any{"cmd": tt.command})
		if !slices.Equal(got, tt.want) {
			t.Errorf("commandExecutables(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}
