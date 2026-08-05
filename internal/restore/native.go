package restore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// NativeFork preserves an agent's complete native history while assigning a new
// session id, remapping cwd, and appending the Mission Handoff context.
func NativeFork(agent string, raw []byte, newID, cwd, context, targetModelProvider string) ([]byte, error) {
	switch agent {
	case "codex":
		return nativeCodex(raw, newID, cwd, context, targetModelProvider)
	case "claude":
		return nativeClaude(raw, newID, cwd, context)
	default:
		return nil, fmt.Errorf("unsupported native agent %q", agent)
	}
}

func nativeCodex(raw []byte, newID, cwd, context, targetModelProvider string) ([]byte, error) {
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	primaryID := ""
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		var wrapper map[string]any
		if json.Unmarshal(line, &wrapper) == nil && wrapper["type"] == "session_meta" {
			if payload, ok := wrapper["payload"].(map[string]any); ok {
				id, _ := payload["id"].(string)
				if primaryID == "" && id != "" {
					primaryID = id
				}
				if id == primaryID && primaryID != "" {
					payload["id"] = newID
					if sessionID, ok := payload["session_id"].(string); ok && sessionID == primaryID {
						payload["session_id"] = newID
					}
					if targetModelProvider != "" {
						payload["model_provider"] = targetModelProvider
					}
					if cwd != "" {
						payload["cwd"] = cwd
					}
					line, _ = json.Marshal(wrapper)
				}
			}
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if primaryID == "" {
		return nil, errors.New("Codex session_meta not found")
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	appendJSONLine(&out, map[string]any{"timestamp": ts, "type": "event_msg", "payload": map[string]any{"type": "user_message", "message": context}})
	appendJSONLine(&out, map[string]any{"timestamp": ts, "type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": context}}}})
	return out.Bytes(), nil
}

func nativeClaude(raw []byte, newID, cwd, context string) ([]byte, error) {
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	found := false
	parent := ""
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		var record map[string]any
		if json.Unmarshal(line, &record) == nil {
			if _, ok := record["sessionId"]; ok {
				record["sessionId"], found = newID, true
			}
			if cwd != "" {
				if _, ok := record["cwd"]; ok {
					record["cwd"] = cwd
				}
			}
			if uuid, ok := record["uuid"].(string); ok && uuid != "" {
				parent = uuid
			}
			line, _ = json.Marshal(record)
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("Claude sessionId not found")
	}
	uuid := deterministicLineID(newID, context)
	record := map[string]any{
		"type": "user", "sessionId": newID, "uuid": uuid, "cwd": cwd,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "version": "amh-handoff",
		"userType": "external", "isSidechain": false, "entrypoint": "amh",
		"message": map[string]any{"role": "user", "content": context},
	}
	if parent == "" {
		record["parentUuid"] = nil
	} else {
		record["parentUuid"] = parent
	}
	appendJSONLine(&out, record)
	return out.Bytes(), nil
}

func appendJSONLine(out *bytes.Buffer, value any) {
	body, _ := json.Marshal(value)
	out.Write(body)
	out.WriteByte('\n')
}

func deterministicLineID(sessionID, text string) string {
	// UUID shape is sufficient for Claude transcript linkage; the session id
	// makes collisions between restored sessions impossible in practice.
	clean := strings.ReplaceAll(sessionID, "-", "")
	if len(clean) < 32 {
		clean += "00000000000000000000000000000000"
	}
	return clean[:8] + "-" + clean[8:12] + "-4" + clean[13:16] + "-8" + clean[17:20] + "-" + clean[20:32]
}
