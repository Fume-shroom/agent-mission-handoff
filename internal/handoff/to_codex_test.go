package handoff

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func TestToCodexUsesPortableResumeMetadata(t *testing.T) {
	body, sessionID, err := ToCodex(AgentSession{
		SourceAgent: "claude",
		ThreadID:    "source-session",
		CWD:         "/work/repo",
		Conversation: []Turn{
			{Role: RoleUser, Text: "continue"},
		},
	}, "target-provider")
	if err != nil {
		t.Fatal(err)
	}

	sc := bufio.NewScanner(bytes.NewReader(body))
	if !sc.Scan() {
		t.Fatal("missing session metadata")
	}
	var wrapper struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(sc.Bytes(), &wrapper); err != nil {
		t.Fatal(err)
	}
	if wrapper.Payload["id"] != sessionID || wrapper.Payload["session_id"] != sessionID {
		t.Fatalf("inconsistent Codex identity: %+v", wrapper.Payload)
	}
	if wrapper.Payload["model_provider"] != "target-provider" {
		t.Fatalf("target-local model provider was not injected: %+v", wrapper.Payload)
	}
}
