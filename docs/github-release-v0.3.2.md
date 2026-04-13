# GitHub Release Copy

Suggested tag:

- `v0.3.2`

Suggested title:

- `RoamBench v0.3.2`

Suggested subtitle:

- `Viewer-first file opens, Office previews, and safer inline PDFs`

## English

`v0.3.2` tightens the built-in file workflow so more files stay inside Viewer instead of spilling into fallback browser or split-editor behavior.

Highlights:

- Viewer now supports DOCX and XLSX previews, alongside improved in-view PPTX slide navigation
- file selections now open in Viewer first, which fixes extensionless plain-text files unexpectedly forcing split editor layout
- single-pane desktop workspaces can widen the embedded file panel with a horizontal drag handle
- inline PDF preview now works more reliably in Chromium-family browsers that hand PDFs off to extension-backed viewers

Operational notes:

- after upgrading, rebuild the `roambench` binary and restart the service because front-end assets are embedded in the Go binary
- release builds still use `make release-packages TAG=v0.3.2`

Suggested release assets:

- Linux `amd64` tarball
- Linux `arm64` tarball
- matching `.sha256` files for both archives

Short release blurb:

`RoamBench v0.3.2 adds Office previews, fixes viewer-first file opening for extensionless text files, improves inline PDF compatibility, and makes the single-pane file panel resizable.`
