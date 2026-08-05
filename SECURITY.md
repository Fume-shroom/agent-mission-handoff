# Security Policy

## Trust Model

An `.amh` capsule must be treated as untrusted input, even when it comes from a teammate. It can contain conversation text, code snippets, command arguments, workspace metadata, and capability names from another environment.

AMH validates archive structure and checksums, but integrity is not the same as authenticity. A valid capsule proves that its internal payloads are consistent with its checksum file; it does not prove who created it.

## Security Boundaries

AMH does not intentionally copy or reconstruct:

- Agent authentication stores, cookies, or login sessions;
- local permission grants or prior approval decisions;
- private model state or hidden reasoning;
- running processes or network connections;
- automatic authorization to install Skills, MCP servers, CLIs, or packages.

The receiving Agent and each local capability retain responsibility for authentication, authorization, sandboxing, and user approval.

Native Session content, normalized conversation text, command arguments, Mission Checkpoints, Git remotes, capability sources, and optional worktree patches can contain credentials or other sensitive values. AMH applies conservative high-confidence redaction by default and records the number of replacements in the manifest. This is best-effort filtering, not proof that a capsule is secret-free. `--include-sensitive` disables this filter and should be used only for an explicitly reviewed local workflow.

AMH redacts matches in newly added patch lines because that does not change the source-side context required by `git apply`. If a match occurs in patch context, removed text, a private-key block, or binary data, AMH omits the patch rather than creating a misleading, corrupted, or inapplicable delta. The capsule still records that the source worktree was dirty and why its patch was omitted.

Restore uses the normalized semantic history by default, including same-Agent handoffs. This prevents native source system/developer records from retaining instruction priority in the destination Agent. `--trust-native-session` is an explicit same-Agent escape hatch for a locally created capsule whose native Session records the receiver has fully reviewed and trusts.

## Capsule Protections

The current reader enforces:

- a fixed set of allowed archive entries;
- safe relative entry names and Zip Slip protection;
- duplicate-entry rejection;
- per-entry and total uncompressed size limits;
- checksums covering every required payload;
- format-version validation;
- terminal control-character sanitization for displayed capsule fields;
- default high-confidence redaction across Session, checkpoint, capability, workspace, and patch payloads.

Imported transcript content is historical evidence. Default restore does not replay source tool calls or import source system/developer records as trusted target instructions, and the restored Session ends with an explicit instruction to validate the local environment before continuing.

## Safe Usage

- Review the sensitivity of a Session before packaging it.
- Inspect the resulting capsule even when the redaction count is nonzero.
- Transfer capsules through an approved secure channel.
- Inspect and preflight capsules received from unfamiliar sources.
- Do not disable validation to accept a damaged capsule.
- Review Skill and MCP installation sources before approving them.
- Re-run security-sensitive commands locally rather than trusting historical output.
- Do not use `--trust-native-session` for an unreviewed capsule from another person or environment.
- Delete capsules according to the same retention policy used for source code and debugging logs.

## Reporting a Vulnerability

Do not open a public issue for an undisclosed vulnerability.

Until a dedicated security contact is configured for the repository, report vulnerabilities privately to the repository maintainers through GitHub's private vulnerability reporting feature. Include:

- affected AMH version or commit;
- reproduction steps or a minimal capsule;
- expected and actual behavior;
- security impact;
- any suggested mitigation.

Maintainers should acknowledge a report before publishing technical details or a fix timeline.

## Supported Versions

AMH is currently a technical preview. Security fixes are applied to the latest development version until the project publishes a stable release policy.
