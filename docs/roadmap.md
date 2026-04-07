# RoamBench Roadmap

This roadmap tracks the next practical improvements for RoamBench after the
current `v0.1.x` baseline.

Public product name: `RoamBench`
Current technical identifiers: `roambench`

Guiding constraints:

- keep RoamBench single-user and self-hosted
- prefer small, high-value workflow improvements over large platform features
- deepen the terminal + file workspace model instead of expanding into multi-user administration

## Phase 1: Complete The File Workflow

Status: shipped in the current branch.

Goal: make the built-in file workspace usable without dropping back to the shell
for common file management tasks.

Delivered:

- expose file rename and move in the browser UI using the existing backend route
- add `New File`
- add `Save As`
- add file duplicate / copy
- support drag-and-drop upload
- add upload cancel support in the UI
- allow path-aware folder creation from the browser UI

## Phase 2: Make Editing Safer

Status: shipped in the current branch.

Goal: reduce accidental data loss and make the editor reliable for daily use.

Delivered:

- warn before leaving the page with dirty editor tabs
- preserve and restore unsaved drafts after session expiry or reconnect
- show clearer dirty-state and save-state feedback
- add editor find / replace
- add go-to-line
- add optional line numbers

## Phase 3: Improve Large Directory Navigation

Status: in progress.

Goal: keep the file browser usable when directories stop being small.

Shipped:

- add file search / filter in the current directory
- add breadcrumb navigation

Remaining:

- add recent directories or pinned directories
- add batch selection for delete and download
- replace direct destructive delete with a safer flow where practical

## Phase 4: Add Workflow Differentiators

Status: in progress.

Goal: improve the terminal workspace itself without changing the product model.

Shipped:

- server-side workspace sync for users who move between browsers

Remaining:

- terminal startup templates with working directory and optional startup command
- workspace activity indicators for background terminal output
- quick project shortcuts for common directories

## Continuous Work

These items should move alongside feature work instead of waiting for later.

- add focused tests for `internal/filebrowser`
- add coverage for rename, delete, write, and path-resolution edge cases
- add lightweight front-end regression coverage for editor and file actions
- keep the roadmap aligned with shipped changes in `CHANGELOG.md`

## Explicit Non-Goals For The Near Term

These are intentionally lower priority than the workflow items above.

- multi-user support
- role-based permissions
- SSO / OIDC integration
- turning RoamBench into a general-purpose web IDE
