# RoamBench Launch Playbook

This document turns the product positioning into launch material you can actually use.

Public product name: `RoamBench`
Current technical identifiers: `roambench`

## Core Message

Use this as the top-level promise:

- English: `Keep your AI coding sessions running. Reconnect from anywhere.`
- 中文：`让 AI coding session 一直跑着，你随时都能从别的设备接回。`
- 日本語: `AI coding session を走らせ続けたまま、どこからでも復帰できる。`

Back it up with these three points:

- keep terminal sessions alive with `tmux`
- run `2 / 4`-pane workspaces for multiple CLI tools side by side
- inspect output and make small file edits without a heavy browser IDE

If the launch materials do not yet include a real mobile screenshot or GIF, keep phone as a supporting use case, not the main headline.

## README Structure

The README should feel like a product landing page for a technical buyer.

Recommended section order:

1. one-line hook
2. three proof bullets
3. use cases people instantly recognize
4. comparison table against SSH and heavier browser IDEs
5. screenshots
6. fast path install

If a section reads like a feature dump, compress it until the use case is obvious again.

## Demo Story

The homepage demo should show one workflow, not a feature checklist.

Recommended sequence:

1. Desktop view with `4` panes running Codex, Claude Code, Kimi-CLI, and OpenCode side by side.
2. A file pane open next to the terminals to show lightweight edits and file copy.
3. Phone view reconnecting to the same workspace and checking task progress.
4. A small edit or command from the phone, then back to watching long-running output.

That sequence proves:

- split-view concurrency
- cross-device recovery
- mobile usefulness
- terminal-first agent workflows

## Demo Assets To Capture

Before launch, capture at least these assets:

- `docs/screenshot-main.png`
  - desktop screenshot
  - use `4` panes
  - label panes clearly with real tool names
- `docs/screenshot-file-browser.png`
  - supporting desktop screenshot
  - show the built-in file browser or editor next to the terminal workflow
- `docs/screenshot-mobile.jpg`
  - phone screenshot reconnecting to the same session
- `docs/screenshot-mobile-login.jpg`
  - optional supporting login screenshot for mobile access flow
- `docs/demo-resume.gif`
  - short clip: start task on desktop, reopen on phone, inspect output, make a small file change

Asset rules:

- show real task output, not placeholder lorem ipsum
- keep one terminal visibly busy so the viewer understands this is for long-running work
- keep the file browser visible in at least one asset
- avoid showing secret paths, tokens, or private prompts

## Proof That It Is Lighter

Do not claim exact performance wins unless you measured them. Instead, prepare concrete proof points.

Recommended evidence:

- binary size
- idle memory footprint
- reconnect speed after refresh
- recovery after restarting the RoamBench service with `tmux`
- side-by-side screenshot against a heavier browser IDE if you want a stronger comparison

Suggested commands and checks:

```bash
ls -lh roambench
ps -o pid,rss,command -p <pid>
go test ./...
node --check web/js/app.js
```

Suggested evidence table for launch notes:

| Metric | How To Measure | Current Value | Notes |
| --- | --- | --- | --- |
| Binary size | `ls -lh roambench` | fill in | after `make build` |
| Idle RSS | `ps -o rss= -p <pid>` | fill in | after login and idle |
| Refresh recovery | manual | fill in | same terminal still attached |
| Restart recovery | manual with `tmux` | fill in | restart the RoamBench service only |
| Mobile reconnect | manual | fill in | reopen same workspace on phone |

## Distribution Plan

Focus on channels where self-hosted, terminal-heavy, and AI-coding users already live.

### GitHub

What to do:

- keep the English README as the default landing page
- keep the README screenshot-heavy near the top
- add a comparison table near the top (`SSH` vs browser IDE vs RoamBench)
- publish the current public build as the next tagged release (currently `v0.3.2`)
- keep the repo description tight and technical

Angle:

- terminal-first remote workbench
- mobile reconnection to long-running agent workflows
- lightweight alternative to heavy browser IDE setups

### Hacker News

Use a concrete title, not marketing language.

Good title options:

- `Show HN: RoamBench, a small self-hosted remote workbench between SSH and browser IDEs`
- `Show HN: RoamBench, reconnect terminal-first coding workflows from anywhere`
- `Show HN: RoamBench, a lightweight remote workbench for terminal-first coding`

Opening paragraph structure:

1. what it is
2. why you built it
3. what it deliberately does not try to be

### Reddit / selfhosted

Lead with the pain point:

- long-running terminal workflows
- wanting phone access without a heavy web IDE
- keeping `tmux` sessions and split views easy to reopen

Suggested post angle:

`I wanted a lighter way to reconnect to Codex / Claude Code / other CLI tools from my phone without running a full browser IDE, so I built RoamBench.`

### AI Coding Communities

Emphasize:

- Codex / Claude Code / Kimi-CLI / OpenCode side-by-side
- phone-friendly supervision
- low overhead compared with heavier remote IDE stacks

### Chinese Developer Communities

Lead with the use case, not the architecture.

Good title directions:

- `我做了一个能在手机上接回 Codex / Claude Code 的轻量远程工作台`
- `一个适合 vibe coding 的自托管轻量远程工作台`
- `在手机上盯 Codex、Claude Code、Kimi-CLI 的轻量方案`

Post structure:

1. why SSH alone was not enough
2. why full browser IDEs felt too heavy
3. what RoamBench keeps
4. phone and split-view demo
5. install link and screenshot

## Installation Friction

The launch path should optimize for "can try it in minutes", not for teaching every deployment option first.

Recommended order:

1. show the fast path first
2. put the secure full setup right after it
3. move advanced configuration deeper into docs

Current fast-path file:

- [configs/roambench.quickstart.toml](../configs/roambench.quickstart.toml)

Current primary docs:

- [README.md](../README.md)
- [docs/lightweight-evidence.md](lightweight-evidence.md)
- [docs/configuration.md](configuration.md)
- [docs/github-publishing.md](github-publishing.md)

## Final Pre-Launch Checklist

- README hero line is sharp
- README opening is scenario-led, not feature-led
- comparison table is visible without much scrolling
- desktop screenshot shows `2 / 4` split views clearly
- file browser screenshot exists
- if the hero is phone-led, mobile screenshot exists
- release notes mention Codex / Claude Code style workflows
- quickstart path works without editing multiple config fields
- one concrete "lightweight" proof point is ready

## Launch Automation

Use this helper to generate community drafts and (optionally) post to X when credentials are configured:

- Script: `scripts/publish-roambench.sh`
- Default behavior: `--dry-run` style output (no API write)
- X send mode: add `--send` and set `ROAMBENCH_X_BEARER_TOKEN`
- Example:

```bash
scripts/publish-roambench.sh --tag v0.3.2 --targets x,reddit,hn,ph,v2ex,cn --dry-run
```

Script output includes:
- X copy (ready to post, two-part text)
- Reddit draft
- Hacker News draft
- Product Hunt draft
- Chinese community draft for V2EX / Chinese channels
