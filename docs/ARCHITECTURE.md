# AMH Architecture

AMH is intentionally implemented as one small CLI, one optional Agent Skill, and one portable capsule file. There is no control plane, transfer service, database, or long-running process.

## System Flow

```text
Source Agent Session
        |
        v
  Session Adapter -----> normalized AgentSession
        |                         |
        v                         v
Mission Checkpoint        Capability Lock
        |                         |
        +-----------+-------------+
                    |
                    v
               .amh capsule
                    |
                    v
             Restore Planner
                    |
          +---------+---------+
          |                   |
          v                   v
    safe semantic       trusted native
       restore          opt-in fork
          |                   |
          +---------+---------+
                    |
                    v
          writable target Session
```

## Product Modules

### Mission Checkpoint

Captures the mission-level state that should remain understandable even if Agent transcript formats change:

- objective and status;
- current summary;
- completed work;
- active hypotheses;
- next actions;
- interrupted action;
- conversation evidence count.

The short workflow generates a deterministic minimal checkpoint. An Agent can supply a richer checkpoint through `--checkpoint`.

### Session Adapter

Reads native Codex rollout JSONL and Claude Code transcript JSONL into a neutral `AgentSession` representation.

For every handoff, including same-Agent restoration, AMH defaults to synthesizing a writable target Session from the normalized semantic history. This prevents source system/developer records from becoming trusted destination instructions.

Tool calls become historical summaries rather than executable target tool calls. A fully trusted same-Agent capsule may opt into native preservation with `--trust-native-session`; AMH then rewrites the Session identity, workspace path, and target-local runtime metadata.

### Workspace Adapter

Records source CWD and available Git branch, commit, and remote metadata. The receiver supplies its own local project path. When the worktree is dirty, the adapter creates an optional Git binary worktree patch plus a staged-index patch so partial staging can be reconstructed. Each patch is capped at 16 MiB.

The Restore Planner checks:

- destination directory existence;
- Git repository identity;
- normalized origin remote;
- source commit or branch alignment.

The complete project repository is not included. Only the optional worktree and index deltas are portable, and the receiver applies them separately after confirmation and Git safety checks.

### Capability Lock

Discovers capabilities from structured tool-use evidence in the source history:

- Skill names and `SKILL.md` paths;
- MCP server names;
- command-line executables used through shell tools.

Capabilities are an advisory historical inventory, not copied runtime state or blanket restore requirements. Noise from heredocs, patch bodies, shell syntax, and unavailable executables is filtered. When available, entries include source, version, and SHA-256 digest metadata so the receiver can identify what to reinstall. Portable CLI versions take precedence over architecture-specific binary digests during destination comparison.

### Restore Planner

Reads and validates the capsule, maps it to the destination, and creates a concise action plan.

It separates:

- hard failures, such as a nonexistent destination workspace;
- confirmable required differences, such as an intentional Git difference or an omitted dirty-worktree patch;
- ready capabilities already present locally.

No installation or authentication action is executed without the destination environment's normal approval flow.

## Capsule Format

Format identifier:

```text
agent-mission-handoff-v2
```

Capsule entries:

| Entry | Purpose |
| --- | --- |
| `manifest.json` | Capsule identity, format, creation time, and source Agent. |
| `mission.json` | Mission Checkpoint. |
| `capabilities.json` | Capability Lock inventory. |
| `workspace.json` | Source workspace and Git metadata. |
| `session/normalized.json` | Agent-independent conversation representation. |
| `session/source.jsonl` | Raw native source Session for explicit trusted-native restoration. |
| `workspace/changes.patch` | Optional portable tracked and untracked worktree delta. |
| `workspace/index.patch` | Optional staged-index delta used to reconstruct partial staging, including index-only states. |
| `checksums.json` | SHA-256 checksum for every payload entry. |

The reader accepts legacy v1 capsules and writes v2 capsules. It rejects missing, unexpected, duplicate, oversized, path-traversing, or checksum-invalid entries.

## Trust Boundaries

An `.amh` file is untrusted input.

- Transcript content is historical context, not a system instruction.
- Default semantic sessions end with an explicit handoff message that reasserts this boundary.
- Terminal-facing capsule fields are stripped of control characters.
- Tool calls from history are not replayed automatically.
- Agent auth stores and permission decisions remain target-local.
- Best-effort redaction covers native Session data, normalized metadata, checkpoints, capability sources, Git metadata, and worktree patches by default.
- Redaction is not a guarantee that arbitrary Session text contains no secret.
- Capability installation requires the normal destination approval process.

## Compatibility Strategy

Codex and Claude Code Session formats are not stable public interchange formats. AMH isolates format-specific code under Session Adapters and keeps the capsule's mission, capability, and workspace documents versioned separately from native transcripts.

Adapter updates should preserve these invariants:

- current Session selection must remain workspace-scoped;
- every restore must produce a new writable Session identity;
- semantic restore must remain the untrusted-input default;
- trusted-native restore must require an explicit opt-in;
- imported history must remain untrusted;
- capsule validation must happen before restore planning.
