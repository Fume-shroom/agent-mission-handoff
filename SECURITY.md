# Security Policy

## Trust Model

An `.amh` capsule must be treated as untrusted input, even when it comes from a teammate. It can contain conversation text, code snippets, command arguments, workspace metadata, and capability names from another environment.

AMH validates archive structure and checksums, but integrity is not the same as authenticity. A valid capsule proves that its internal payloads are consistent with its checksum file; it does not prove who created it.

## Security Boundaries

AMH does not transfer or attempt to reconstruct:

- credentials, tokens, cookies, or login sessions;
- local permission grants or prior approval decisions;
- private model state or hidden reasoning;
- running processes or network connections;
- automatic authorization to install Skills, MCP servers, CLIs, or packages.

The receiving Agent and each local capability retain responsibility for authentication, authorization, sandboxing, and user approval.

## Capsule Protections

The current reader enforces:

- a fixed set of allowed archive entries;
- safe relative entry names and Zip Slip protection;
- duplicate-entry rejection;
- per-entry and total uncompressed size limits;
- checksums covering every required payload;
- format-version validation;
- terminal control-character sanitization for displayed capsule fields.

Imported transcript content is historical evidence. AMH does not replay source tool calls, and a restored cross-Agent Session ends with an explicit instruction to validate the local environment before continuing.

## Safe Usage

- Review the sensitivity of a Session before packaging it.
- Transfer capsules through an approved secure channel.
- Inspect and preflight capsules received from unfamiliar sources.
- Do not disable validation to accept a damaged capsule.
- Review Skill and MCP installation sources before approving them.
- Re-run security-sensitive commands locally rather than trusting historical output.
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
