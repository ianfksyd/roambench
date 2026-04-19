# GitHub Release Checklist

This file is the practical release-prep checklist for publishing RoamBench on GitHub.

It is intentionally short and operational: use it to decide what is already ready, what must be fixed before the first public push, and what should be done in the GitHub UI on release day.

## Ready Now

These pieces are already present in the repo:

- English-first product README: [README.md](../../README.md)
- Simplified Chinese README: [README.zh-CN.md](../../README.zh-CN.md)
- Short Japanese README: [README.ja.md](../../README.ja.md)
- Release-note draft: [docs/releases/v0.3.2.md](../releases/v0.3.2.md)
- Release packaging guide: [docs/internal/release-packaging.md](release-packaging.md)
- GitHub publishing metadata: [docs/internal/github-publishing.md](github-publishing.md)
- Launch/distribution playbook: [docs/internal/launch-playbook.md](launch-playbook.md)
- Branding assets, repo avatar candidate, and social preview: [docs/branding](branding)
- Main screenshot: [docs/screenshot-main.png](../screenshot-main.png)
- Lightweight runtime evidence: [docs/lightweight-evidence.md](lightweight-evidence.md)
- Changelog: [CHANGELOG.md](../../CHANGELOG.md)
- License: [LICENSE](../../LICENSE)
- Contributing guide: [CONTRIBUTING.md](../../CONTRIBUTING.md)
- Security policy: [SECURITY.md](../../SECURITY.md)
- Issue templates and PR template: [.github](../../.github)
- Quickstart config for trusted local/LAN trials: [configs/roambench.quickstart.toml](../../configs/roambench.quickstart.toml)

## Blockers Before Public Push

These are the items to clear before the repo is made public.

### 1. Confirm local-only files are not tracked

Do not publish machine-specific or secret-bearing local files.

Must stay local:

- `roambench.lan.toml`
- built binary `roambench`
- local binary variants such as `roambench.new` and `roambench.next`
- `.claude/`
- editor scratch files such as `editor-test.txt`
- draft screenshots and issue-triage assets under `issues/`
- server-specific branding or access assets that contain a real host, QR code, or private route

Current note:

- `.gitignore` already excludes the key local configs, local binaries, `issues/`, and server-specific branding/access assets
- that is only safe if they were never added to git
- before the public push, confirm they are not tracked in the real git repo

### 2. Decide the first public version number

Current repo state is ahead of the historical `0.1.0` baseline.

Why this matters:

- [CHANGELOG.md](../../CHANGELOG.md) already has a substantial `Unreleased` section
- [README.md](../../README.md) describes features that were not part of the original `0.1.0`
- [docs/releases/v0.2.0.md](../releases/v0.2.0.md) already matches the current public-release target better than a `v0.1.0` draft

Recommendation:

- If this exact repo state is the first public GitHub release, prefer `v0.2.0`
- Only keep `v0.1.0` if you intentionally want a smaller first tag and are willing to rewrite the release notes and changelog to match

### 3. Verify the current runtime behavior on a clean browser

Before the public screenshots and release text are considered final, verify:

- the app entry URL is the root path `/`, not `/home/{username}`
- a newly created terminal starts in `~/`
- multi-view workspaces still restore correctly
- the file browser, viewer, and editor behave correctly after a hard refresh
- mobile access still works for the current layout rules

### 4. Decide whether to publish under `roambench-web` or rename the repo now

Current product name:

- `RoamBench`

Current technical identifiers still in code/config:

- `roambench`

Recommendation:

- repo display name can be `RoamBench`
- repo slug can stay `roambench-web` temporarily if you want less churn
- if you want cleaner public branding, rename the GitHub repo to `roambench`

### 5. Decide whether `CODE_OF_CONDUCT.md` is needed now

Not a blocker for a small founder-led release, but worth adding if you want the repo to look more like a standard open-source project from day one.

## GitHub UI Setup

Set these in the GitHub repository settings:

- Repository name: `roambench` if you are ready to rename, otherwise keep `roambench-web`
- Description: use the recommended line from [docs/internal/github-publishing.md](github-publishing.md)
- Website: optional, only if you have a project homepage
- Topics:
  - `web-terminal`
  - `self-hosted`
  - `tmux`
  - `golang`
  - `browser-terminal`
  - `remote-workspace`
  - `vibe-coding`
  - `ai-agents`

Branding assets to upload:

- Repo avatar: [docs/branding/roambench-icon.svg](../branding/roambench-icon.svg)
- Social preview image: [docs/branding/roambench-social-preview.png](../branding/roambench-social-preview.png)

## First Push Checklist

Before pushing the repo public:

1. Make sure no local config, binary, or scratch file is tracked.
2. Make sure the README opens cleanly on GitHub and images render correctly.
3. Make sure the repo root does not depend on private paths or local absolute references.
4. Make sure the chosen release version matches the actual changelog state.
5. Make sure `LICENSE`, `CONTRIBUTING`, `SECURITY`, issue templates, and PR template are visible in the root or `.github`.

## First Release Checklist

Recommended sequence:

1. Push the repo.
2. Set repo description, topics, avatar, and social preview.
3. Pick the first public tag:
   - recommended: `v0.2.0` for the current repo state
4. Update [CHANGELOG.md](../CHANGELOG.md) so the release section matches the chosen tag.
5. Keep the draft release notes aligned with the final tag and changelog.
6. Create the release on GitHub using the copy from [docs/releases/v0.2.0.md](../releases/v0.2.0.md), adjusted if you make last-minute release-note edits.
7. Follow the posting sequence in [docs/internal/launch-playbook.md](launch-playbook.md).

## Nice To Have, Not Blocking

- add `CODE_OF_CONDUCT.md`
- add a favicon `.ico` variant for broader browser compatibility
- finish the code-level rename from `roambench` to `RoamBench`
- add a short demo GIF or video after the static screenshots

## Final Recommendation

If you want the cleanest first public launch:

- publish from the current repo state as `RoamBench`
- keep the web entry URL at `/`
- verify new terminals start in `~/`
- make sure local LAN config and binary artifacts are not tracked
- ship the first public tag as `v0.2.0`, not `v0.1.0`
