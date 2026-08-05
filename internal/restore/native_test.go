package restore

import (
	"bytes"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

func TestNativeCodexForkPreservesToolHistory(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"old","session_id":"old","cwd":"/old"}}
{"type":"session_meta","payload":{"id":"old","session_id":"old","cwd":"/old-again"}}
{"type":"session_meta","payload":{"id":"historical","cwd":"/historical"}}
{"type":"response_item","payload":{"type":"function_call","name":"mcp__logs__search","arguments":"{}"}}
`)
	got, err := NativeFork("codex", raw, "new", "/old", "/new", "continue mission", "target-provider")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"id":"new"`)) || !bytes.Contains(got, []byte(`mcp__logs__search`)) || !bytes.Contains(got, []byte(`continue mission`)) {
		t.Fatalf("native history not preserved: %s", got)
	}
	if !bytes.Contains(got, []byte(`"id":"historical"`)) {
		t.Fatalf("historical session metadata was rewritten: %s", got)
	}
	if bytes.Contains(got, []byte(`"id":"old"`)) || bytes.Count(got, []byte(`"id":"new"`)) != 2 {
		t.Fatalf("repeated primary session metadata was not fully rewritten: %s", got)
	}
	if bytes.Contains(got, []byte(`"session_id":"old"`)) || bytes.Count(got, []byte(`"session_id":"new"`)) != 2 {
		t.Fatalf("Codex resume identity was not fully rewritten: %s", got)
	}
	if bytes.Count(got, []byte(`"model_provider":"target-provider"`)) != 2 {
		t.Fatalf("target-local Codex provider was not injected: %s", got)
	}
	if !bytes.Contains(got, []byte(`"cwd":"/historical"`)) || bytes.Contains(got, []byte(`"cwd":"/old-again"`)) {
		t.Fatalf("session metadata cwd mapping is incorrect: %s", got)
	}
	s, err := handoff.FromCodexBytes(got)
	if err != nil {
		t.Fatal(err)
	}
	if s.ThreadID != "new" || s.CWD != "/new" {
		t.Fatalf("unexpected fork metadata: %+v", s)
	}
}

func TestNativeClaudeForkRewritesSessionAndCWD(t *testing.T) {
	raw := []byte(`{"type":"user","sessionId":"old","uuid":"11111111-1111-4111-8111-111111111111","cwd":"/old","message":{"role":"user","content":"debug"}}
`)
	got, err := NativeFork("claude", raw, "22222222-2222-4222-8222-222222222222", "/old", "/new", "continue mission", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"sessionId":"22222222-2222-4222-8222-222222222222"`)) || !bytes.Contains(got, []byte(`"cwd":"/new"`)) || !bytes.Contains(got, []byte(`continue mission`)) {
		t.Fatalf("native history not preserved: %s", got)
	}
}

func TestNativeCodexForkRemapsTurnContextAndWorldState(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"old","cwd":"/source"}}
{"type":"turn_context","payload":{"cwd":"/launcher","workspace_roots":["/source","/other"]}}
{"type":"world_state","payload":{"state":{"cwd":"/source","path":"/source/pkg/file.go","xml":"<root>/launcher</root>"}}}
`)
	got, err := NativeFork("codex", raw, "new", "/source", "/target", "continue", "openai")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(`"cwd":"/launcher"`)) || bytes.Contains(got, []byte(`"workspace_roots":["/source"`)) {
		t.Fatalf("runtime paths were not remapped: %s", got)
	}
	if !bytes.Contains(got, []byte(`/target/pkg/file.go`)) {
		t.Fatalf("world state path was not remapped: %s", got)
	}
	if bytes.Contains(got, []byte(`/launcher`)) {
		t.Fatalf("historical runtime workspace was not remapped: %s", got)
	}
}
