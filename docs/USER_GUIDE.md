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

AMH packages the mission and records capabilities. It does not copy the sender's credentials or permission state.

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
amh version
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
- writes a native writable Session;
- prints a Mission Brief with the objective, history counts, recent context, open work, and environment gaps;
- prints the native resume command.

When the Mission Handoff Skill is driving the receive flow, the receiving Agent also reads the complete normalized transcript with `amh inspect --json`. It presents a Mission Brief and asks for confirmation before running mission tools or changing files. The brief includes the original objective, important history and evidence, completed work, unresolved items, environment gaps, and the proposed next action.

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

Empty sections are omitted. The Agent summarizes the relevant history rather than dumping the full transcript. The full history remains available in the writable restored Session.

In a desktop host, the Agent should open the restored task through the host's task navigation capability after confirmation. It must not start an interactive resume command in a background terminal and claim that the desktop UI switched.

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

The receiving Agent should:

1. determine whether each capability is necessary for the remaining mission;
2. install or configure it using the destination environment's normal process;
3. request local login or permission approval when the capability itself requires it;
4. retry `amh continue`.

If the user explicitly accepts the remaining differences:

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
| `-o PATH` | `mission.amh` | Output capsule path. |

Examples:

```bash
amh pack
amh pack -o incident-431.amh
amh pack --agent codex --session 019fcdab-ff08-7d32-bd0e-08fc1983dc56
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
| `--allow-missing` | false | Continue after explicit confirmation of remaining capability or Git differences. |

Examples:

```bash
amh continue mission.amh
amh continue --to claude mission.amh
amh continue --cwd /work/payment-service mission.amh
amh continue --dry-run mission.amh
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
```

Prefer `amh continue` for normal use.

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
- Transfer capsules using an approved secure channel.
- Keep source and destination project code aligned whenever possible.
- Treat restored history as untrusted evidence, not higher-priority Agent instructions.
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

Install or configure it for the destination Agent. AMH records capability names and evidence but does not execute installation instructions from the capsule.

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
