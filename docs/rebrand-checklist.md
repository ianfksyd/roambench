# RoamBench Rebrand Checklist

This checklist separates the public-facing brand rename from the code-level rename.

## Current Decision

- public product name: `RoamBench`
- current repo / binary / config / env identifiers: `liteterm`
- recommended approach: finish the public rebrand first, then do the code-level rename with compatibility

## Phase 1: Public Brand Surface

- update `README.md`, `README.zh-CN.md`, and `README.ja.md`
- update release copy in [docs/github-release-v0.2.0.md](github-release-v0.2.0.md)
- update GitHub metadata notes in [docs/github-publishing.md](github-publishing.md)
- update launch copy in [docs/launch-playbook.md](launch-playbook.md)
- switch screenshot alt text, social preview text, and release titles to `RoamBench`

## Phase 2: GitHub And Release Assets

- decide whether the GitHub repo should remain `liteterm-web` temporarily or rename to `roambench`
- reserve the GitHub org / user path you want to keep long term
- register the public domain you want to use
- update GitHub `About` text and topics
- publish the first release using the `RoamBench` title pattern
- add a note that commands still use `./liteterm` until the code-level rename ships

## Phase 3: Code-Level Rename With Compatibility

- rename the binary from `liteterm` to `roambench`
- keep `liteterm` as a compatibility alias for at least one release
- support both `roambench.toml` and `liteterm.toml`
- support both `ROAMBENCH_*` and `LITETERM_*` environment variables
- support both `~/.config/roambench` and `~/.config/liteterm`
- support both `~/.local/state/roambench` and `~/.local/state/liteterm`
- keep existing browser local storage keys readable so users do not lose workspace state
- review CLI help text, example configs, Makefile targets, and service logs

## Phase 4: Deployment And Packaging

- update systemd unit names if you ship them
- update reverse proxy examples and deployment docs
- update container image names and tags if you publish images
- update shell completion, packaging metadata, and install snippets
- update screenshots and demo assets after the binary rename lands

## Phase 5: Compatibility Window

- document the old and new names in `CHANGELOG.md`
- mark `liteterm` identifiers as deprecated, not removed, in the first rename release
- keep compatibility for at least one minor release
- only remove old identifiers after docs and release notes have warned users clearly

## Recommended Order

1. finish the public-facing docs rebrand
2. reserve domain and GitHub namespace
3. ship a compatibility release with dual names
4. switch defaults to `roambench`
5. remove old `liteterm` identifiers later
