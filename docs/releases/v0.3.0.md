# GitHub Release Copy

Suggested tag:

- `v0.3.0`

Suggested title:

- `RoamBench v0.3.0`

Suggested subtitle:

- `Naming cleanup and smoother viewer drafting`

## English

`v0.3.0` tightens the runtime naming around `roambench` and smooths out the viewer workflow.

Highlights:

- runtime identifiers now align on `roambench`
- empty viewer can start a new draft directly from pasted clipboard text or images
- closing viewer edit mode returns the viewer to an empty state instead of forcing split layout
- opening another preview while a viewer draft has content now goes through the same discard flow

Operational cleanup:

- removed support for `LITETERM_PASSWORD_HASH` and `LITETERM_USER`
- removed fallback reads of `liteterm.toml` and legacy `~/.config/liteterm` / `/etc/liteterm` config locations
- removed the legacy `liteterm_session` cookie fallback
- terminal metadata now persists under `~/.local/state/roambench/terminals` only

Upgrade note:

- if you still rely on `liteterm`-prefixed env vars, config filenames, cookie expectations, or state paths, migrate them to `roambench` before upgrading

Suggested release assets:

- Linux `amd64` tarball
- Linux `arm64` tarball
- matching `.sha256` files for both archives

Short release blurb:

`RoamBench v0.3.0 finishes the runtime naming cleanup and makes the viewer draft flow more direct, clipboard-first, and less disruptive.`
