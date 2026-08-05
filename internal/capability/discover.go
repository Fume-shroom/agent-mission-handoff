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

var skillPath = regexp.MustCompile(`[A-Za-z0-9_./~:\\-]*[/\\]([A-Za-z0-9._-]+)[/\\]SKILL\.md`)
var executableName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
var heredocStart = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z0-9_]+)['"]?`)

func Discover(raw []byte, session handoff.AgentSession) []capsule.Capability {
	return DiscoverWithOptions(raw, session, Options{})
}

type Options struct {
	Agent string
	CWD   string
	Home  string
}

func DiscoverWithOptions(raw []byte, session handoff.AgentSession, opts Options) []capsule.Capability {
	found := map[string]capsule.Capability{}
	add := func(c capsule.Capability) {
		key := c.Kind + ":" + c.Name
		if old, ok := found[key]; ok {
			found[key] = mergeCapability(old, c)
		} else {
			found[key] = c
		}
	}

	for _, turn := range session.Conversation {
		if turn.Role != handoff.RoleTool || turn.Tool == "" {
			continue
		}
		name := normalizeTool(turn.Tool)
		if strings.HasPrefix(name, "mcp:") {
			add(capsule.Capability{Kind: "mcp", Name: strings.TrimPrefix(name, "mcp:"), Detection: "observed", Confidence: 1})
		}
	}

	discoverStructured(raw, add)

	out := make([]capsule.Capability, 0, len(found))
	for _, c := range found {
		enriched := enrich(c, opts)
		if enriched.Kind == "cli" && enriched.Source == "" {
			continue
		}
		if enriched.Kind == "skill" && c.Detection == "observed-path" && enriched.Source == "" {
			continue
		}
		out = append(out, enriched)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func mergeCapability(old, current capsule.Capability) capsule.Capability {
	if current.Confidence > old.Confidence {
		old.Confidence = current.Confidence
		old.Detection = current.Detection
	}
	if current.Source != "" && (old.Source == "" || sourceScore(current.Source) > sourceScore(old.Source)) {
		old.Source = current.Source
	}
	if current.Version != "" {
		old.Version = current.Version
	}
	if current.Digest != "" {
		old.Digest = current.Digest
	}
	old.Required = old.Required || current.Required
	return old
}

func sourceScore(path string) int {
	if path == "" {
		return 0
	}
	if _, err := os.Stat(path); err == nil {
		return 2
	}
	if absolute, err := filepath.Abs(path); err == nil {
		if _, err := os.Stat(absolute); err == nil {
			return 2
		}
	}
	return 1
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
			for skill, path := range skillReferences(name, input) {
				add(capsule.Capability{Kind: "skill", Name: skill, Source: path, Detection: "observed-path", Confidence: 1})
			}
			if name == "Skill" || name == "skill" {
				for _, key := range []string{"skill", "name"} {
					if v, ok := input[key].(string); ok && v != "" {
						add(capsule.Capability{Kind: "skill", Name: v, Detection: "observed", Confidence: 1})
					}
				}
			}
			if strings.HasPrefix(name, "mcp__") {
				parts := strings.Split(name, "__")
				if len(parts) >= 2 {
					add(capsule.Capability{Kind: "mcp", Name: parts[1], Detection: "observed", Confidence: 1})
				}
			}
			for _, executable := range commandExecutables(name, input) {
				add(capsule.Capability{Kind: "cli", Name: executable, Detection: "observed", Confidence: 0.95})
			}
		})
	}
}

func skillReferences(tool string, input map[string]any) map[string]string {
	set := map[string]string{}
	if isShellTool(tool) {
		if command := commandText(input); command != "" {
			collectSkillReferences(stripHeredocBodies(command), set)
		}
		return set
	}
	collectSkillReferences(input, set)
	return set
}

func collectSkillReferences(value any, set map[string]string) {
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case string:
			for _, match := range skillPath.FindAllStringSubmatch(x, -1) {
				set[match[1]] = strings.TrimSpace(match[0])
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
}

func commandExecutables(tool string, input map[string]any) []string {
	if input == nil {
		return nil
	}
	if !isShellTool(tool) {
		return nil
	}
	command := commandText(input)
	if command == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, segment := range splitShellCommands(stripHeredocBodies(command)) {
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
		if !executableName.MatchString(name) {
			continue
		}
		if baselineCommand(name) {
			continue
		}
		switch name {
		case "cd", "printf", "echo", "test", "true", "false", "export", "unset", "for", "while", "until", "if", "then", "else", "elif", "fi", "case", "esac", "do", "done", "function", "{", "}", "apply_patch", "command", "builtin", "source", ".":
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func isShellTool(tool string) bool {
	lower := strings.ToLower(tool)
	return strings.Contains(lower, "exec_command") || lower == "bash" || lower == "shell"
}

func commandText(input map[string]any) string {
	for _, key := range []string{"cmd", "command"} {
		switch value := input[key].(type) {
		case string:
			return value
		case []any:
			if len(value) > 0 {
				command, _ := value[0].(string)
				return command
			}
		}
	}
	return ""
}

func baselineCommand(name string) bool {
	switch name {
	case "awk", "basename", "cat", "chmod", "chown", "cmp", "cp", "cut", "date", "dd", "dirname", "env", "exit", "find", "grep", "head", "kill", "less", "ln", "local", "ls", "mkdir", "mktemp", "mv", "nl", "open", "paste", "pgrep", "pkill", "ps", "pwd", "readlink", "rm", "rmdir", "sed", "set", "sh", "sleep", "sort", "tail", "tee", "test", "time", "touch", "tr", "trap", "uname", "uniq", "wc", "which", "xargs":
		return true
	}
	return false
}

func stripHeredocBodies(command string) string {
	lines := strings.Split(command, "\n")
	kept := make([]string, 0, len(lines))
	var delimiters []string
	for _, line := range lines {
		if len(delimiters) > 0 {
			candidate := strings.TrimSpace(line)
			if candidate == delimiters[0] {
				delimiters = delimiters[1:]
			}
			continue
		}
		kept = append(kept, line)
		for _, match := range heredocStart.FindAllStringSubmatch(line, -1) {
			if len(match) > 1 {
				delimiters = append(delimiters, match[1])
			}
		}
	}
	return strings.Join(kept, "\n")
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
