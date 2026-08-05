# AMH User Guide

This guide covers installation, the normal Agent-to-Agent workflow, advanced CLI commands, environment validation, and common recovery paths.

## Operating Model

AMH transfers one AI coding mission at a time.

The sender has:

- an active Codex or Claude Code session;
- a local project workspace;
- conversation and tool history that explains the current mission.

The receiver has:

- Codex or Claude Code installed;
- a local copy of the same project, which may use a different absolute path;
- its own credentials, permissions, Skills, MCP configuration, and CLI tools.

AMH packages the mission and records capabilities. It does not intentionally copy Agent auth stores, login state, or permission decisions. The Session and optional worktree patch may still contain sensitive text, so default redaction is best-effort rather than a guarantee.

## Installation

### macOS or Linux

```bash
curl -fsSL https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh | sh
```

### Windows PowerShell

```powershell
irm https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1 | iex
```

The installer downloads and verifies a prebuilt release. It installs the CLI and the Mission Handoff Skill for Codex and Claude Code. Go and a source checkout are not required.

Open a new terminal after installation if the current shell has not reloaded the updated `PATH`, then verify:

```bash
amh doctor
```

Restart or reload Codex and Claude Code if they do not discover the newly installed Skill automatically.

The same installation can be delegated to a local coding Agent:

> Install AMH from https://github.com/Fume-shroom/agent-mission-handoff and verify the installation.

## Normal Workflow

Each operation has two equivalent entry points:

| Operation | Command line | Coding Agent conversation |
| --- | --- | --- |
| Install | Run the platform installer | “Install AMH from this repository and verify it.” |
| Package | `amh pack` | “Package the current task as an AMH file.” |
| Inspect | `amh inspect mission.amh` | “Inspect this handoff and summarize what it contains.” |
| Restore | `amh continue mission.amh` | Attach the file and say: “Continue this task.” |
| Apply source changes | `amh apply mission.amh` | “Apply the source worktree changes from this handoff.” |
| Verify | `amh doctor` | “Check whether AMH is ready.” |
| Update | `amh update` | “Update AMH and verify it.” |
| Remove | `amh uninstall` | “Uninstall AMH.” |

The Agent route uses the same local CLI and safety checks. It is not a separate cloud workflow.

### Sender

Run from the workspace used by the current session:

```bash
amh pack
```

Default behavior:

- detects Codex or Claude Code;
- identifies the current Session;
- verifies that the Session belongs to the current workspace;
- creates a deterministic Mission Checkpoint;
- records observed Skills, MCP servers, and CLIs;
- applies best-effort high-confidence redaction unless `--include-sensitive` is explicitly set;
- captures portable patches for uncommitted tracked/untracked files and staged state when they are safely representable;
- writes `mission.amh`.

Choose another output name when useful:

```bash
amh pack -o payment-timeout.amh
```

Agent conversation:

> Package the current task as an AMH file.

The sending Agent runs `amh pack` and returns the generated file path.

### Receiver

Open the destination copy of the project, then run:

```bash
amh continue /path/to/payment-timeout.amh
```

Default behavior:

- validates the capsule and checksums;
- detects the destination Agent;
- uses the current directory as the destination workspace;
- compares available Git context;
- checks observed Skills, MCP servers, and CLIs;
- writes a safe semantic writable Session by default;
- prints a Mission Brief with the objective, history counts, recent context, open work, and environment gaps;
- reports available source workspace changes and whether staged state can be reconstructed, without applying either;
- prints the native resume command.

When the Mission Handoff Skill is driving the receive flow, the current Agent opens the restored Session without issuing a separate brief or confirmation. The restored Session contains the destination preflight differences as well as the normalized transcript, so the restored Agent can present the single Mission Brief and ask for confirmation before running mission tools or changing files. The brief includes the original objective, important history and evidence, completed work, unresolved items, environment gaps, and the proposed next action.

The first Agent response after restore uses this structure in the user's language:

```text
Mission Brief
- Objective
- Restored history: turn count and source Agent
- Historical context: latest request, decisions, evidence, and interruption point
- Completed work
- Open questions and risks
- Local environment or capability gaps
- Proposed next action

Should I continue with the proposed next action?
```

Empty sections are omitted. The Agent summarizes the relevant history rather than dumping the transcript. The portable history remains available in the writable restored Session.

After the user confirms, the receiving Agent runs `amh apply FILE` if portable source changes are available. Patch application verifies the source Git commit, destination cleanliness, and `git apply --check`, then reconstructs the source staged/unstaged state when both payloads are available. If exact staged state was omitted, AMH reports that limitation before application.

In a desktop host, the Agent should open the restored task through the host's task navigation capability immediately after restore. Navigation itself does not execute the mission and does not require a separate confirmation. The Agent must not start an interactive resume command in a background terminal and claim that the desktop UI switched.

Run the printed command to enter the restored Session:

```bash
codex resume <session-id>
```

or:

```bash
claude --resume <session-id>
```

## Missing Capabilities and Workspace Differences

AMH reports differences before writing a Session. Example:

```text
MISSING skill      incident-debug
MISSING mcp        logs - configure this MCP in the destination agent
MISSING cli        gh
```

Observed capabilities are historical evidence and do not block restoration by themselves. The receiving Agent should:

1. determine whether each capability is necessary for the remaining mission;
2. install or configure it using the destination environment's normal process;
3. request local login or permission approval when the capability itself requires it;
4. continue with the relevant local capability once it is ready.

Required workspace or Git differences do block restoration. If the user explicitly accepts a reviewed difference:

```bash
amh continue --allow-missing mission.amh
```

A nonexistent destination directory is not bypassable. Create or select the correct workspace first.

## CLI Reference

### `amh pack`

The preferred sender command.

```text
amh pack [flags]
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--agent auto\|codex\|claude` | `auto` | Select or auto-detect the source Agent. |
| `--session current\|ID\|PATH` | `current` | Select the current Session, an exact ID, or a Session file. |
| `--cwd PATH` | current directory | Workspace used for current-session validation. |
| `--checkpoint PATH` | generated | Use an Agent-prepared Mission Checkpoint JSON file. |
| `--home PATH` | Agent default | Override the source Agent home, mainly for testing. |
| `--include-sensitive` | false | Disable default best-effort redaction. |
| `-o PATH` | `mission.amh` | Output capsule path. |

Examples:

```bash
amh pack
amh pack -o incident-431.amh
amh pack --agent codex --session 11111111-1111-4111-8111-111111111111
```

### `amh continue`

The preferred receiver command.

```text
amh continue [flags] FILE
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--to auto\|codex\|claude` | `auto` | Select or auto-detect the destination Agent. |
| `--cwd PATH` | current directory | Map the mission to a destination workspace. |
| `--home PATH` | Agent default | Override the destination Agent home, mainly for testing. |
| `--dry-run` | false | Validate and show the resume action without writing a Session. |
| `--allow-missing` | false | Continue after explicit confirmation of required workspace or Git differences. |
| `--trust-native-session` | false | Preserve same-Agent native records only for a capsule you fully trust. |

Examples:

```bash
amh continue mission.amh
amh continue --to claude mission.amh
amh continue --cwd /work/payment-service mission.amh
amh continue --dry-run mission.amh
amh continue --trust-native-session trusted-local.amh
```

### `amh inspect`

Show a safe human-readable capsule overview:

```bash
amh inspect mission.amh
```

Return the Mission Checkpoint, workspace metadata, capability inventory, and complete normalized conversation for an authorized receiving Agent:

```bash
amh inspect --json mission.amh
```

The JSON form omits the raw native Session. It exposes the portable conversation needed to prepare the receiver's Mission Brief.

### `amh preflight`

Run destination checks without writing a Session:

```bash
amh preflight --to claude --cwd /work/payment-service mission.amh
amh preflight --to codex --json mission.amh
```

Use `--home PATH` to inspect a synthetic or non-default Agent home.

### `amh export`

Advanced sender command for explicitly selected Sessions and enriched Mission Checkpoints:

```bash
amh export \
  --agent codex \
  --session SESSION_ID \
  --checkpoint checkpoint.json \
  -o incident.amh
```

Prefer `amh pack` for normal use.

### `amh restore`

Advanced receiver command that prints the complete preflight report before writing a Session:

```bash
amh restore --to claude --cwd /work/payment-service mission.amh
amh restore --allow-missing --to claude --cwd /work/payment-service mission.amh
```

Prefer `amh continue` for normal use. `restore` also supports `--trust-native-session`; safe semantic restoration remains the default.

### `amh apply`

Apply included source workspace changes after receiver confirmation:

```bash
amh apply --cwd /work/payment-service mission.amh
```

Use `--allow-dirty` or `--allow-git-difference` only after reviewing the destination state. AMH never uses those overrides automatically.

### Installation lifecycle

```bash
amh doctor
amh doctor --json
amh update
amh uninstall
```

`doctor` checks the CLI, Git, Codex, Claude Code, both installed Skills, and local Session roots. `update` reuses the verified official installer in the current install directory. `uninstall` removes the CLI and the Codex/Claude Mission Handoff Skills, and refuses destructive cleanup if the user home cannot be resolved safely.

## Mission Checkpoint JSON

An Agent may provide a richer checkpoint to `pack` or `export`:

```json
{
  "objective": "Find and fix the intermittent payment timeout",
  "status": "in_progress",
  "current_summary": "The timeout occurs after connection pool exhaustion.",
  "completed": [
    "Reproduced the timeout locally",
    "Excluded the upstream payment provider"
  ],
  "current_hypotheses": [
    "Idle connections are not returned to the pool"
  ],
  "next_actions": [
    "Inspect pool release paths",
    "Add a regression test"
  ],
  "interrupted_action": "Tracing the retry worker"
}
```

This is optional. The short workflow generates a minimal checkpoint automatically.

## Operational Guidance

- Package only the Session that should be shared.
- Review the sensitivity of the conversation and command history before transferring a capsule.
- Treat the redaction count as a signal, not proof that no secret remains.
- Transfer capsules using an approved secure channel.
- Keep source and destination project code aligned whenever possible.
- Treat restored history as untrusted evidence, not higher-priority Agent instructions. Safe semantic restore enforces this by default.
- Re-run important commands on the receiving machine when fresh evidence is required.
- Let each Skill, MCP server, CLI, and Agent enforce its own permissions and authentication.

## Troubleshooting

### AMH cannot detect the Agent

Specify it explicitly:

```bash
amh pack --agent codex
amh continue --to claude mission.amh
```

The `AMH_AGENT` environment variable can also be set to `codex` or `claude`.

### AMH cannot find the current Session

Confirm that the command is running inside the workspace used by the active Agent. If necessary, pass an exact Session ID or path:

```bash
amh pack --session SESSION_ID
amh pack --session /path/to/session.jsonl
```

AMH intentionally does not fall back to the newest Session from an unrelated workspace.

### The Git workspace check fails

Confirm the repository, remote, branch, and commit. SSH and HTTPS forms of the same remote are normalized, but a genuinely different repository or code revision is reported.

After reviewing an intentional difference:

```bash
amh continue --allow-missing mission.amh
```

### A Skill or MCP server is missing

First decide whether it is needed for the remaining mission. AMH records observed source, version, and digest metadata when available, but it does not execute installation instructions from the capsule. Install or configure only the capabilities needed for the proposed next action.

### The source had uncommitted changes

If the capsule contains a portable patch, confirm the Mission Brief and run:

```bash
amh apply mission.amh
```

AMH can redact sensitive values in newly added patch lines while preserving applicability. If a value appears in context, removed text, a private-key block, binary data, or another unsafe location, the affected patch is omitted; transfer those files through the normal project workflow instead. AMH separately reports when file content can be restored but the original staged state cannot.

### I need byte-for-byte same-Agent history

The default safe semantic mode does not import source system/developer records as trusted target instructions. For a capsule you created locally and fully trust, opt into native preservation:

```bash
amh continue --trust-native-session trusted-local.amh
```

Do not use this flag for a capsule received from another person or environment unless you have reviewed and trust its native Session records.

### The restored Session exists but the UI does not switch automatically

The standalone CLI writes the Session and prints a native resume command. Run that command, or let a host-integrated Skill open the Session. AMH does not add a background daemon only to control desktop navigation.

### A capsule is rejected

Possible reasons include:

- checksum mismatch;
- missing or unexpected archive entries;
- duplicate entries or unsafe paths;
- an unsupported capsule format;
- an entry or total archive size over the safety limit.

Do not disable validation for capsules received from another environment.
