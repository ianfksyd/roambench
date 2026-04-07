# Contributing to RoamBench

Thanks for contributing.

RoamBench is intentionally small and opinionated: single-user, self-hosted, terminal-first, and usable from mobile browsers. Contributions that preserve that focus are the best fit for the project.

`RoamBench` is the public-facing product name. See setup snippets below for how to point docs at your local binary and config paths during the current transition.

## Before You Start

- Check [README.md](README.md), [docs/roadmap.md](docs/roadmap.md), and existing issues before opening a new proposal.
- For security issues, do not open a public issue. Follow [SECURITY.md](SECURITY.md).
- Large behavior changes should explain why they fit the project's core model instead of turning RoamBench into a full browser IDE or a multi-user platform.

## Local Development

Typical setup:

```bash
make build
APP_BIN=<path-to-binary>         # e.g. ./roambench
APP_CONFIG=<path-to-config-file>  # e.g. ./roambench.toml
cp configs/roambench.example.toml "$APP_CONFIG"
"$APP_BIN" --password-hash
"$APP_BIN" --config "$APP_CONFIG"
```

Useful commands:

- `make build`
- `make build-pam`
- `make run`
- `go test ./...`
- `node --check web/js/app.js`

Notes:

- front-end assets under `web/` are embedded into the Go binary
- after changing front-end files, rebuild and restart the service to verify the updated UI
- `tmux` is strongly recommended during development because it matches the intended persistence model

## What Good Changes Look Like

- Keep the single-user model intact.
- Prefer terminal-first workflows over IDE-style feature sprawl.
- Keep mobile and small-screen usability in mind.
- Avoid adding heavy dependencies unless the value is clear.
- Keep behavior predictable across refresh, reconnect, and restart.
- Update docs when user-facing behavior changes.

## Testing Expectations

Before opening a pull request, run what is relevant:

- `go test ./...`
- `node --check web/js/app.js`
- manual browser checks for UI changes

If your change affects:

- terminal behavior: test both normal interaction and reconnect behavior
- file tools: test with nested paths and destructive flows carefully
- workspace behavior: test refresh and cross-browser recovery when possible

## Pull Requests

Keep pull requests focused and easy to review.

Please include:

- a short summary of the problem and change
- any user-facing impact
- screenshots or recordings for UI changes
- the commands you ran to verify the change
- notes about config, upgrade, or restart implications when relevant

Try to avoid mixing feature work with unrelated cleanup.

## Documentation And Changelog

Update documentation when behavior or positioning changes.

Common files to touch:

- [README.md](README.md)
- [README.zh-CN.md](README.zh-CN.md)
- [README.ja.md](README.ja.md)
- [CHANGELOG.md](CHANGELOG.md)
- files under [docs](docs)

## Questions

If you are unsure whether an idea fits the project, open an issue or draft pull request and explain the workflow you are trying to improve.
