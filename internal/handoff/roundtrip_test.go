package handoff

import "testing"

func TestCodexToClaudePortableSession(t *testing.T) {
	codex := []byte(`{"timestamp":"2026-08-05T00:00:00Z","type":"session_meta","payload":{"id":"11111111-1111-4111-8111-111111111111","cwd":"/work/repo"}}
{"timestamp":"2026-08-05T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"debug timeout"}}
{"timestamp":"2026-08-05T00:00:02Z","type":"event_msg","payload":{"type":"agent_message","message":"check connection pool"}}
`)
	s, err := FromCodexBytes(codex)
	if err != nil {
		t.Fatal(err)
	}
	claude, id, err := ToClaude(s)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("missing translated session id")
	}
	back, err := FromClaudeBytes(claude)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Conversation) < 3 {
		t.Fatalf("expected preamble and conversation, got %+v", back.Conversation)
	}
}

func TestClaudeToCodexPortableSession(t *testing.T) {
	claude := []byte(`{"type":"user","sessionId":"22222222-2222-4222-8222-222222222222","uuid":"u1","cwd":"/work/repo","timestamp":"2026-08-05T00:00:00Z","message":{"role":"user","content":"debug timeout"}}
{"type":"assistant","sessionId":"22222222-2222-4222-8222-222222222222","uuid":"u2","parentUuid":"u1","cwd":"/work/repo","timestamp":"2026-08-05T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"check connection pool"}]}}
`)
	s, err := FromClaudeBytes(claude)
	if err != nil {
		t.Fatal(err)
	}
	codex, id, err := ToCodex(s)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("missing translated session id")
	}
	back, err := FromCodexBytes(codex)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Conversation) < 3 {
		t.Fatalf("expected preamble and conversation, got %+v", back.Conversation)
	}
}

func TestCodexUsesPrimarySessionMeta(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"primary","cwd":"/primary"}}
{"type":"session_meta","payload":{"id":"historical","cwd":"/historical"}}
{"type":"event_msg","payload":{"type":"user_message","message":"continue"}}
`)
	s, err := FromCodexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.ThreadID != "primary" || s.CWD != "/primary" {
		t.Fatalf("historical metadata replaced primary: %+v", s)
	}
}
