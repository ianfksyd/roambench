# GitHub Release Copy

Suggested tag:

- `v0.3.1`

Suggested title:

- `RoamBench v0.3.1`

Suggested subtitle:

- `Mobile proof, file type badges, and reusable release packaging`

## English

`v0.3.1` makes the project easier to evaluate and easier to ship.

Highlights:

- README and launch materials now include real mobile reconnect screenshots
- the file browser now shows compact file type badges instead of generic file icons
- release packaging now has a reusable flow for Linux `amd64` / `arm64` archives with matching `.sha256` files
- a generic installer script can now resolve the right asset from GitHub Releases by tag

Operational notes:

- release builds now inject the binary version via `ldflags`
- `make release-packages TAG=v0.3.1` now produces the release artifacts expected by the installer flow

Suggested release assets:

- Linux `amd64` tarball
- Linux `arm64` tarball
- matching `.sha256` files for both archives

Short release blurb:

`RoamBench v0.3.1 adds real mobile proof, clearer file browser badges, and a reusable binary release flow for Linux amd64/arm64.`
