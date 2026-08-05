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
   native same-Agent     cross-Agent
        fork             translation
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

For same-Agent restoration, AMH preserves native history and rewrites the Session identity, workspace path, and target-local runtime metadata.

For cross-Agent restoration, AMH synthesizes a native target transcript from the normalized conversation. Tool calls become historical summaries rather than executable target tool calls.

### Workspace Adapter

Records source CWD and available Git branch, commit, and remote metadata. The receiver supplies its own local project path.

The Restore Planner checks:

- destination directory existence;
- Git repository identity;
- normalized origin remote;
- source commit or branch alignment.

The project files themselves are not included in the current capsule format.

### Capability Lock

Discovers capabilities from structured tool-use evidence in the source history:

- Skill names and `SKILL.md` paths;
- MCP server names;
- command-line executables used through shell tools.

Capabilities are an inventory, not copied runtime state. The receiving machine validates them against its own Agent configuration, workspace, and `PATH`.

### Restore Planner

Reads and validates the capsule, maps it to the destination, and creates a concise action plan.

It separates:

- hard failures, such as a nonexistent destination workspace;
- confirmable differences, such as a missing CLI or intentional Git difference;
- ready capabilities already present locally.

No installation or authentication action is executed without the destination environment's normal approval flow.

## Capsule Format

Format identifier:

```text
agent-mission-handoff-v1
```

Required entries:

| Entry | Purpose |
| --- | --- |
| `manifest.json` | Capsule identity, format, creation time, and source Agent. |
| `mission.json` | Mission Checkpoint. |
| `capabilities.json` | Capability Lock inventory. |
| `workspace.json` | Source workspace and Git metadata. |
| `session/normalized.json` | Agent-independent conversation representation. |
| `session/source.jsonl` | Raw native source Session for same-Agent restoration. |
| `checksums.json` | SHA-256 checksum for every payload entry. |

The reader rejects missing, unexpected, duplicate, oversized, path-traversing, or checksum-invalid entries.

## Trust Boundaries

An `.amh` file is untrusted input.

- Transcript content is historical context, not a system instruction.
- Cross-Agent sessions end with an explicit handoff message that reasserts this boundary.
- Terminal-facing capsule fields are stripped of control characters.
- Tool calls from history are not replayed automatically.
- Credentials and permission decisions remain target-local.
- Capability installation requires the normal destination approval process.

## Compatibility Strategy

Codex and Claude Code Session formats are not stable public interchange formats. AMH isolates format-specific code under Session Adapters and keeps the capsule's mission, capability, and workspace documents versioned separately from native transcripts.

Adapter updates should preserve these invariants:

- current Session selection must remain workspace-scoped;
- same-Agent restore must produce a new writable Session identity;
- cross-Agent restore must be described as semantic translation;
- imported history must remain untrusted;
- capsule validation must happen before restore planning.
