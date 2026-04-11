# RoamBench Rebrand Checklist

This checklist separates the public-facing brand rename from the code-level rename.

## Current Decision

- public product name: `RoamBench`
- current repo / binary / config / env identifiers: `roambench`
- recommended approach: finish the public rebrand first, then do the code-level rename with compatibility

## Phase 1: Public Brand Surface

- update `README.md`, `README.zh-CN.md`, and `README.ja.md`
- update release copy in [docs/github-release-v0.2.0.md](github-release-v0.2.0.md)
- update GitHub metadata notes in [docs/github-publishing.md](github-publishing.md)
- update launch copy in [docs/launch-playbook.md](launch-playbook.md)
- switch screenshot alt text, social preview text, and release titles to `RoamBench`

## Phase 2: GitHub And Release Assets

- decide whether the GitHub repo should remain `roambench-web` temporarily or rename to `roambench`
- reserve the GitHub org / user path you want to keep long term
- register the public domain you want to use
- update GitHub `About` text and topics
- publish the first release using the `RoamBench` title pattern
- add a note that commands still use `./roambench` until the code-level rename ships

## Phase 3: Code-Level Rename Cleanup

- keep the binary, config, state directory, cookies, and browser storage keys aligned on `roambench`
- remove remaining pre-rebrand aliases from startup, auth, persistence, and packaging flows
- review CLI help text, example configs, Makefile targets, and service logs

## Phase 4: Deployment And Packaging

- update systemd unit names if you ship them
- update reverse proxy examples and deployment docs
- update container image names and tags if you publish images
- update shell completion, packaging metadata, and install snippets
- update screenshots and demo assets after the binary rename lands

## Phase 5: Final Verification

- document the naming cleanup in `CHANGELOG.md`
- verify configs, release assets, and local state paths only use `roambench`
- check browser storage migration expectations before removing fallback readers

## Recommended Order

1. finish the public-facing docs rebrand
2. reserve domain and GitHub namespace
3. ship a release that consolidates remaining names
4. switch defaults to `roambench`
5. remove leftover pre-rebrand identifiers later
