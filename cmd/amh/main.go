package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fume-shroom/agent-mission-handoff/internal/capability"
	"github.com/Fume-shroom/agent-mission-handoff/internal/capsule"
	"github.com/Fume-shroom/agent-mission-handoff/internal/handoff"
	"github.com/Fume-shroom/agent-mission-handoff/internal/restore"
	"github.com/Fume-shroom/agent-mission-handoff/internal/security"
	workspacecapture "github.com/Fume-shroom/agent-mission-handoff/internal/workspace"
)

var version = "v0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "pack":
		err = runPack(os.Args[2:])
	case "continue":
		err = runContinue(os.Args[2:])
	case "export":
		err = runExport(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "preflight":
		err = runPreflight(os.Args[2:])
	case "restore":
		err = runRestore(os.Args[2:])
	case "apply":
		err = runApply(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "update":
		err = runUpdate(os.Args[2:])
	case "uninstall":
		err = runUninstall(os.Args[2:])
	case "version":
		fmt.Println("amh " + version)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return
	default:
		usage()
	}
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	printUsage(os.Stderr)
	os.Exit(2)
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, `Agent Mission Handoff (amh)

Commands:
	amh pack      [-o mission.amh]
	amh continue  mission.amh
	amh doctor
	amh update
	amh uninstall

Advanced:
  amh export    --agent codex|claude --session latest|ID|PATH -o mission.amh
  amh inspect   [--json] mission.amh
  amh preflight [--cwd PATH] [--json] mission.amh
  amh restore   --to codex|claude --cwd PATH [--home PATH] mission.amh
  amh apply     [--cwd PATH] mission.amh`)
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	agent := fs.String("agent", "", "source agent: codex or claude")
	session := fs.String("session", "latest", "session path, id, or latest")
	output := fs.String("o", "mission.amh", "output capsule")
	checkpointPath := fs.String("checkpoint", "", "optional Mission Checkpoint JSON prepared by the source agent")
	home := fs.String("home", "", "source agent home override")
	cwd := fs.String("cwd", "", "source workspace for session resolution")
	includeSensitive := fs.Bool("include-sensitive", false, "disable best-effort credential redaction")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *agent != "codex" && *agent != "claude" {
		return errors.New("--agent must be codex or claude")
	}
	result, err := exportMission(exportOptions{Agent: *agent, Session: *session, Output: *output, CheckpointPath: *checkpointPath, Home: *home, CWD: *cwd, IncludeSensitive: *includeSensitive})
	if err != nil {
		return err
	}
	fmt.Printf("Exported %s mission %s to %s\n", result.Agent, result.SessionID, result.CapsulePath)
	fmt.Printf("Turns: %d, capabilities: %d, cwd: %s\n", result.TurnCount, result.Capabilities, result.CWD)
	return nil
}

type exportOptions struct {
	Agent            string
	Session          string
	Output           string
	CheckpointPath   string
	Home             string
	CWD              string
	IncludeSensitive bool
}

type exportResult struct {
	Agent         string
	SessionID     string
	SessionPath   string
	CapsulePath   string
	TurnCount     int
	Capabilities  int
	CWD           string
	Redactions    int
	Dirty         bool
	PatchIncluded bool
	PatchOmission string
	Workspace     capsule.Workspace
}

func exportMission(opts exportOptions) (exportResult, error) {
	path, err := resolveSession(sessionQuery{Agent: opts.Agent, Query: opts.Session, Home: opts.Home, CWD: opts.CWD})
	if err != nil {
		return exportResult{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return exportResult{}, err
	}
	redactions := security.Report{}
	if !opts.IncludeSensitive {
		raw, redactions = security.Redact(raw)
	}
	var normalized handoff.AgentSession
	if opts.Agent == "codex" {
		normalized, err = handoff.FromCodexBytes(raw)
	} else {
		normalized, err = handoff.FromClaudeBytes(raw)
	}
	if err != nil {
		return exportResult{}, err
	}
	if len(normalized.Conversation) == 0 {
		return exportResult{}, errors.New("no portable conversation found in session")
	}
	workspaceState, worktreePatch, indexPatch, err := workspacecapture.Capture(firstNonEmpty(opts.CWD, normalized.CWD))
	if err != nil {
		return exportResult{}, err
	}
	if !opts.IncludeSensitive {
		var report security.Report
		workspaceState, report, err = security.RedactJSON(workspaceState)
		if err != nil {
			return exportResult{}, err
		}
		redactions.Add(report)
	}
	if workspaceState.CWD != "" {
		normalized.CWD = workspaceState.CWD
	}
	if workspaceState.Git != nil {
		normalized.Git = workspaceState.Git
	} else if normalized.Git != nil {
		workspaceState.Git = normalized.Git
	}
	if !opts.IncludeSensitive && len(worktreePatch) > 0 {
		var patchReport security.Report
		var safe bool
		worktreePatch, patchReport, safe = security.RedactPatch(worktreePatch)
		redactions.Add(patchReport)
		workspaceState.PatchRedactions = patchReport.Count
		if !safe {
			worktreePatch = nil
			indexPatch = nil
			workspaceState.PatchIncluded = false
			workspaceState.PatchBytes = 0
			workspaceState.PatchOmission = "worktree patch contained a sensitive value outside added text and was omitted"
			workspaceState.IndexPatchIncluded = false
			workspaceState.IndexPatchBytes = 0
			workspaceState.IndexPatchOmission = workspaceState.PatchOmission
			workspaceState.PathOnly = true
		} else {
			workspaceState.PatchBytes = len(worktreePatch)
		}
	}
	if !opts.IncludeSensitive && len(indexPatch) > 0 {
		var patchReport security.Report
		var safe bool
		indexPatch, patchReport, safe = security.RedactPatch(indexPatch)
		redactions.Add(patchReport)
		workspaceState.IndexPatchRedactions = patchReport.Count
		if !safe {
			indexPatch = nil
			workspaceState.IndexPatchIncluded = false
			workspaceState.IndexPatchBytes = 0
			workspaceState.IndexPatchOmission = "staged patch contained a sensitive value outside added text and was omitted"
		} else {
			workspaceState.IndexPatchBytes = len(indexPatch)
		}
	}
	workspaceState.PatchIncluded = len(worktreePatch) > 0 || len(indexPatch) > 0
	workspaceState.IndexPatchIncluded = len(indexPatch) > 0
	workspaceState.PatchBytes = len(worktreePatch)
	workspaceState.IndexPatchBytes = len(indexPatch)
	workspaceState.PathOnly = !workspaceState.PatchIncluded
	if workspaceState.Dirty && !workspaceState.PatchIncluded && workspaceState.PatchOmission == "" {
		workspaceState.PatchOmission = firstNonEmpty(workspaceState.IndexPatchOmission, "source changes could not be represented as a portable patch")
	}

	mission := checkpoint(normalized)
	if opts.CheckpointPath != "" {
		body, readErr := os.ReadFile(opts.CheckpointPath)
		if readErr != nil {
			return exportResult{}, readErr
		}
		if err := json.Unmarshal(body, &mission); err != nil {
			return exportResult{}, fmt.Errorf("checkpoint: %w", err)
		}
		mission.EvidenceTurnCount = len(normalized.Conversation)
		if mission.Status == "" {
			mission.Status = "in_progress"
		}
	}
	capabilities := capability.DiscoverWithOptions(raw, normalized, capability.Options{Agent: opts.Agent, CWD: normalized.CWD, Home: opts.Home})
	if !opts.IncludeSensitive {
		var report security.Report
		mission, report, err = security.RedactJSON(mission)
		if err != nil {
			return exportResult{}, err
		}
		redactions.Add(report)
		capabilities, report, err = security.RedactJSON(capabilities)
		if err != nil {
			return exportResult{}, err
		}
		redactions.Add(report)
	}
	data := capsule.Data{
		Manifest: capsule.Manifest{
			Format: capsule.Format, CapsuleID: newID(), CreatedAt: time.Now().UTC().Format(time.RFC3339),
			SourceAgent: opts.Agent, SourceSessionID: normalized.ThreadID, RedactionCount: redactions.Count,
			SensitiveContentPolicy: sensitivePolicy(opts.IncludeSensitive),
		},
		Mission:       mission,
		Capabilities:  capabilities,
		Workspace:     workspaceState,
		Session:       normalized,
		RawSession:    raw,
		WorktreePatch: worktreePatch,
		IndexPatch:    indexPatch,
	}
	if err := capsule.Write(opts.Output, data); err != nil {
		return exportResult{}, err
	}
	result := exportResult{
		Agent: opts.Agent, SessionID: normalized.ThreadID, SessionPath: path,
		CapsulePath: opts.Output, TurnCount: len(normalized.Conversation),
		Capabilities: len(data.Capabilities), CWD: normalized.CWD,
		Redactions: redactions.Count, Dirty: workspaceState.Dirty,
		PatchIncluded: workspaceState.PatchIncluded, PatchOmission: workspaceState.PatchOmission,
		Workspace: workspaceState,
	}
	return result, nil
}

func runPack(args []string) error {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	agent := fs.String("agent", "auto", "source agent: auto, codex, or claude")
	session := fs.String("session", "current", "session path, id, or current")
	output := fs.String("o", "mission.amh", "output capsule")
	checkpointPath := fs.String("checkpoint", "", "optional Mission Checkpoint JSON")
	home := fs.String("home", "", "source agent home override")
	cwd := fs.String("cwd", "", "source workspace")
	includeSensitive := fs.Bool("include-sensitive", false, "disable best-effort credential redaction")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: amh pack [-o mission.amh]")
	}
	if *cwd == "" {
		current, err := os.Getwd()
		if err != nil {
			return err
		}
		*cwd = current
	}
	if *agent == "auto" {
		detected, err := detectSourceAgent(*cwd, *home)
		if err != nil {
			return err
		}
		*agent = detected
	}
	if *agent != "codex" && *agent != "claude" {
		return errors.New("--agent must be auto, codex, or claude")
	}
	result, err := exportMission(exportOptions{Agent: *agent, Session: *session, Output: *output, CheckpointPath: *checkpointPath, Home: *home, CWD: *cwd, IncludeSensitive: *includeSensitive})
	if err != nil {
		return err
	}
	fmt.Printf("Packed current %s mission to %s\n", result.Agent, result.CapsulePath)
	if result.Redactions > 0 {
		fmt.Printf("Redacted %d high-confidence sensitive value(s).\n", result.Redactions)
	}
	if result.PatchIncluded {
		fmt.Println("Included portable source workspace changes for optional receiver-side application.")
		if result.Workspace.Staged && !result.Workspace.IndexPatchIncluded {
			fmt.Printf("Source staged state was not preserved: %s.\n", firstNonEmpty(result.Workspace.IndexPatchOmission, "no portable staged-state patch was available"))
		}
	} else if result.Dirty {
		fmt.Printf("Source worktree is dirty; no portable patch was included: %s.\n", result.PatchOmission)
	}
	return nil
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "include the complete normalized conversation as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: amh inspect [--json] FILE")
	}
	data, err := capsule.Read(fs.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		body, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	}
	fmt.Printf("Mission capsule: %s\n", restore.SafeTerminal(data.Manifest.CapsuleID))
	fmt.Printf("Source: %s session %s\n", restore.SafeTerminal(data.Manifest.SourceAgent), restore.SafeTerminal(data.Manifest.SourceSessionID))
	fmt.Printf("Objective: %s\n", restore.SafeTerminal(data.Mission.Objective))
	fmt.Printf("Status: %s\n", restore.SafeTerminal(data.Mission.Status))
	fmt.Printf("Current summary: %s\n", restore.SafeTerminal(data.Mission.CurrentSummary))
	fmt.Printf("Conversation turns: %d\n", len(data.Session.Conversation))
	fmt.Printf("Workspace: %s\n", restore.SafeTerminal(data.Workspace.CWD))
	if data.Manifest.RedactionCount > 0 {
		fmt.Printf("Sensitive values redacted: %d\n", data.Manifest.RedactionCount)
	}
	if data.Workspace.Dirty {
		fmt.Printf("Source workspace: dirty (portable changes: %t, %d bytes)\n", data.Workspace.PatchIncluded, data.Workspace.PatchBytes+data.Workspace.IndexPatchBytes)
		if redactions := data.Workspace.PatchRedactions + data.Workspace.IndexPatchRedactions; redactions > 0 {
			fmt.Printf("Portable patch redactions: %d\n", redactions)
		}
		if data.Workspace.Staged && !data.Workspace.IndexPatchIncluded {
			fmt.Printf("Source staged state: not preserved (%s)\n", restore.SafeTerminal(firstNonEmpty(data.Workspace.IndexPatchOmission, "no portable staged-state patch was available")))
		}
	}
	for _, c := range data.Capabilities {
		fmt.Printf("- %s %s (%s, %.0f%%)\n", restore.SafeTerminal(c.Kind), restore.SafeTerminal(c.Name), restore.SafeTerminal(c.Detection), c.Confidence*100)
	}
	return nil
}

func runPreflight(args []string) error {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "destination workspace")
	target := fs.String("to", "", "destination agent: codex or claude")
	home := fs.String("home", "", "destination agent home override")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: amh preflight [--cwd PATH] FILE")
	}
	data, err := capsule.Read(fs.Arg(0))
	if err != nil {
		return err
	}
	if *target != "" && *target != "codex" && *target != "claude" {
		return errors.New("--to must be codex or claude")
	}
	checks := restore.PreflightFor(data, restore.Options{CWD: *cwd, Target: *target, Home: *home})
	if *asJSON {
		body, _ := json.MarshalIndent(checks, "", "  ")
		fmt.Println(string(body))
	} else {
		fmt.Print(restore.FormatChecks(checks))
	}
	return nil
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	target := fs.String("to", "", "destination agent: codex or claude")
	cwd := fs.String("cwd", "", "destination workspace")
	home := fs.String("home", "", "destination agent home override")
	dryRun := fs.Bool("dry-run", false, "print plan without writing")
	allowMissing := fs.Bool("allow-missing", false, "continue after confirmation of required environment differences")
	trustNative := fs.Bool("trust-native-session", false, "preserve same-Agent native records from a trusted capsule")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: amh restore --to codex|claude --cwd PATH FILE")
	}
	if *target != "codex" && *target != "claude" {
		return errors.New("--to must be codex or claude")
	}
	data, err := capsule.Read(fs.Arg(0))
	if err != nil {
		return err
	}
	if *cwd == "" {
		*cwd = data.Workspace.CWD
	}
	if *cwd == "" {
		return errors.New("destination cwd is required")
	}

	result, err := restoreMission(restoreOptions{Target: *target, CapsulePath: fs.Arg(0), CWD: *cwd, Home: *home, DryRun: *dryRun, AllowMissing: *allowMissing, TrustNative: *trustNative})
	if err != nil {
		return err
	}
	fmt.Print(restore.FormatChecks(result.Checks))
	if result.DryRun {
		fmt.Printf("Dry run passed: a writable %s session would be restored in %s mode. No session was written.\n", result.Target, result.Mode)
		return nil
	}
	fmt.Printf("Restored writable %s session %s in %s mode\n", result.Target, result.SessionID, result.Mode)
	fmt.Printf("Session file: %s\n", result.Destination)
	fmt.Printf("Continue with: %s\n", result.ResumeCommand)
	return nil
}

type restoreOptions struct {
	Target       string
	CapsulePath  string
	CWD          string
	Home         string
	DryRun       bool
	AllowMissing bool
	TrustNative  bool
}

type restoreResult struct {
	Target        string
	SessionID     string
	Destination   string
	CWD           string
	ResumeCommand string
	Checks        []restore.Check
	Mission       capsule.Data
	DryRun        bool
	Mode          string
}

func restoreMission(opts restoreOptions) (restoreResult, error) {
	data, err := capsule.Read(opts.CapsulePath)
	if err != nil {
		return restoreResult{}, err
	}
	if opts.CWD == "" {
		opts.CWD, err = os.Getwd()
		if err != nil {
			return restoreResult{}, err
		}
	}
	if opts.CWD == "" {
		return restoreResult{}, errors.New("destination cwd is required")
	}
	opts.CWD, err = filepath.Abs(opts.CWD)
	if err != nil {
		return restoreResult{}, err
	}

	checks := restore.PreflightFor(data, restore.Options{CWD: opts.CWD, Target: opts.Target, Home: opts.Home})
	if missing := hardWorkspaceMissing(checks); len(missing) > 0 {
		return restoreResult{}, fmt.Errorf("destination workspace is not ready:\n%sselect or prepare the correct workspace and retry", restore.FormatChecks(missing))
	}
	if missing := confirmableMissing(checks); len(missing) > 0 && !opts.AllowMissing {
		return restoreResult{}, fmt.Errorf("destination needs confirmed fixes:\n%srerun with --allow-missing after user confirmation", restore.FormatChecks(missing))
	}
	session := data.Session
	session.CWD = opts.CWD
	capsulePath, err := filepath.Abs(opts.CapsulePath)
	if err != nil {
		return restoreResult{}, err
	}
	context := missionContext(data, capsulePath, checks)

	var body []byte
	var sessionID string
	targetModelProvider := ""
	if opts.Target == "codex" {
		targetModelProvider, err = codexModelProvider(opts.Home)
		if err != nil {
			return restoreResult{}, err
		}
	}
	mode := "safe-semantic"
	if data.Manifest.SourceAgent == opts.Target && opts.TrustNative {
		mode = "trusted-native"
		sessionID = newID()
		body, err = restore.NativeFork(opts.Target, data.RawSession, sessionID, data.Workspace.CWD, opts.CWD, context, targetModelProvider)
	} else {
		session.ThreadID = data.Manifest.CapsuleID + ":" + newID()
		session.Conversation = append(session.Conversation, handoff.Turn{Role: handoff.RoleUser, Text: context})
		if opts.Target == "codex" {
			body, sessionID, err = handoff.ToCodex(session, targetModelProvider)
		} else {
			body, sessionID, err = handoff.ToClaude(session)
		}
	}
	if err != nil {
		return restoreResult{}, err
	}
	dest, err := destinationPath(opts.Target, opts.Home, opts.CWD, sessionID)
	if err != nil {
		return restoreResult{}, err
	}
	resume := ""
	if !opts.DryRun {
		resume = "codex resume " + sessionID
		if opts.Target == "claude" {
			resume = "claude --resume " + sessionID
		}
	}
	data.RawSession = nil
	result := restoreResult{Target: opts.Target, SessionID: sessionID, Destination: dest, CWD: opts.CWD, ResumeCommand: resume, Checks: checks, Mission: data, DryRun: opts.DryRun, Mode: mode}
	if opts.DryRun {
		return result, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return restoreResult{}, fmt.Errorf("destination session already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return restoreResult{}, err
	}
	if err := os.WriteFile(dest, body, 0o600); err != nil {
		return restoreResult{}, err
	}
	return result, nil
}

func runContinue(args []string) error {
	fs := flag.NewFlagSet("continue", flag.ContinueOnError)
	target := fs.String("to", "auto", "destination agent: auto, codex, or claude")
	cwd := fs.String("cwd", "", "destination workspace")
	home := fs.String("home", "", "destination agent home override")
	dryRun := fs.Bool("dry-run", false, "validate and print the resume action without writing")
	allowMissing := fs.Bool("allow-missing", false, "continue after confirmation of required environment differences")
	trustNative := fs.Bool("trust-native-session", false, "preserve same-Agent native records from a trusted capsule")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: amh continue [--allow-missing] FILE")
	}
	if *target == "auto" {
		detected, err := detectTargetAgent()
		if err != nil {
			return err
		}
		*target = detected
	}
	if *target != "codex" && *target != "claude" {
		return errors.New("--to must be auto, codex, or claude")
	}
	result, err := restoreMission(restoreOptions{Target: *target, CapsulePath: fs.Arg(0), CWD: *cwd, Home: *home, DryRun: *dryRun, AllowMissing: *allowMissing, TrustNative: *trustNative})
	if err != nil {
		return err
	}
	if result.DryRun {
		printMissionBrief(result)
		fmt.Printf("Dry run passed: a writable %s session would be restored in %s mode. No session was written.\n", result.Target, result.Mode)
		return nil
	}
	printMissionBrief(result)
	fmt.Printf("Mission restored in %s mode. Continue with: %s\n", result.Mode, result.ResumeCommand)
	return nil
}

func runApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	cwd := fs.String("cwd", "", "destination workspace")
	allowDirty := fs.Bool("allow-dirty", false, "apply despite existing destination changes")
	allowGitDifference := fs.Bool("allow-git-difference", false, "apply despite a different destination HEAD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: amh apply [--cwd PATH] FILE")
	}
	if *cwd == "" {
		current, err := os.Getwd()
		if err != nil {
			return err
		}
		*cwd = current
	}
	data, err := capsule.Read(fs.Arg(0))
	if err != nil {
		return err
	}
	result, err := workspacecapture.Apply(data, workspacecapture.ApplyOptions{CWD: *cwd, AllowDirty: *allowDirty, AllowGitDifference: *allowGitDifference})
	if err != nil {
		return err
	}
	if result.AlreadyApplied {
		if result.IndexRestored {
			fmt.Println("Source worktree changes were already present; restored the source staged state.")
			return nil
		}
		fmt.Println("Source worktree changes are already present.")
		return nil
	}
	fmt.Printf("Applied source workspace changes to %s\n", result.CWD)
	if result.IndexRestored {
		fmt.Println("Restored the source staged state.")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sensitivePolicy(includeSensitive bool) string {
	if includeSensitive {
		return "included-by-explicit-request"
	}
	return "best-effort-redaction"
}
