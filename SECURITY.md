# Security Policy

RoamBench is a single-user self-hosted tool, but it still exposes authentication, terminal, and file-management surfaces. Issues in those areas should be treated seriously.

`RoamBench` is the public-facing product name. Current technical identifiers still use `liteterm`.

## Supported Versions

Security fixes are best-effort for:

- the latest state of the default branch
- the most recent tagged release

Older versions may not receive fixes.

## How To Report A Vulnerability

Please do not open public issues for security vulnerabilities.

Preferred process:

1. Use GitHub private vulnerability reporting or a repository security advisory if it is enabled.
2. If that is not available, contact the maintainer through a private channel before disclosure.
3. Include:
   - affected version, commit, or deployment details
   - impact
   - reproduction steps
   - any mitigation ideas you already have

If you cannot find a private reporting path, open a minimal public issue without exploit details and ask for a secure contact route.

## Response Targets

These are best-effort goals, not guarantees:

- acknowledgement within 7 days
- initial triage within 30 days
- coordinated disclosure after a fix or mitigation is available

## In Scope

Examples of in-scope reports:

- authentication bypass
- session fixation or session hijacking
- privilege escalation beyond the configured single Unix user
- path traversal or unintended file access outside the intended home-rooted workspace
- command execution that escapes the expected terminal model
- sensitive data exposure in logs, APIs, or persisted state

## Out Of Scope

Examples usually out of scope:

- deployments that explicitly disable TLS or widen IP exposure and are then reachable over unsafe networks
- local compromise of the host operating system
- denial of service caused by the owner's own workload on a single-user instance
- general hardening advice without a concrete vulnerability

## Safe Testing

Only test against systems you own or are authorized to assess. Avoid destructive testing on shared or production systems.
