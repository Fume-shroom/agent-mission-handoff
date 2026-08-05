# AMH Tutorials

These tutorials use the public two-command workflow. Replace example paths and file names with values from your environment.

## Tutorial 1: Codex to Claude Code

Use this when an investigation starts in Codex and should continue in Claude Code.

### On the Codex machine

Open the active project and tell Codex:

> Package the current task as an AMH file.

With the Skill installed, Codex runs:

```bash
amh pack -o incident.amh
```

Expected result:

```text
Packed current codex mission to incident.amh
```

Transfer `incident.amh` through an approved channel.

### On the Claude Code machine

Open the local copy of the same project. Give Claude Code the file and say:

> Continue this task.

Claude Code runs:

```bash
amh continue --to claude /path/to/incident.amh
```

If all checks pass, AMH prints:

```text
Mission restored. Continue with: claude --resume <session-id>
```

Claude Code resumes that Session and continues from the final Mission Checkpoint.

Before running tools, the receiving Agent summarizes the restored objective, important history, completed work, unresolved items, and next action, then asks whether to continue.

If capabilities are missing, Claude Code should show the concise list, request approval once, resolve what it safely can, and retry.

## Tutorial 2: Claude Code to Codex

Use this when work starts in Claude Code and should continue in Codex.

### On the Claude Code machine

Tell Claude Code:

> Package the current task as an AMH file.

Claude Code runs:

```bash
amh pack -o feature-handoff.amh
```

Transfer the resulting file.

### On the Codex machine

Open the destination project, attach `feature-handoff.amh`, and say:

> Continue this task.

Codex runs:

```bash
amh continue --to codex /path/to/feature-handoff.amh
```

AMH creates a writable Codex Session and prints:

```text
Mission restored. Continue with: codex resume <session-id>
```

Codex resumes the Session, treats the imported transcript as historical context, and re-runs local tools when fresh evidence is needed.

Before running tools, Codex presents a Mission Brief from the complete imported transcript and asks whether to continue. In Codex Desktop it opens the restored task through native task navigation rather than launching an interactive CLI inside a background terminal.

## Tutorial 3: Move a Session to Another Machine

Use this at the end of the day when the same Agent and project exist on another development machine.

### Source machine

From the active project:

```bash
amh pack -o daily-checkpoint.amh
```

Transfer both of these independently:

- the project code using your normal source-control or workspace process;
- `daily-checkpoint.amh` using an approved file channel.

AMH does not package the repository itself.

### Destination machine

Check out the expected source revision and enter the project directory:

```bash
git status
git rev-parse HEAD
```

Dry-run the handoff first if the environment differs significantly:

```bash
amh continue --dry-run /path/to/daily-checkpoint.amh
```

Restore it:

```bash
amh continue /path/to/daily-checkpoint.amh
```

Because the destination Agent matches the source Agent, AMH creates a native writable fork that preserves the original native history.

## Tutorial 4: Teammate Handoff with a Missing Skill

The receiving Agent may report:

```text
MISSING skill      incident-debug
```

Recommended flow:

1. The receiving Agent explains that the Skill was observed in the source mission.
2. The teammate reviews the Skill source and installation method.
3. The teammate approves installation using the destination Agent's normal process.
4. The Agent retries `amh continue mission.amh`.

If the Skill is not needed for the remaining work, the teammate can explicitly approve continuation:

```bash
amh continue --allow-missing mission.amh
```

AMH does not copy or execute the source Skill automatically.

## Tutorial 5: Inspect Before Restoring

For a capsule received through an unfamiliar channel:

```bash
amh inspect mission.amh
amh preflight --to codex --cwd /work/project mission.amh
amh continue --dry-run --to codex --cwd /work/project mission.amh
```

These commands validate the capsule and show the restore plan without creating a Session. Continue only after the capsule source and destination differences are understood.
