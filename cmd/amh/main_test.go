package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
)

func TestDetectAgentPrefersExplicitOverride(t *testing.T) {
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CLAUDECODE", "1")
	got, err := detectAgent("--agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "codex" {
		t.Fatalf("agent = %q, want codex", got)
	}
}

func TestDetectAgentPrefersClaudeOverInheritedCodexEnvironment(t *testing.T) {
	t.Setenv("AMH_AGENT", "")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
	t.Setenv("CODEX_THREAD_ID", "inherited-thread")
	got, err := detectAgent("--agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude" {
		t.Fatalf("agent = %q, want claude", got)
	}
}

func TestDetectAgentUsesCodexThread(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	got, err := detectAgent("--agent")
	if err != nil {
		t.Fatal(err)
	}
	if got != "codex" {
		t.Fatalf("agent = %q, want codex", got)
	}
}

func TestDetectAgentReturnsActionableError(t *testing.T) {
	clearAgentEnv(t)
	_, err := detectAgent("--to")
	if err == nil || !strings.Contains(err.Error(), "--to") {
		t.Fatalf("error = %v, want explicit override guidance", err)
	}
}

func TestResolveCodexSessionUsesCurrentThread(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	want := writeCodexSession(t, home, "current-thread", cwd, "current")
	writeCodexSession(t, home, "other-thread", cwd, "newer")
	t.Setenv("CODEX_THREAD_ID", "current-thread")

	got, err := resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session = %q, want %q", got, want)
	}
}

func TestResolveCodexSessionScopesLatestToWorkspace(t *testing.T) {
	clearAgentEnv(t)
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo-a")
	other := filepath.Join(t.TempDir(), "repo-b")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	want := writeCodexSession(t, home, "repo-a-thread", cwd, "older")
	otherPath := writeCodexSession(t, home, "repo-b-thread", other, "newer")
	older := mustStat(t, want).ModTime().Add(-2)
	newer := mustStat(t, otherPath).ModTime().Add(2)
	if err := os.Chtimes(want, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(otherPath, newer, newer); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session = %q, want workspace-scoped %q", got, want)
	}
}

func TestResolveCurrentCodexThreadRejectsDifferentWorkspace(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo-a")
	other := filepath.Join(t.TempDir(), "repo-b")
	writeCodexSession(t, home, "current-thread", other, "current")
	t.Setenv("CODEX_THREAD_ID", "current-thread")

	_, err := resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: cwd})
	if err == nil || !strings.Contains(err.Error(), "no current codex session") {
		t.Fatalf("error = %v, want workspace mismatch", err)
	}
}

func TestResolveCurrentCodexThreadAcceptsObservedToolWorkspace(t *testing.T) {
	home := t.TempDir()
	initial := filepath.Join(t.TempDir(), "launcher")
	cwd := filepath.Join(t.TempDir(), "worktree")
	path := writeCodexSession(t, home, "current-thread", initial, "current")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	args, err := json.Marshal(map[string]string{"cmd": "go test ./...", "workdir": cwd})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":` + jsonString(string(args)) + `}}` + "\n")
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatalf("append session: %v, close: %v", err, closeErr)
	}
	t.Setenv("CODEX_THREAD_ID", "current-thread")

	got, err := resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("session = %q, want %q", got, path)
	}
}

func TestResolveCurrentCodexThreadUsesLastObservedWorkspace(t *testing.T) {
	home := t.TempDir()
	repoA := filepath.Join(t.TempDir(), "repo-a")
	repoB := filepath.Join(t.TempDir(), "repo-b")
	path := writeCodexSession(t, home, "current-thread", repoB, "current")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{repoA, repoB} {
		args, err := json.Marshal(map[string]string{"cmd": "pwd", "workdir": cwd})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":` + jsonString(string(args)) + `}}` + "\n")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_THREAD_ID", "current-thread")

	_, err = resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: repoA})
	if err == nil {
		t.Fatal("expected historical workspace touch to be rejected")
	}
}

func TestResolveExplicitSessionRequiresExactID(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeCodexSession(t, home, "session-123-extra", cwd, "newer-session-123")
	want := writeCodexSession(t, home, "session-123", cwd, "older-session-123")

	got, err := resolveSession(sessionQuery{Agent: "codex", Query: "session-123", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session = %q, want exact id %q", got, want)
	}
}

func TestResolveClaudeSessionScopesToProjectDirectory(t *testing.T) {
	clearAgentEnv(t)
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	wantDir := filepath.Join(home, "projects", claudeProjectKey(cwd))
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wantDir, "current.jsonl")
	writeFile(t, want, claudeSession("current", cwd))
	otherDir := filepath.Join(home, "projects", claudeProjectKey(cwd+"-other"))
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(otherDir, "newer.jsonl"), claudeSession("newer", cwd+"-other"))

	got, err := resolveSession(sessionQuery{Agent: "claude", Query: "current", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("session = %q, want %q", got, want)
	}
}

func TestContinueDefaultsToDestinationWorkingDirectory(t *testing.T) {
	destination := t.TempDir()
	home := t.TempDir()
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, nil)
	withWorkingDirectory(t, destination)

	result, err := restoreMission(restoreOptions{Target: "claude", CapsulePath: capsulePath, Home: home, AllowMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if normalizedPath(result.CWD) != normalizedPath(destination) {
		t.Fatalf("cwd = %q, want %q", result.CWD, destination)
	}
	if !strings.Contains(result.Destination, claudeProjectKey(destination)) {
		t.Fatalf("destination = %q, want target workspace key", result.Destination)
	}
}

func TestContinueStopsForMissingRequiredCapability(t *testing.T) {
	destination := t.TempDir()
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, []capsule.Capability{{Kind: "cli", Name: "amh-command-that-does-not-exist", Required: true}})

	_, err := restoreMission(restoreOptions{Target: "claude", CapsulePath: capsulePath, Home: t.TempDir(), CWD: destination})
	if err == nil || !strings.Contains(err.Error(), "--allow-missing") {
		t.Fatalf("error = %v, want confirmation guidance", err)
	}

	result, err := restoreMission(restoreOptions{Target: "claude", CapsulePath: capsulePath, Home: t.TempDir(), CWD: destination, AllowMissing: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" {
		t.Fatal("missing restored session id")
	}
}

func TestContinueCannotBypassMissingWorkspace(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, nil)
	missing := filepath.Join(t.TempDir(), "missing")

	_, err := restoreMission(restoreOptions{Target: "claude", CapsulePath: capsulePath, Home: t.TempDir(), CWD: missing, AllowMissing: true})
	if err == nil || !strings.Contains(err.Error(), "workspace is not ready") {
		t.Fatalf("error = %v, want non-bypassable workspace failure", err)
	}
}

func TestCrossAgentRestoreEndsWithSafetyContext(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, nil)
	home := t.TempDir()
	cwd := t.TempDir()

	result, err := restoreMission(restoreOptions{Target: "codex", CapsulePath: capsulePath, Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Destination)
	if err != nil {
		t.Fatal(err)
	}
	session, err := handoff.FromCodexBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	last := session.Conversation[len(session.Conversation)-1]
	if last.Role != handoff.RoleUser || !strings.Contains(last.Text, "untrusted historical context") {
		t.Fatalf("last turn does not reassert the safety boundary: %+v", last)
	}
}

func TestPackAndContinueShortestPath(t *testing.T) {
	home := t.TempDir()
	sourceCWD := t.TempDir()
	destinationCWD := t.TempDir()
	writeCodexSession(t, filepath.Join(home, ".codex"), "current-thread", sourceCWD, "current")
	setTestHome(t, home)
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	withWorkingDirectory(t, sourceCWD)

	if err := runPack(nil); err != nil {
		t.Fatal(err)
	}
	capsulePath := filepath.Join(destinationCWD, "mission.amh")
	if err := os.Rename(filepath.Join(sourceCWD, "mission.amh"), capsulePath); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, destinationCWD)
	t.Setenv("AMH_AGENT", "claude")
	if err := runContinue([]string{"mission.amh"}); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeProjectKeyMatchesClaudeCode(t *testing.T) {
	got := claudeProjectKey(`/Users/bytedance/.codex/worktrees/agent_mission-handoff`)
	want := `-Users-bytedance--codex-worktrees-agent-mission-handoff`
	if got != want {
		t.Fatalf("claude project key = %q, want %q", got, want)
	}
}

func TestClaudeProjectKeyHandlesWindowsPaths(t *testing.T) {
	got := claudeProjectKey(`C:\work\agent.mission`)
	want := `C--work-agent-mission`
	if got != want {
		t.Fatalf("claude project key = %q, want %q", got, want)
	}
}

func TestCodexModelProviderUsesTargetHome(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = \"target-provider\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := codexModelProvider(home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "target-provider" {
		t.Fatalf("model provider = %q, want target-provider", got)
	}
}

func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"AMH_AGENT", "CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CODEX_THREAD_ID", "CODEX_CI"} {
		t.Setenv(key, "")
	}
}

func writeCodexSession(t *testing.T, home, id, cwd, name string) string {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "08", "05", name+"-"+id+".jsonl")
	body := `{"type":"session_meta","payload":{"id":` + jsonString(id) + `,"session_id":` + jsonString(id) + `,"cwd":` + jsonString(cwd) + `}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","message":"debug timeout"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"agent_message","message":"continue checking logs"}}` + "\n"
	writeFile(t, path, []byte(body))
	return path
}

func claudeSession(id, cwd string) []byte {
	return []byte(`{"type":"user","sessionId":` + jsonString(id) + `,"uuid":"11111111-1111-4111-8111-111111111111","cwd":` + jsonString(cwd) + `,"message":{"role":"user","content":"debug"}}` + "\n")
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func setTestHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func writeCapsule(t *testing.T, path string, capabilities []capsule.Capability) {
	t.Helper()
	raw := claudeSession("source-session", "/source/repo")
	data := capsule.Data{
		Manifest:     capsule.Manifest{Format: capsule.Format, CapsuleID: "cap-1", SourceAgent: "claude", SourceSessionID: "source-session"},
		Mission:      capsule.MissionCheckpoint{Objective: "debug", Status: "in_progress"},
		Capabilities: capabilities,
		Workspace:    capsule.Workspace{CWD: "/source/repo", PathOnly: true},
		Session:      handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "claude", ThreadID: "source-session", CWD: "/source/repo", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
		RawSession:   raw,
	}
	if err := capsule.Write(path, data); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func withWorkingDirectory(t *testing.T, path string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func captureOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), runErr
}
