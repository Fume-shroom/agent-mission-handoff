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

Verify the installation with:

```bash
amh doctor
```

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
| Apply source changes | “Apply the source worktree changes from this handoff.” | `amh apply mission.amh` |
| Verify | “Check whether AMH is ready.” | `amh doctor` |
| Update | “Update AMH and verify it.” | `amh update` |
| Remove | “Uninstall AMH.” | `amh uninstall` |

The Agent route still uses the local `amh` CLI internally. It does not require a cloud AMH service.

## Receiver Experience

`amh continue` restores a writable Session and prints a concise Mission Brief with:

- the original objective and current status;
- restored history counts and recent conversation context;
- completed work, unresolved questions, and suggested next actions;
- missing Skills, MCP servers, CLIs, or workspace requirements;
- whether portable source workspace changes and staged state are available;
- the native resume command.

When a coding Agent receives the file, it opens the restored Session. The restored Agent reads the portable transcript, responds first with a higher-quality Mission Brief, and asks whether to continue. It must not run mission tools, apply source changes, or modify project files until the user confirms.

The portable history remains available in the restored writable Session. The brief summarizes it instead of dumping the entire transcript into the chat.

## What Travels

One `.amh` capsule contains:

- Mission Checkpoint: objective, progress, completed work, risks, and next actions;
- portable conversation history and the native source Session;
- workspace and Git identity, plus optional checked patches for uncommitted tracked/untracked files and staged state;
- observed Skills, MCP servers, and CLI tools with available source, version, and digest metadata;
- checksums and archive safety metadata.

AMH does not intentionally copy Agent auth stores, login state, permission grants, running processes, private model state, or the complete project repository. Session text, command history, checkpoints, patches, and Git metadata can still contain sensitive values. AMH applies best-effort high-confidence redaction by default, but this is not proof that a capsule is secret-free. Review it and transfer it through an approved secure channel.

Source workspace changes are never applied automatically. The receiver Agent reports them and waits for confirmation before running `amh apply`.
If AMH redacts added patch content, the Mission Brief reports the replacement count so the receiver can review `[REDACTED]` placeholders before testing or committing.
If file content is portable but the exact staged state is not, the Mission Brief calls that out explicitly instead of claiming a complete workspace restore.

## Maintain

```bash
amh doctor      # verify CLI, Agents, Skills, and Session roots
amh update      # install the latest verified release in place
amh uninstall   # remove the CLI and both Mission Handoff Skills
```

## Supported Handoffs

| Source | Destination | Restore mode |
| --- | --- | --- |
| Codex | Codex | Safe writable semantic restore by default |
| Claude Code | Claude Code | Safe writable semantic restore by default |
| Codex | Claude Code | Semantic session translation |
| Claude Code | Codex | Semantic session translation |

Semantic restore preserves useful conversation and mission context without importing source system/developer records as trusted target instructions. For a capsule you fully trust, `--trust-native-session` enables a same-Agent native fork. AMH does not claim byte-exact reproduction of private runtime state or tool-call internals.

## Documentation

- [User guide and CLI reference](docs/USER_GUIDE.md)
- [Tutorials](docs/tutorials/README.md)
- [Architecture and capsule format](docs/ARCHITECTURE.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

## License

MIT. Third-party attribution is recorded in [NOTICE](NOTICE).
