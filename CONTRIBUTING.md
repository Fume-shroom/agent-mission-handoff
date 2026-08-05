# Contributing to AMH

AMH aims to remain a small local-first tool: one CLI, one optional Skill, and one portable file. Contributions should preserve that constraint unless a broader architecture has been explicitly agreed.

## Development Setup

Requirements:

- Go 1.24 or newer;
- Git;
- Codex or Claude Code for adapter integration testing.

Run the standard checks:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Cross-platform build check:

```bash
GOOS=darwin GOARCH=arm64 go build ./cmd/amh
GOOS=linux GOARCH=amd64 go build ./cmd/amh
GOOS=windows GOARCH=amd64 go build ./cmd/amh
```

## Contribution Areas

Useful contributions include:

- Codex and Claude Code adapter compatibility updates;
- additional defensive parsing tests;
- capability discovery precision improvements;
- workspace and Git mapping improvements;
- capsule format documentation;
- tutorials for real Agent-to-Agent workflows;
- support for additional coding Agents through isolated adapters.

## Design Principles

- Keep the happy path at `amh pack` and `amh continue FILE`.
- Do not require a daemon, hosted account, database, or GitHub transfer flow.
- Never intentionally copy auth stores or bypass target-local permissions; treat redaction as best-effort rather than a guarantee.
- Treat imported transcripts and capsule metadata as untrusted.
- Prefer target-local validation over pretending runtime state can be copied.
- Keep safe semantic restore as the default; native same-Agent preservation must remain an explicit trusted opt-in.
- Describe cross-Agent restoration honestly as semantic translation.
- Keep advanced inspection and recovery commands available without exposing them in the normal interaction.

## Pull Requests

Before opening a pull request:

1. keep the change focused on one behavior;
2. add regression tests for parser, safety, or restore changes;
3. run the full test, race, and vet commands;
4. test both sender and receiver behavior when changing the public CLI;
5. update README, user guide, architecture, or tutorials when behavior changes;
6. preserve third-party notices and license files.

For Session Adapter changes, include sanitized fixture shapes or synthetic test data. Do not commit real transcripts containing credentials, proprietary source code, internal paths, or personal conversation history.

## Compatibility Changes

Codex and Claude Code transcript formats can change without notice. Adapter changes should remain defensive:

- skip unknown records instead of crashing when safe;
- preserve current Session and workspace selection rules;
- keep same-Agent native restore writable;
- verify cross-Agent output with the real target Agent when possible;
- add a regression test for every observed format change.

## Licensing

By contributing, you agree that your contribution will be licensed under the repository's MIT License. Preserve attribution for code derived from third-party projects in `NOTICE` and the corresponding `third_party` license directory.
