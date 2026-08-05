package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
	"github.com/Fume-shroom/agent-mission-handoff/internal/restore"
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

func TestDetectSourceAgentUsesNewestWorkspaceSessionWithoutAgentEnvironment(t *testing.T) {
	clearAgentEnv(t)
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	codex := writeCodexSession(t, home, "codex-thread", cwd, "codex")
	claudeDir := filepath.Join(home, "projects", claudeProjectKey(cwd))
	claude := filepath.Join(claudeDir, "claude-thread.jsonl")
	writeFile(t, claude, claudeSession("claude-thread", cwd))
	older := mustStat(t, claude).ModTime().Add(-time.Minute)
	if err := os.Chtimes(claude, older, older); err != nil {
		t.Fatal(err)
	}
	newer := mustStat(t, codex).ModTime().Add(time.Minute)
	if err := os.Chtimes(codex, newer, newer); err != nil {
		t.Fatal(err)
	}

	got, err := detectSourceAgent(cwd, home)
	if err != nil {
		t.Fatal(err)
	}
	if got != "codex" {
		t.Fatalf("agent = %q, want codex", got)
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

func TestResolveCurrentPrefersNewerObservedWorkspaceOverOlderInitialMatch(t *testing.T) {
	clearAgentEnv(t)
	home := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "worktree")
	launcher := filepath.Join(t.TempDir(), "launcher")
	older := writeCodexSession(t, home, "older-thread", cwd, "older")
	newer := writeCodexSession(t, home, "newer-thread", launcher, "newer")
	f, err := os.OpenFile(newer, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	args, err := json.Marshal(map[string]string{"cmd": "go test ./...", "workdir": cwd})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":` + jsonString(string(args)) + `}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-time.Minute)
	newTime := time.Now()
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSession(sessionQuery{Agent: "codex", Query: "current", Home: home, CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if got != newer {
		t.Fatalf("session = %q, want newer observed-workspace session %q", got, newer)
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
	if result.Mission.RawSession != nil {
		t.Fatal("restore result retained the raw native session")
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

func TestRequiredCapabilityIdentityDifferenceNeedsConfirmation(t *testing.T) {
	checks := []restore.Check{{Kind: "skill", Name: "incident-debug", Status: "different", Required: true}}
	if got := confirmableMissing(checks); len(got) != 1 {
		t.Fatalf("required identity difference was not treated as confirmable: %+v", got)
	}
}

func TestRestoreStopsForRequiredDifferenceByDefault(t *testing.T) {
	destination := t.TempDir()
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, []capsule.Capability{{Kind: "cli", Name: "amh-command-that-does-not-exist", Required: true}})

	_, err := captureOutput(t, func() error {
		return runRestore([]string{"--to", "claude", "--cwd", destination, "--home", t.TempDir(), "--dry-run", capsulePath})
	})
	if err == nil || !strings.Contains(err.Error(), "--allow-missing") {
		t.Fatalf("error = %v, want explicit confirmation guidance", err)
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

func TestInspectJSONIncludesPortableHistoryWithoutRawSession(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, nil)

	out, err := captureOutput(t, func() error {
		return runInspect([]string{"--json", capsulePath})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("inspect output is not valid JSON: %s", out)
	}
	if !strings.Contains(out, `"conversation"`) || !strings.Contains(out, `"text": "debug"`) {
		t.Fatalf("portable conversation is missing: %s", out)
	}
	if strings.Contains(out, `"uuid"`) {
		t.Fatalf("raw native session leaked into inspect JSON: %s", out)
	}
}

func TestMissionContextRequiresBriefingAndConfirmation(t *testing.T) {
	data := capsule.Data{
		Manifest: capsule.Manifest{SourceAgent: "codex"},
		Mission: capsule.MissionCheckpoint{
			Objective:         "debug timeout",
			Status:            "in_progress",
			CurrentSummary:    "pool exhaustion reproduced",
			Completed:         []string{"captured logs"},
			CurrentHypotheses: []string{"connection leak"},
			NextActions:       []string{"inspect pool metrics"},
			InterruptedAction: "tail production logs",
		},
		Workspace: capsule.Workspace{Dirty: true, Staged: true, PatchIncluded: true, IndexPatchOmission: "staged metadata was omitted"},
		Session:   handoff.AgentSession{Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "debug"}}},
	}
	context := missionContext(data, "/tmp/mission.amh", []restore.Check{{Kind: "skill", Name: "incident-debug", Status: "different", Detail: "content digest differs"}})
	for _, want := range []string{
		"Read the complete imported conversation",
		"first response must be a concise Mission Brief",
		"historical context (latest request, key decisions, evidence, and interruption point)",
		"captured logs",
		"connection leak",
		"inspect pool metrics",
		"tail production logs",
		"asking whether to continue with the proposed next action",
		"Do not run tools or change files until the user explicitly confirms",
		"amh apply \"/tmp/mission.amh\"",
		"Source agent: codex",
		"Portable history: 1 turns",
		"Destination environment differences observed during restore",
		"incident-debug",
		"staged metadata was omitted",
	} {
		if !strings.Contains(context, want) {
			t.Fatalf("mission context missing %q:\n%s", want, context)
		}
	}
}

func TestMissionBriefShowsHistoryContextAndGaps(t *testing.T) {
	result := restoreResult{
		Target: "codex",
		Mission: capsule.Data{
			Manifest: capsule.Manifest{SourceAgent: "codex", SourceSessionID: "source-session"},
			Mission: capsule.MissionCheckpoint{
				Objective:         "debug timeout",
				Status:            "in_progress",
				CurrentSummary:    "pool exhaustion reproduced",
				Completed:         []string{"captured logs"},
				CurrentHypotheses: []string{"connection leak"},
				NextActions:       []string{"inspect pool metrics"},
			},
			Session: handoff.AgentSession{Conversation: []handoff.Turn{
				{Role: handoff.RoleUser, Text: "please debug the timeout"},
				{Role: handoff.RoleTool, Tool: "logs", Text: "large internal tool output"},
				{Role: handoff.RoleAssistant, Text: "the pool appears exhausted"},
			}},
		},
		Checks: []restore.Check{{Kind: "skill", Name: "incident-debug", Status: "missing", Detail: "install locally", Required: true}},
	}

	out, err := captureOutput(t, func() error {
		printMissionBrief(result)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Mission Brief",
		"Source: codex session source-session",
		"History: 3 turns (1 user, 1 assistant, 1 tool)",
		"User: please debug the timeout",
		"Assistant: the pool appears exhausted",
		"captured logs",
		"connection leak",
		"inspect pool metrics",
		"skill: incident-debug (install locally)",
		"Restored history is available in the writable codex session",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("brief missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "large internal tool output") {
		t.Fatalf("brief should not dump tool output:\n%s", out)
	}
}

func TestCheckpointUsesLatestSubstantialRequestAndSections(t *testing.T) {
	got := checkpoint(handoff.AgentSession{Conversation: []handoff.Turn{
		{Role: handoff.RoleUser, Text: "initial research request that is long enough to be considered the old objective"},
		{Role: handoff.RoleAssistant, Text: "old summary"},
		{Role: handoff.RoleUser, Text: "修复这八个问题，然后完整测试并同步文档"},
		{Role: handoff.RoleAssistant, Text: "Completed:\n- fixed parsing\nRisks:\n- real Agent compatibility\nNext:\n- run E2E"},
		{Role: handoff.RoleUser, Text: "<subagent_notification>{\"status\":\"completed\",\"report\":\"long internal review output that must not become the mission objective\"}"},
	}})
	if !strings.Contains(got.Objective, "八个问题") {
		t.Fatalf("objective = %q", got.Objective)
	}
	if len(got.Completed) != 1 || len(got.CurrentHypotheses) != 1 || len(got.NextActions) != 1 {
		t.Fatalf("checkpoint sections not extracted: %+v", got)
	}
}

func TestSyntheticMissionContextIsNotAUserObjective(t *testing.T) {
	for _, text := range []string{
		"<subagent_notification>{}",
		"<in-app-browser-context source=\"ambient-ui-state\">",
		"# AGENTS.md instructions\n<INSTRUCTIONS>internal</INSTRUCTIONS>",
		"Another language model started to solve this problem and produced a summary",
	} {
		if substantialRequest(text) {
			t.Fatalf("synthetic context was accepted as a mission request: %q", text)
		}
	}
	wrapped := `<codex_internal_context source="goal"><objective>修复恢复流程并完整测试</objective></codex_internal_context>`
	if got := missionRequestText(wrapped); got != "修复恢复流程并完整测试" {
		t.Fatalf("wrapped user goal was not extracted: %q", got)
	}
}

func TestCheckpointPrefersExplicitRequestOverLaterWrappedGoal(t *testing.T) {
	got := checkpoint(handoff.AgentSession{Conversation: []handoff.Turn{
		{Role: handoff.RoleUser, Text: "review 下这个工具，在功能、用户体验上还有哪些可以提升"},
		{Role: handoff.RoleAssistant, Text: "我会先检查当前实现。"},
		{Role: handoff.RoleUser, Text: `<codex_internal_context source="goal"><objective>修复之前的八个问题</objective></codex_internal_context>`},
	}})
	if got.Objective != "review 下这个工具，在功能、用户体验上还有哪些可以提升" {
		t.Fatalf("wrapped historical goal replaced the explicit request: %q", got.Objective)
	}

	fallback := checkpoint(handoff.AgentSession{Conversation: []handoff.Turn{{
		Role: handoff.RoleUser,
		Text: `<codex_internal_context source="goal"><objective>修复恢复流程并完整测试</objective></codex_internal_context>`,
	}}})
	if fallback.Objective != "修复恢复流程并完整测试" {
		t.Fatalf("wrapped goal was not used as fallback: %q", fallback.Objective)
	}
}

func TestCrossAgentRestoreEndsWithSafetyContext(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, []capsule.Capability{{Kind: "skill", Name: "missing-incident-skill"}})
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
	if last.Role != handoff.RoleUser || !strings.Contains(last.Text, "untrusted historical context") || !strings.Contains(last.Text, "asking whether to continue with the proposed next action") || !strings.Contains(last.Text, "missing-incident-skill") {
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
	out, err := captureOutput(t, func() error {
		return runContinue([]string{"mission.amh"})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Mission Brief", "History: 2 turns", "Mission restored in safe-semantic mode. Continue with: claude --resume"} {
		if !strings.Contains(out, want) {
			t.Fatalf("continue output missing %q:\n%s", want, out)
		}
	}
}

func TestPackAndApplyRestoresDirtyWorktreeToIndependentClone(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "init")
	runGitCommand(t, source, "config", "user.email", "amh@example.com")
	runGitCommand(t, source, "config", "user.name", "AMH Test")
	writeFile(t, filepath.Join(source, "tracked.txt"), []byte("before\n"))
	runGitCommand(t, source, "add", "tracked.txt")
	runGitCommand(t, source, "commit", "-m", "base")
	writeFile(t, filepath.Join(source, "tracked.txt"), []byte("after\n"))
	writeFile(t, filepath.Join(source, "new.txt"), []byte("new\n"))
	writeCodexSession(t, filepath.Join(home, ".codex"), "current-thread", source, "current")
	setTestHome(t, home)
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	if err := runPack([]string{"--cwd", source, "-o", capsulePath}); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "clone", source, target)
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone source: %v\n%s", err, body)
	}
	if err := runApply([]string{"--cwd", target, capsulePath}); err != nil {
		t.Fatal(err)
	}
	if err := runApply([]string{"--cwd", target, capsulePath}); err != nil {
		t.Fatalf("second apply should detect the existing patch: %v", err)
	}
	for path, want := range map[string]string{"tracked.txt": "after\n", "new.txt": "new\n"} {
		body, err := os.ReadFile(filepath.Join(target, path))
		if err != nil || strings.ReplaceAll(string(body), "\r\n", "\n") != want {
			t.Fatalf("%s = %q, %v", path, body, err)
		}
	}
}

func TestPackRedactsStructuredCheckpointAndGitRemote(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "init")
	runGitCommand(t, source, "config", "user.email", "amh@example.com")
	runGitCommand(t, source, "config", "user.name", "AMH Test")
	writeFile(t, filepath.Join(source, "tracked.txt"), []byte("base\n"))
	runGitCommand(t, source, "add", "tracked.txt")
	runGitCommand(t, source, "commit", "-m", "base")
	runGitCommand(t, source, "remote", "add", "origin", "https://user:remote-secret-token@github.com/example/repo.git")
	writeCodexSession(t, filepath.Join(home, ".codex"), "current-thread", source, "current")
	checkpointPath := filepath.Join(t.TempDir(), "checkpoint.json")
	writeFile(t, checkpointPath, []byte(`{"objective":"password=checkpoint-secret-value","status":"in_progress"}`))
	setTestHome(t, home)
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	if err := runPack([]string{"--cwd", source, "--checkpoint", checkpointPath, "-o", capsulePath}); err != nil {
		t.Fatal(err)
	}

	data, err := capsule.Read(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"checkpoint-secret-value", "remote-secret-token"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("structured secret %q remained in capsule metadata", secret)
		}
	}
	if data.Manifest.RedactionCount < 2 || data.Workspace.Git == nil || !strings.Contains(data.Workspace.Git.Remote, "[REDACTED]") {
		t.Fatalf("unexpected redaction metadata: %+v, workspace=%+v", data.Manifest, data.Workspace)
	}
}

func TestPackRedactsSensitiveAddedLinesWithoutDroppingPatch(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "init")
	runGitCommand(t, source, "config", "user.email", "amh@example.com")
	runGitCommand(t, source, "config", "user.name", "AMH Test")
	writeFile(t, filepath.Join(source, "tracked.txt"), []byte("base\n"))
	runGitCommand(t, source, "add", "tracked.txt")
	runGitCommand(t, source, "commit", "-m", "base")
	writeFile(t, filepath.Join(source, "secret.txt"), []byte("api_key=abcdefghijklmnopqrstuvwxyz123456\n"))
	writeCodexSession(t, filepath.Join(home, ".codex"), "current-thread", source, "current")
	setTestHome(t, home)
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	if err := runPack([]string{"--cwd", source, "-o", capsulePath}); err != nil {
		t.Fatal(err)
	}
	data, err := capsule.Read(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.WorktreePatch) == 0 || !data.Workspace.PatchIncluded || strings.Contains(string(data.WorktreePatch), "abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("sensitive added line should be redacted in a usable patch: %+v\n%s", data.Workspace, data.WorktreePatch)
	}
	if data.Manifest.RedactionCount == 0 {
		t.Fatal("sensitive patch redaction was not recorded")
	}
}

func TestPackOmitsPatchWhenSensitiveValueIsRequiredContext(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "init")
	runGitCommand(t, source, "config", "user.email", "amh@example.com")
	runGitCommand(t, source, "config", "user.name", "AMH Test")
	writeFile(t, filepath.Join(source, "config.txt"), []byte("api_key=abcdefghijklmnopqrstuvwxyz123456\nmode=before\n"))
	runGitCommand(t, source, "add", "config.txt")
	runGitCommand(t, source, "commit", "-m", "base")
	writeFile(t, filepath.Join(source, "config.txt"), []byte("api_key=abcdefghijklmnopqrstuvwxyz123456\nmode=after\n"))
	writeCodexSession(t, filepath.Join(home, ".codex"), "current-thread", source, "current")
	setTestHome(t, home)
	t.Setenv("AMH_AGENT", "codex")
	t.Setenv("CODEX_THREAD_ID", "current-thread")
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	out, err := captureOutput(t, func() error { return runPack([]string{"--cwd", source, "-o", capsulePath}) })
	if err != nil {
		t.Fatal(err)
	}
	data, err := capsule.Read(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	if data.Workspace.PatchIncluded || len(data.WorktreePatch) != 0 || !strings.Contains(data.Workspace.PatchOmission, "sensitive") {
		t.Fatalf("sensitive context patch should be omitted: %+v", data.Workspace)
	}
	if !strings.Contains(out, "no portable patch was included") || strings.Contains(out, "Included portable source workspace changes") {
		t.Fatalf("pack output misreported omitted patch:\n%s", out)
	}
}

func TestSameAgentRestoreUsesSafeSemanticModeByDefault(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	raw := []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"source\",\"cwd\":\"/source\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"developer\",\"content\":[{\"type\":\"input_text\",\"text\":\"MALICIOUS_NATIVE_DIRECTIVE\"}]}}\n")
	data := capsule.Data{
		Manifest:   capsule.Manifest{Format: capsule.Format, CapsuleID: "cap-safe", SourceAgent: "codex", SourceSessionID: "source"},
		Mission:    capsule.MissionCheckpoint{Objective: "debug", Status: "in_progress"},
		Workspace:  capsule.Workspace{CWD: "/source", PathOnly: true},
		Session:    handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", ThreadID: "source", CWD: "/source", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "portable request"}}},
		RawSession: raw,
	}
	if err := capsule.Write(capsulePath, data); err != nil {
		t.Fatal(err)
	}
	result, err := restoreMission(restoreOptions{Target: "codex", CapsulePath: capsulePath, Home: t.TempDir(), CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "safe-semantic" || bytes.Contains(body, []byte("MALICIOUS_NATIVE_DIRECTIVE")) || !bytes.Contains(body, []byte("portable request")) {
		t.Fatalf("same-Agent safe restore imported native instructions: mode=%s\n%s", result.Mode, body)
	}
}

func TestSameAgentRestoreRequiresExplicitTrustForNativeMode(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	raw := []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"source\",\"cwd\":\"/source\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"developer\",\"content\":[{\"type\":\"input_text\",\"text\":\"TRUSTED_NATIVE_DIRECTIVE\"}]}}\n")
	data := capsule.Data{
		Manifest:   capsule.Manifest{Format: capsule.Format, CapsuleID: "cap-native", SourceAgent: "codex", SourceSessionID: "source"},
		Mission:    capsule.MissionCheckpoint{Objective: "debug", Status: "in_progress"},
		Workspace:  capsule.Workspace{CWD: "/source", PathOnly: true},
		Session:    handoff.AgentSession{Format: handoff.IRFormat, SourceAgent: "codex", ThreadID: "source", CWD: "/source", Conversation: []handoff.Turn{{Role: handoff.RoleUser, Text: "portable request"}}},
		RawSession: raw,
	}
	if err := capsule.Write(capsulePath, data); err != nil {
		t.Fatal(err)
	}
	result, err := restoreMission(restoreOptions{Target: "codex", CapsulePath: capsulePath, Home: t.TempDir(), CWD: t.TempDir(), TrustNative: true})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(result.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "trusted-native" || !bytes.Contains(body, []byte("TRUSTED_NATIVE_DIRECTIVE")) {
		t.Fatalf("explicit native restore did not preserve trusted records: mode=%s\n%s", result.Mode, body)
	}
}

func TestContinueDryRunDoesNotPrintUnusableResumeCommand(t *testing.T) {
	capsulePath := filepath.Join(t.TempDir(), "mission.amh")
	writeCapsule(t, capsulePath, nil)
	out, err := captureOutput(t, func() error {
		return runContinue([]string{"--to", "claude", "--cwd", t.TempDir(), "--home", t.TempDir(), "--dry-run", capsulePath})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "claude --resume") || !strings.Contains(out, "No session was written") {
		t.Fatalf("dry run exposed an unusable resume command:\n%s", out)
	}
}

func TestDetectTargetAgentRequiresChoiceWhenBothAreInstalled(t *testing.T) {
	clearAgentEnv(t)
	bin := t.TempDir()
	writeFakeCommand(t, bin, "codex")
	writeFakeCommand(t, bin, "claude")
	t.Setenv("PATH", bin)
	_, err := detectTargetAgent()
	if err == nil || !strings.Contains(err.Error(), "both Codex and Claude Code") {
		t.Fatalf("error = %v, want explicit target choice", err)
	}
}

func TestInstalledAgentResumeCLIContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "codex", args: []string{"resume", "--help"}, want: "SESSION_ID"},
		{name: "claude", args: []string{"--help"}, want: "--resume"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.name); err != nil {
				t.Skipf("%s is not installed", test.name)
			}
			body, err := exec.Command(test.name, test.args...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s help failed: %v\n%s", test.name, err, body)
			}
			if !strings.Contains(string(body), test.want) {
				t.Fatalf("%s resume contract missing %q", test.name, test.want)
			}
		})
	}
}

func TestClaudeProjectKeyMatchesClaudeCode(t *testing.T) {
	got := claudeProjectKey(`/Users/example/.codex/worktrees/agent_mission-handoff`)
	want := `-Users-example--codex-worktrees-agent-mission-handoff`
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

func writeFakeCommand(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	body := []byte("#!/bin/sh\nexit 0\n")
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = []byte("@exit /b 0\r\n")
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	}
	if err := os.WriteFile(path, body, 0o755); err != nil {
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

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if body, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, body)
	}
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
