# Agent Mission Handoff

**English** | [简体中文](README.zh-CN.md)

Move a writable AI coding mission between machines, teammates, Codex, and Claude Code with one portable `.amh` file.

<p align="center">
  <img src="docs/assets/amh-demo.gif" alt="AMH sender and receiver workflow demo" width="100%">
</p>

### Use It With Your Coding Agent

| Sender | Receiver |
| --- | --- |
| Tell your Agent: **“Package the current task as an AMH file.”** | Attach `mission.amh` and tell your Agent: **“Continue this task.”** |
| The Agent runs `amh pack` and creates one portable `mission.amh` file. | The Agent runs `amh continue mission.amh`, summarizes the restored context, and asks whether to continue. |

Or use the CLI directly:

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

## Agent Or CLI

Every AMH operation supports a conversation with a capable local coding Agent or a direct command.

| Step | Tell your coding Agent | Command line |
| --- | --- | --- |
| Install | “Install AMH from this repository and verify it.” | Run the installer above |
| Package | “Package the current task as an AMH file.” | `amh pack` |
| Inspect | “Inspect this handoff and summarize what it contains.” | `amh inspect mission.amh` |
| Restore | Attach the file and say: “Continue this task.” | `amh continue mission.amh` |

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
