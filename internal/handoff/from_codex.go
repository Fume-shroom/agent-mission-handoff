package handoff

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// codexLine is the on-disk rollout wrapper: {timestamp, type, payload}.
type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// FromCodexRollout reads a Codex rollout JSONL file and extracts the neutral
// AgentSession: visible user/assistant conversation from event_msg lines,
// durable conversation from response_item lines, and bounded tool evidence.
// Equivalent visible/durable message pairs are deduplicated while mixed-format
// history remains in source order. Project context comes from session_meta.
// Parsing is defensive — unknown or malformed lines are skipped, never fatal.
func FromCodexRollout(path string) (AgentSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return AgentSession{}, err
	}
	defer f.Close()
	return fromCodexReader(f)
}

// FromCodexBytes extracts the neutral AgentSession from in-memory Codex rollout
// bytes (e.g. a bundle entry), so callers need not write a temp file.
func FromCodexBytes(b []byte) (AgentSession, error) {
	return fromCodexReader(bytes.NewReader(b))
}

func fromCodexReader(r io.Reader) (AgentSession, error) {
	session := AgentSession{Format: IRFormat, SourceAgent: "codex"}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	toolNames := map[string]string{}
	var prose codexProseDeduper
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var cl codexLine
		if json.Unmarshal([]byte(line), &cl) != nil {
			continue
		}
		switch cl.Type {
		case "session_meta":
			// Forked/imported rollouts can contain historical session_meta records.
			// The first one identifies the rollout being read; later records belong
			// to carried history and must not replace the primary identity.
			if session.ThreadID == "" {
				applyCodexMeta(&session, cl)
			}
		case "event_msg":
			consumeCodexEvent(&session, cl.Payload, &prose)
		case "response_item":
			consumeCodexResponseItem(&session, cl.Payload, toolNames, &prose)
		}
	}
	if err := sc.Err(); err != nil {
		return session, err
	}
	return session, nil
}

type codexProseDeduper struct {
	source    string
	role      Role
	text      string
	turnIndex int
}

func (d *codexProseDeduper) add(s *AgentSession, source string, role Role, text string) {
	text = clip(text, maxTurnText)
	if text == "" {
		return
	}
	if d.source != "" && d.source != source && d.role == role && d.text == text && d.turnIndex == len(s.Conversation)-1 {
		return
	}
	s.Conversation = append(s.Conversation, Turn{Role: role, Text: text})
	d.source = source
	d.role = role
	d.text = text
	d.turnIndex = len(s.Conversation) - 1
}

func applyCodexMeta(s *AgentSession, cl codexLine) {
	var p struct {
		ID        string          `json:"id"`
		CWD       string          `json:"cwd"`
		Timestamp string          `json:"timestamp"`
		Git       json.RawMessage `json:"git"`
	}
	if json.Unmarshal(cl.Payload, &p) != nil {
		return
	}
	if p.ID != "" {
		s.ThreadID = p.ID
	}
	if p.CWD != "" {
		s.CWD = p.CWD
	}
	if p.Timestamp != "" {
		s.CreatedAt = p.Timestamp
	} else if cl.Timestamp != "" {
		s.CreatedAt = cl.Timestamp
	}
	if g := decodeGit(p.Git); g != nil {
		s.Git = g
	}
}

// consumeCodexEvent handles visible event_msg payloads used by older rollouts.
func consumeCodexEvent(s *AgentSession, payload json.RawMessage, prose *codexProseDeduper) {
	var p struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return
	}
	switch p.Type {
	case "user_message":
		prose.add(s, "event", RoleUser, stripMarker(p.Message))
	case "agent_message":
		prose.add(s, "event", RoleAssistant, p.Message)
	}
}

// consumeCodexResponseItem carries bounded tool calls and results as historical
// text. They are never replayed as executable calls in the destination Agent.
func consumeCodexResponseItem(s *AgentSession, payload json.RawMessage, toolNames map[string]string, prose *codexProseDeduper) {
	var p struct {
		Type      string          `json:"type"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"`
		CallID    string          `json:"call_id"`
		Output    string          `json:"output"`
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return
	}
	switch p.Type {
	case "message":
		if p.Role != string(RoleUser) && p.Role != string(RoleAssistant) {
			return
		}
		text := codexMessageText(p.Content)
		if p.Role == string(RoleUser) {
			text = stripMarker(text)
		}
		prose.add(s, "response", Role(p.Role), text)
	case "function_call":
		name := p.Name
		if name == "" {
			name = "tool"
		}
		if p.CallID != "" {
			toolNames[p.CallID] = name
		}
		text := "ran " + name
		if args := briefArgs(p.Arguments); args != "" {
			text += ": " + args
		}
		s.addTurn(RoleTool, name, text)
	case "function_call_output":
		name := toolNames[p.CallID]
		if name == "" {
			name = "tool"
		}
		if output := strings.TrimSpace(p.Output); output != "" {
			s.addTurn(RoleTool, name, "result from "+name+": "+clip(output, 2000))
		}
	case "web_search_call":
		s.addTurn(RoleTool, "web_search", "performed a web search")
	}
}

func codexMessageText(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case "input_text", "output_text", "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// briefArgs extracts a short, human-readable snippet from a tool-call arguments
// JSON blob (best-effort): a "command"/"cmd" string, else a truncated raw form.
func briefArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) == nil {
		for _, k := range []string{"command", "cmd", "query", "path", "file_path"} {
			if v, ok := m[k]; ok {
				return clip(fmt.Sprint(stringifyArg(v)), 200)
			}
		}
	}
	return clip(raw, 200)
}

func stringifyArg(v any) string {
	switch t := v.(type) {
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprint(t)
	}
}

func stripMarker(msg string) string {
	const marker = "USER_MESSAGE_BEGIN"
	if i := strings.Index(msg, marker); i != -1 {
		msg = msg[i+len(marker):]
	}
	return strings.TrimSpace(msg)
}

// decodeGit pulls branch/commit/remote from a session_meta git object, trying a
// few plausible key names (Codex's exact field names have varied).
func decodeGit(raw json.RawMessage) *GitInfo {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	g := &GitInfo{
		Branch: firstString(m, "branch", "git_branch"),
		Commit: firstString(m, "sha", "commit_hash", "commit", "git_sha"),
		Remote: firstString(m, "origin_url", "repository_url", "remote_url", "git_origin_url"),
	}
	if g.Branch == "" && g.Commit == "" && g.Remote == "" {
		return nil
	}
	return g
}

func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
	}
	return ""
}
