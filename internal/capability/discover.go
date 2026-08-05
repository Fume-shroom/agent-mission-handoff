package capability

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

var skillPath = regexp.MustCompile(`(?:^|[/\\])([A-Za-z0-9._-]+)[/\\]SKILL\.md`)

func Discover(raw []byte, session handoff.AgentSession) []capsule.Capability {
	found := map[string]capsule.Capability{}
	add := func(c capsule.Capability) {
		key := c.Kind + ":" + c.Name
		if old, ok := found[key]; !ok || c.Confidence > old.Confidence {
			found[key] = c
		}
	}

	for _, turn := range session.Conversation {
		if turn.Role != handoff.RoleTool || turn.Tool == "" {
			continue
		}
		name := normalizeTool(turn.Tool)
		if strings.HasPrefix(name, "mcp:") {
			add(capsule.Capability{Kind: "mcp", Name: strings.TrimPrefix(name, "mcp:"), Detection: "observed", Confidence: 1, Required: true})
		}
	}

	discoverStructured(raw, add)

	out := make([]capsule.Capability, 0, len(found))
	for _, c := range found {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func discoverStructured(raw []byte, add func(capsule.Capability)) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var value any
		if json.Unmarshal(sc.Bytes(), &value) != nil {
			continue
		}
		walk(value, func(name string, input map[string]any) {
			for _, skill := range skillNames(input) {
				add(capsule.Capability{Kind: "skill", Name: skill, Detection: "observed", Confidence: 1, Required: true})
			}
			if name == "Skill" || name == "skill" {
				for _, key := range []string{"skill", "name"} {
					if v, ok := input[key].(string); ok && v != "" {
						add(capsule.Capability{Kind: "skill", Name: v, Detection: "observed", Confidence: 1, Required: true})
					}
				}
			}
			if strings.HasPrefix(name, "mcp__") {
				parts := strings.Split(name, "__")
				if len(parts) >= 2 {
					add(capsule.Capability{Kind: "mcp", Name: parts[1], Detection: "observed", Confidence: 1, Required: true})
				}
			}
			for _, executable := range commandExecutables(name, input) {
				add(capsule.Capability{Kind: "cli", Name: executable, Detection: "observed", Confidence: 0.95, Required: true})
			}
		})
	}
}

func skillNames(value any) []string {
	set := map[string]bool{}
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case string:
			for _, match := range skillPath.FindAllStringSubmatch(x, -1) {
				set[match[1]] = true
			}
		case map[string]any:
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(value)
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func commandExecutables(tool string, input map[string]any) []string {
	if input == nil {
		return nil
	}
	lower := strings.ToLower(tool)
	if !strings.Contains(lower, "exec_command") && lower != "bash" && lower != "shell" {
		return nil
	}
	var command string
	for _, key := range []string{"cmd", "command"} {
		switch value := input[key].(type) {
		case string:
			command = value
		case []any:
			if len(value) > 0 {
				command, _ = value[0].(string)
			}
		}
		if command != "" {
			break
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, segment := range splitShellCommands(command) {
		fields := strings.Fields(strings.TrimSpace(segment))
		if len(fields) == 0 {
			continue
		}
		if strings.Contains(fields[0], "$(") || strings.Contains(fields[0], "`") {
			continue
		}
		for len(fields) > 0 && shellAssignment(fields[0]) {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		executable := strings.Trim(fields[0], "'\"")
		if filepath.IsAbs(executable) && strings.HasPrefix(filepath.Clean(executable), filepath.Clean(os.TempDir())+string(filepath.Separator)) {
			continue
		}
		name := filepathBase(executable)
		switch name {
		case "cd", "printf", "echo", "test", "true", "false", "export", "unset", "for", "while", "until", "if", "then", "else", "elif", "fi", "case", "esac", "do", "done", "function", "{", "}":
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func splitShellCommands(command string) []string {
	var segments []string
	start := 0
	quote := rune(0)
	escaped := false
	runes := []rune(command)
	flush := func(end int) {
		if segment := strings.TrimSpace(string(runes[start:end])); segment != "" {
			segments = append(segments, segment)
		}
	}
	for i, r := range runes {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			continue
		}
		separator := r == ';' || r == '\n' || r == '|'
		if r == '&' && i+1 < len(runes) && runes[i+1] == '&' {
			separator = true
		}
		if separator {
			flush(i)
			if (r == '|' || r == '&') && i+1 < len(runes) && runes[i+1] == r {
				runes[i+1] = ' '
			}
			start = i + 1
		}
	}
	flush(len(runes))
	return segments
}

func shellAssignment(token string) bool {
	key, _, ok := strings.Cut(token, "=")
	if !ok || key == "" {
		return false
	}
	for i, r := range key {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func walk(v any, visit func(string, map[string]any)) {
	switch x := v.(type) {
	case map[string]any:
		name, _ := x["name"].(string)
		input, _ := x["input"].(map[string]any)
		if input == nil {
			if args, ok := x["arguments"].(string); ok {
				_ = json.Unmarshal([]byte(args), &input)
			}
		}
		if name != "" {
			visit(name, input)
		}
		for _, child := range x {
			walk(child, visit)
		}
	case []any:
		for _, child := range x {
			walk(child, visit)
		}
	}
}

func normalizeTool(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.Split(name, "__")
		if len(parts) >= 2 {
			return "mcp:" + parts[1]
		}
	}
	return name
}
