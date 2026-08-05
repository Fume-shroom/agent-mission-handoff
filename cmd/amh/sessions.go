package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

const maxWorkspaceFallbackSessions = 32
const recentWorkspaceTailBytes = 8 << 20

func codexModelProvider(overrideHome string) (string, error) {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".codex")
	if overrideHome != "" {
		root = overrideHome
	}
	body, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if errors.Is(err, os.ErrNotExist) {
		return "openai", nil
	}
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "model_provider" {
			continue
		}
		provider, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil || provider == "" {
			return "", fmt.Errorf("invalid model_provider in %s", filepath.Join(root, "config.toml"))
		}
		return provider, nil
	}
	return "openai", nil
}

type sessionQuery struct {
	Agent string
	Query string
	Home  string
	CWD   string
}

func detectAgent(flagName string) (string, error) {
	if explicit := strings.ToLower(strings.TrimSpace(os.Getenv("AMH_AGENT"))); explicit != "" {
		if explicit != "codex" && explicit != "claude" {
			return "", errors.New("AMH_AGENT must be codex or claude")
		}
		return explicit, nil
	}
	for _, key := range []string{"CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"} {
		if os.Getenv(key) != "" {
			return "claude", nil
		}
	}
	for _, key := range []string{"CODEX_THREAD_ID", "CODEX_CI"} {
		if os.Getenv(key) != "" {
			return "codex", nil
		}
	}
	return "", fmt.Errorf("cannot detect the current coding agent; rerun with %s codex|claude or set AMH_AGENT", flagName)
}

func detectSourceAgent(cwd, overrideHome string) (string, error) {
	if detected, err := detectAgent("--agent"); err == nil {
		return detected, nil
	}
	type candidate struct {
		agent string
		path  string
		time  time.Time
	}
	var candidates []candidate
	for _, agent := range []string{"codex", "claude"} {
		path, err := resolveSession(sessionQuery{Agent: agent, Query: "current", Home: overrideHome, CWD: cwd})
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err == nil {
			candidates = append(candidates, candidate{agent: agent, path: path, time: info.ModTime()})
		}
	}
	if len(candidates) == 1 {
		return candidates[0].agent, nil
	}
	if len(candidates) > 1 {
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].time.After(candidates[j].time) })
		if !candidates[0].time.Equal(candidates[1].time) {
			return candidates[0].agent, nil
		}
	}
	return "", errors.New("cannot identify the current coding Agent for this workspace; rerun with --agent codex|claude")
}

func detectTargetAgent() (string, error) {
	if detected, err := detectAgent("--to"); err == nil {
		return detected, nil
	}
	available := map[string]bool{}
	for _, agent := range []string{"codex", "claude"} {
		_, err := exec.LookPath(agent)
		available[agent] = err == nil
	}
	if available["codex"] != available["claude"] {
		if available["codex"] {
			return "codex", nil
		}
		return "claude", nil
	}
	if available["codex"] && available["claude"] {
		return "", errors.New("both Codex and Claude Code are installed; rerun with --to codex|claude")
	}
	return "", errors.New("cannot select the destination coding Agent; rerun with --to codex|claude")
}

func resolveSession(opts sessionQuery) (string, error) {
	if info, err := os.Stat(opts.Query); err == nil && !info.IsDir() {
		return filepath.Abs(opts.Query)
	}
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".codex")
	if opts.Agent == "claude" {
		root = filepath.Join(home, ".claude")
	}
	if opts.Home != "" {
		root = opts.Home
	}
	searchRoot := filepath.Join(root, "sessions")
	if opts.Agent == "claude" {
		searchRoot = filepath.Join(root, "projects")
		if opts.Query == "current" && opts.CWD != "" {
			searchRoot = filepath.Join(searchRoot, claudeProjectKey(filepath.Clean(opts.CWD)))
		}
	}
	query := opts.Query
	if query == "" {
		query = "current"
	}
	currentRequested := query == "current"
	if query == "current" {
		if opts.Agent == "codex" && os.Getenv("CODEX_THREAD_ID") != "" {
			query = os.Getenv("CODEX_THREAD_ID")
		} else if opts.Agent == "claude" && os.Getenv("CLAUDE_CODE_SESSION_ID") != "" {
			query = os.Getenv("CLAUDE_CODE_SESSION_ID")
		}
	}
	var matches []string
	err := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if query != "current" && query != "latest" && !strings.Contains(filepath.Base(path), query) {
			return nil
		}
		if query != "latest" && query != "current" {
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var session handoff.AgentSession
			var parseErr error
			if opts.Agent == "codex" {
				session, parseErr = handoff.FromCodexBytes(body)
			} else {
				session, parseErr = handoff.FromClaudeBytes(body)
			}
			if parseErr != nil {
				return nil
			}
			if query != "current" && session.ThreadID != query {
				return nil
			}
			if currentRequested && opts.CWD != "" && normalizedPath(session.CWD) != normalizedPath(opts.CWD) && sessionLastWorkspace(body) != normalizedPath(opts.CWD) {
				return nil
			}
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		if opts.Query == "current" {
			return "", fmt.Errorf("no current %s session found for workspace %q; use --session ID|PATH", opts.Agent, opts.CWD)
		}
		return "", fmt.Errorf("no %s session matches %q", opts.Agent, query)
	}
	sort.Slice(matches, func(i, j int) bool {
		a, _ := os.Stat(matches[i])
		b, _ := os.Stat(matches[j])
		return a.ModTime().After(b.ModTime())
	})
	if currentRequested && query == "current" && opts.CWD != "" {
		wanted := normalizedPath(opts.CWD)
		limit := len(matches)
		if limit > maxWorkspaceFallbackSessions {
			limit = maxWorkspaceFallbackSessions
		}
		for _, path := range matches[:limit] {
			if sessionInitialWorkspace(path, opts.Agent) == wanted || sessionRecentWorkspaceFile(path) == wanted {
				return path, nil
			}
		}
		for _, path := range matches[limit:] {
			if sessionInitialWorkspace(path, opts.Agent) == wanted {
				return path, nil
			}
		}
		return "", fmt.Errorf("no current %s session found for workspace %q among the %d most recent fallback candidates; use --session ID|PATH", opts.Agent, opts.CWD, limit)
	}
	return matches[0], nil
}

func sessionLastWorkspace(raw []byte) string {
	return sessionLastWorkspaceScanner(bufio.NewScanner(bytes.NewReader(raw)))
}

func sessionRecentWorkspaceFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - recentWorkspaceTailBytes
	if start < 0 {
		start = 0
	}
	body := make([]byte, info.Size()-start)
	n, err := f.ReadAt(body, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return ""
	}
	body = body[:n]
	if start > 0 {
		if newline := bytes.IndexByte(body, '\n'); newline >= 0 {
			body = body[newline+1:]
		} else {
			return ""
		}
	}
	return sessionLastWorkspace(body)
}

func sessionLastWorkspaceScanner(sc *bufio.Scanner) string {
	latest := ""
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		var value any
		if json.Unmarshal(sc.Bytes(), &value) == nil {
			if workspace := workspaceInValue(value); workspace != "" {
				latest = normalizedPath(workspace)
			}
		}
	}
	return latest
}

func sessionInitialWorkspace(path, agent string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for lines := 0; sc.Scan() && lines < 32; lines++ {
		var value map[string]any
		if json.Unmarshal(sc.Bytes(), &value) != nil {
			continue
		}
		if agent == "claude" {
			if cwd, _ := value["cwd"].(string); cwd != "" {
				return normalizedPath(cwd)
			}
			continue
		}
		if value["type"] != "session_meta" {
			continue
		}
		if payload, ok := value["payload"].(map[string]any); ok {
			if cwd, _ := payload["cwd"].(string); cwd != "" {
				return normalizedPath(cwd)
			}
		}
	}
	return ""
}

func workspaceInValue(value any) string {
	switch current := value.(type) {
	case map[string]any:
		for _, key := range []string{"workdir", "cwd"} {
			if child, ok := current[key].(string); ok && child != "" {
				return child
			}
		}
		for key, child := range current {
			if key == "arguments" {
				if encoded, ok := child.(string); ok {
					var arguments any
					if json.Unmarshal([]byte(encoded), &arguments) == nil {
						if workspace := workspaceInValue(arguments); workspace != "" {
							return workspace
						}
					}
				}
			}
			if workspace := workspaceInValue(child); workspace != "" {
				return workspace
			}
		}
	case []any:
		for i := len(current) - 1; i >= 0; i-- {
			if workspace := workspaceInValue(current[i]); workspace != "" {
				return workspace
			}
		}
	}
	return ""
}

func normalizedPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func destinationPath(agent, overrideHome, cwd, sessionID string) (string, error) {
	home, _ := os.UserHomeDir()
	if agent == "codex" {
		root := filepath.Join(home, ".codex")
		if overrideHome != "" {
			root = overrideHome
		}
		now := time.Now()
		name := fmt.Sprintf("rollout-%s-%s.jsonl", now.Format("2006-01-02T15-04-05"), sessionID)
		return filepath.Join(root, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"), name), nil
	}
	root := filepath.Join(home, ".claude")
	if overrideHome != "" {
		root = overrideHome
	}
	project := claudeProjectKey(filepath.Clean(cwd))
	return filepath.Join(root, "projects", project, sessionID+".jsonl"), nil
}

func claudeProjectKey(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	for _, r := range path {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
