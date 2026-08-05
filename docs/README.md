# RoamBench Documentation

## User Guides

These are the primary docs for deploying and using RoamBench.

- [Configuration Guide](configuration.md) — config file discovery, server/auth/terminal/UI settings
- [Authentication Guide](authentication.md) — password and PAM modes, hash generation
- [Deployment Hardening](deployment-hardening.md) — post-build checklist for safe self-hosted deployment
- [Roadmap](roadmap.md) — planned improvements and non-goals

## Evidence & Positioning

- [Lightweight Evidence](lightweight-evidence.md) — runtime memory and binary size measurements

## Project Control Layer (Design)

The next major evolution of RoamBench. These documents capture the design discussions for adding a structured project control layer on top of the terminal workspace.

- [Project Control Discussions](project-control-discussions/) — full design docs with reading order in its own [README](project-control-discussions/README.md)

## Competitive Analysis

- [cmux Comparison](competitive-analysis/cmux-comparison.md) — feature comparison with manaflow-ai/cmux

## Agent Integrations

- [Overview](agent-integrations/README.md) — quick start and how it works
- [Claude Code](agent-integrations/claude-code.md) — hooks configuration
- [OpenCode](agent-integrations/opencode.md) — integration guide
- [Generic (any agent)](agent-integrations/generic.md) — curl, CLI, and OSC options

## Release Notes

Per-version release copy used for GitHub Releases. See also [CHANGELOG.md](../CHANGELOG.md).

- [v0.4.1](releases/v0.4.1.md)
- [v0.4.0](releases/v0.4.0.md)
- [v0.3.2](releases/v0.3.2.md) · [v0.3.2 中文](releases/v0.3.2-zh.md)
- [v0.3.1](releases/v0.3.1.md) · [v0.3.1 中文](releases/v0.3.1-zh.md)
- [v0.3.0](releases/v0.3.0.md) · [v0.3.0 中文](releases/v0.3.0-zh.md)
- [v0.2.1 中文](releases/v0.2.1-zh.md)
- [v0.2.0](releases/v0.2.0.md)

## Internal / Operational

These docs are for maintainers, not end users.

- [GitHub Publishing Notes](internal/github-publishing.md) — repo metadata, topics, tagline
- [GitHub Release Checklist](internal/github-release-checklist.md) — pre-release verification steps
- [Launch Playbook](internal/launch-playbook.md) — distribution and community posting guide
- [Rebrand Checklist](internal/rebrand-checklist.md) — naming migration tracker
- [Release Packaging](internal/release-packaging.md) — build and bundle flow for GitHub Releases
- [Windows Native Plan](internal/windows-native-plan.md) — Wails + ConPTY desktop app planning
- [Project Control Iteration Plan](internal/project-control-iteration-plan.md) — next steps for auto-progression, agent API, notifications
- [Mobile Interaction and CLI Coordination Plan](internal/mobile-approval-and-cli-coordination-plan-v0.1.md) — interactive phone control, durable decisions, CLI adapters, and cross-CLI messaging
- [Mobile Control Phase 1 Progress](internal/mobile-control-phase-1-progress.md) — SQLite gateway delivered; legacy approval migration and projector still pending

## Branding

- [Branding Assets](branding/) — logos, social preview, QR codes
