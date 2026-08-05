package handoff

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestCodexCarriesBoundedToolResultEvidence(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"primary","cwd":"/work"}}
{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call-1","arguments":"{\"cmd\":\"go test ./...\"}"}}
{"type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"FAIL: timeout regression reproduced"}}
`)
	session, err := FromCodexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Conversation) != 2 || session.Conversation[1].Tool != "exec_command" || session.Conversation[1].Text != "result from exec_command: FAIL: timeout regression reproduced" {
		t.Fatalf("tool result evidence was lost: %+v", session.Conversation)
	}
}

func TestCodexRecoversDurableMessagesWithoutEventRecords(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"primary","cwd":"/work"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"debug timeout"}]}}
{"type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"call-1","arguments":"{\"cmd\":\"go test ./...\"}"}}
{"type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":"FAIL reproduced"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"the failure is reproducible"}]}}
`)
	session, err := FromCodexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Conversation) != 4 {
		t.Fatalf("durable response history was not recovered: %+v", session.Conversation)
	}
	if session.Conversation[0].Role != RoleUser || session.Conversation[3].Role != RoleAssistant {
		t.Fatalf("durable prose order was not preserved: %+v", session.Conversation)
	}
}

func TestCodexDoesNotDuplicateEventAndDurableMessages(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"primary","cwd":"/work"}}
{"type":"event_msg","payload":{"type":"user_message","message":"debug timeout"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"debug timeout"}]}}
{"type":"event_msg","payload":{"type":"agent_message","message":"check the pool"}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"check the pool"}]}}
`)
	session, err := FromCodexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Conversation) != 2 {
		t.Fatalf("duplicate visible and durable messages were retained: %+v", session.Conversation)
	}
}

func TestCodexMergesMixedEventAndDurableHistoryInSourceOrder(t *testing.T) {
	raw := []byte(`{"type":"session_meta","payload":{"id":"primary","cwd":"/work"}}
{"type":"event_msg","payload":{"type":"user_message","message":"old question"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"old answer"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"new question"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"new answer"}]}}
`)
	session, err := FromCodexBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []Turn{
		{Role: RoleUser, Text: "old question"},
		{Role: RoleAssistant, Text: "old answer"},
		{Role: RoleUser, Text: "new question"},
		{Role: RoleAssistant, Text: "new answer"},
	}
	if len(session.Conversation) != len(want) {
		t.Fatalf("mixed-format history was lost: %+v", session.Conversation)
	}
	for i := range want {
		if session.Conversation[i] != want[i] {
			t.Fatalf("turn %d = %+v, want %+v", i, session.Conversation[i], want[i])
		}
	}
}

func TestClaudeCarriesToolResultEvidence(t *testing.T) {
	raw := []byte(`{"type":"assistant","sessionId":"session","cwd":"/work","message":{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"user","sessionId":"session","cwd":"/work","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"FAIL: timeout regression reproduced"}]}}
`)
	session, err := FromClaudeBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Conversation) != 2 || session.Conversation[1].Tool != "Bash" || session.Conversation[1].Text != "result from Bash: FAIL: timeout regression reproduced" {
		t.Fatalf("tool result evidence was lost: %+v", session.Conversation)
	}
}

func TestCurrentCodexFixtureCompatibility(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "codex-0.142.5.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := FromCodexBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	if session.ThreadID != "11111111-1111-4111-8111-111111111111" || session.CWD != "/work/source" {
		t.Fatalf("unexpected Codex metadata: %+v", session)
	}
	if len(session.Conversation) != 3 || session.Conversation[1].Role != RoleTool || session.Conversation[1].Tool != "exec_command" {
		t.Fatalf("unexpected Codex conversation: %+v", session.Conversation)
	}
}

func TestCurrentClaudeFixtureCompatibility(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "claude-2.1.183.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	session, err := FromClaudeBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	if session.ThreadID != "22222222-2222-4222-8222-222222222222" || session.CWD != "/work/source" {
		t.Fatalf("unexpected Claude metadata: %+v", session)
	}
	if len(session.Conversation) != 3 || session.Conversation[1].Role != RoleTool || session.Conversation[1].Tool != "Bash" {
		t.Fatalf("unexpected Claude conversation: %+v", session.Conversation)
	}
}
