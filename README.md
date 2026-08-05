# Agent Mission Handoff

Agent Mission Handoff (`amh`) packages an in-progress AI coding mission into one portable `.amh` file and restores it as a writable Codex or Claude Code session on another machine or Agent.

The normal workflow is one command on each side:

```bash
# Sender
amh pack

# Receiver
amh continue mission.amh
```

No daemon, hosted service, account, database, or GitHub repository is required for the transfer itself.

> Status: technical preview. Codex and Claude Code session formats are private implementation details and may require adapter updates when those products change.

## Why AMH

A conversation export is useful for reading, but a coding mission also depends on its execution context. AMH carries the information another coding Agent needs to understand the work, validate its local environment, and continue from the handoff point.

AMH is designed for:

- moving one active coding session between development machines;
- handing an unfinished investigation to a teammate's coding Agent;
- reproducing a completed debugging or implementation session locally;
- continuing a mission between Codex and Claude Code;
- preserving native history when the source and destination Agent are the same.

## Features

- **One-file handoff**: mission state, session history, workspace metadata, and capability inventory are stored in one `.amh` capsule.
- **Two-command happy path**: `amh pack` on the sender and `amh continue FILE` on the receiver.
- **Agent auto-detection**: detects Codex or Claude Code from the current Agent environment.
- **Current-session detection**: resolves the active session using Agent identity and workspace evidence instead of choosing an unrelated global latest session.
- **Writable restore**: creates a session that the receiving Agent can continue, not only inspect.
- **Cross-Agent translation**: supports Codex to Claude Code and Claude Code to Codex through a normalized conversation format.
- **Native same-Agent fork**: preserves the original native history when restoring to the same Agent.
- **Capability Lock**: records directly observed Skills, MCP servers, and command-line tools.
- **Restore Planner**: checks the destination workspace and capabilities before restoring.
- **Local-first security**: validates checksums, archive entries, workspace identity, and untrusted terminal output without transferring credentials.

## Supported Handoffs

| Source | Destination | Restore mode |
| --- | --- | --- |
| Codex | Codex | Native writable fork |
| Claude Code | Claude Code | Native writable fork |
| Codex | Claude Code | Semantic session translation |
| Claude Code | Codex | Semantic session translation |

Cross-Agent restore preserves the useful conversation and mission context. It does not claim to reproduce private model state, tool-call IDs, permissions, or runtime internals byte for byte.

## Install

macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1 | iex
```

The installer:

- detects the operating system and CPU architecture;
- downloads the matching binary from the latest GitHub Release;
- verifies its SHA-256 checksum;
- installs `amh` under the current user's home directory;
- adds the installation directory to the user's `PATH` when needed;
- installs the Mission Handoff Skill for both Codex and Claude Code;
- verifies the installed CLI.

Users do not need Go or a source checkout. Codex, Claude Code, or both must be installed to create and resume Agent Sessions. The same project code must be available on the sending and receiving machines.

## Five-Minute Start

### 1. Send a mission

From the project directory and the active Codex or Claude Code session:

```bash
amh pack
```

AMH creates `mission.amh` in the current directory.

With the Skill installed, the human interaction can simply be:

> Package the current task as an AMH file.

The Agent runs `amh pack` and returns the file path.

### 2. Transfer the file

Move `mission.amh` using any channel appropriate for its sensitivity, such as an encrypted drive, an approved messaging system, or a secure file transfer tool.

The capsule can contain conversation history, source paths, code snippets, command arguments, and tool metadata. Treat it as potentially sensitive.

The repository's `.gitignore` excludes `*.amh` by default to reduce accidental commits of mission history.

### 3. Continue the mission

On the receiving machine, open the local copy of the same project and run:

```bash
amh continue /path/to/mission.amh
```

If the environment is ready, AMH restores the session and prints one native resume command:

```text
Mission restored. Continue with: codex resume <session-id>
```

or:

```text
Mission restored. Continue with: claude --resume <session-id>
```

With the Skill installed, drag the `.amh` file into the receiving Agent and say:

> Continue this task.

The Agent runs the restore flow, resolves safe local differences, and resumes the writable session.

## When AMH Needs Input

The happy path does not require questions. AMH expands the interaction only when it cannot safely infer or validate something, including:

- the current Agent or Session cannot be detected;
- the destination project path is missing;
- the Git remote, branch, or commit differs;
- an observed Skill, MCP server, or CLI is unavailable;
- a local installation, login, permission, or network action requires approval.

After reviewing a confirmable difference, the receiving Agent can continue with:

```bash
amh continue --allow-missing mission.amh
```

`--allow-missing` does not transfer credentials, bypass the Agent's permission model, or make a nonexistent workspace usable.

## What Is In a Capsule

An `.amh` file is a ZIP-compatible, checksummed capsule containing:

```text
manifest.json
mission.json
capabilities.json
workspace.json
checksums.json
session/normalized.json
session/source.jsonl
```

The logical product modules are:

- **Mission Checkpoint**: objective, status, current summary, completed work, risks, and next actions.
- **Session Adapter**: native session import, export, fork, history display, and cross-Agent translation.
- **Workspace Adapter**: source workspace and Git identity mapped to the receiver's local path.
- **Capability Lock**: observed Skills, MCP servers, CLIs, and supporting evidence.
- **Restore Planner**: destination checks and the actions required before continuation.

## What AMH Does Not Transfer

AMH intentionally does not package:

- credentials, API keys, cookies, or login sessions;
- permission grants or approval state;
- the project repository or arbitrary workspace files;
- running processes, containers, terminals, or network connections;
- private model state or hidden reasoning;
- automatic software installation instructions that bypass user approval.

## Documentation

- [User guide and CLI reference](docs/USER_GUIDE.md)
- [Tutorials](docs/tutorials/README.md)
- [Architecture and capsule format](docs/ARCHITECTURE.md)
- [Security policy and trust model](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

The current implementation has been exercised with real Codex and Claude Code sessions, including same-Agent native forks and both cross-Agent directions.

## Limitations

- Cross-Agent restore is a semantic translation, not a byte-exact replay.
- Mission Checkpoint generation is currently deterministic and intentionally minimal.
- Capability discovery is best-effort and limited to directly observed evidence.
- The receiving machine must already contain the project code; AMH stores workspace and Git metadata, not a repository bundle.
- The standalone CLI writes the restored session and prints the resume command. Directly switching a desktop UI task requires host integration rather than a background AMH daemon.

## License

AMH is available under the [MIT License](LICENSE). Third-party attribution is recorded in [`NOTICE`](NOTICE).
