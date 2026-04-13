# Release Packaging

This guide covers the minimum packaging flow for GitHub releases.

## What It Produces

Run this from the repo root:

```bash
make release-packages TAG=v0.3.1
```

The command builds Linux `amd64` and `arm64` release bundles under `dist/releases/`.

Each architecture gets:

- one `.tar.gz` archive
- one matching `.sha256` file

Asset naming follows this pattern:

- `roambench-release-<shortsha>-<yyyymmdd>-linux-amd64.tar.gz`
- `roambench-release-<shortsha>-<yyyymmdd>-linux-arm64.tar.gz`

## Bundle Contents

Each archive contains:

- `roambench`
- `README.md`
- `LICENSE`
- `roambench.example.toml`
- `roambench.quickstart.toml`
- `roambench.service`

## How Versioning Works

- the binary version banner is injected via build `ldflags`
- pass the intended release tag with `TAG=vX.Y.Z`
- the script strips the leading `v` before embedding the version string into the binary

That means:

- `TAG=v0.3.1` produces a binary that reports `RoamBench v0.3.1`
- local `make build` uses `git describe` output when available

## Publish Sequence

1. Update `CHANGELOG.md` and release notes for the chosen tag.
2. Create or verify the tag you want to ship.
3. Run `make release-packages TAG=vX.Y.Z`.
4. Upload the two archives and two `.sha256` files to the GitHub release.
5. Verify the generic installer script against the uploaded assets:

```bash
bash scripts/install-roambench.sh vX.Y.Z /usr/local/bin
```

## Notes

- the package script uses `CGO_ENABLED=0` for portable Linux release binaries
- release artifacts are written under `dist/`, which is already ignored by git
- if you need a different output path, call the script directly:

```bash
scripts/package-roambench-release.sh --tag v0.3.1 --output-dir /tmp/roambench-release-assets
```
