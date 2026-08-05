# Agent Mission Handoff

**English** | [简体中文](README.zh-CN.md)

Move a writable AI coding mission between machines, teammates, Codex, and Claude Code with one portable `.amh` file.

```bash
# Sender
amh pack

# Receiver
amh continue mission.amh
```

AMH is local-first: no daemon, hosted service, account, database, or GitHub repository is required for the handoff itself.

> Technical preview: Codex and Claude Code session formats are private implementation details and may require adapter updates.

## Install

macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/Fume-shroom/agent-mission-handoff/main/install.ps1 | iex
```

No Go toolchain or source checkout is required. The installer verifies the release checksum and installs the CLI plus the Mission Handoff Skill for Codex and Claude Code.

You can also tell a local coding Agent:

> Install AMH from https://github.com/Fume-shroom/agent-mission-handoff and verify the installation.

## CLI Or Agent

Every AMH operation supports a direct command or a conversation with a capable local coding Agent.

| Step | Command line | Tell your coding Agent |
| --- | --- | --- |
| Install | Run the installer above | “Install AMH from this repository and verify it.” |
| Package | `amh pack` | “Package the current task as an AMH file.” |
| Inspect | `amh inspect mission.amh` | “Inspect this handoff and summarize what it contains.” |
| Restore | `amh continue mission.amh` | Attach the file and say: “Continue this task.” |

The Agent route still uses the local `amh` CLI internally. It does not require a cloud AMH service.

## Receiver Experience

`amh continue` restores a writable Session and prints a concise Mission Brief with:

- the original objective and current status;
- restored history counts and recent conversation context;
- completed work, unresolved questions, and suggested next actions;
- missing Skills, MCP servers, CLIs, or workspace requirements;
- the native resume command.

When a coding Agent receives the file, it additionally reads the complete portable transcript and responds first with a higher-quality Mission Brief. It then asks whether to continue. It must not run mission tools or modify project files until the user confirms.

The complete history remains available in the restored writable Session. The brief summarizes it instead of dumping the entire transcript into the chat.

## What Travels

One `.amh` capsule contains:

- Mission Checkpoint: objective, progress, completed work, risks, and next actions;
- portable conversation history and the native source Session;
- workspace and Git identity;
- observed Skills, MCP servers, and CLI tools;
- checksums and archive safety metadata.

AMH does **not** transfer credentials, permission grants, login state, running processes, private model state, or the project repository.

## Supported Handoffs

| Source | Destination | Restore mode |
| --- | --- | --- |
| Codex | Codex | Native writable fork |
| Claude Code | Claude Code | Native writable fork |
| Codex | Claude Code | Semantic session translation |
| Claude Code | Codex | Semantic session translation |

Cross-Agent restore preserves useful conversation and mission context. It does not claim byte-exact reproduction of private runtime state or tool-call internals.

## Documentation

- [User guide and CLI reference](docs/USER_GUIDE.md)
- [Tutorials](docs/tutorials/README.md)
- [Architecture and capsule format](docs/ARCHITECTURE.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## License

MIT. Third-party attribution is recorded in [NOTICE](NOTICE).
