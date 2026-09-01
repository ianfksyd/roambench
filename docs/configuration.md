# RoamBench Configuration

Public product name: `RoamBench`
Current technical identifiers: `roambench`

This document describes how RoamBench loads configuration, what each section does, and which settings matter most for deployment.

## Config File Discovery

If you do not pass `--config`, RoamBench checks these paths in order:

1. `./roambench.toml`
2. `~/.config/roambench/roambench.toml`
3. `/etc/roambench/roambench.toml`

You can always override this:

```bash
./roambench --config /path/to/roambench.toml
```

## Example

Start from:

```bash
cp configs/roambench.example.toml roambench.toml
```

Fast local/LAN demo path:

```bash
cp configs/roambench.quickstart.toml roambench.toml
```

Use the quickstart file only for trusted local or LAN testing. It enables insecure HTTP and disables IP filtering to minimize setup friction.

## Server

```toml
[server]
host = "0.0.0.0"
port = 3000
tls_cert = "/path/to/cert.pem"
tls_key = "/path/to/key.pem"
allow_insecure_http = false
allowed_ips = ["203.0.113.10"]
allow_all_ips = false
trust_proxy = false
```

Important behavior:

- `host` and `port` define the listening address
- `tls_cert` and `tls_key` must be set together
- for non-loopback hosts, RoamBench expects TLS unless `allow_insecure_http = true`
- if `allow_all_ips = false`, `allowed_ips` must contain at least one valid IP
- `trust_proxy = true` should only be enabled when a trusted reverse proxy sets `X-Forwarded-Proto`

## Authentication

```toml
[auth]
method = "password"
session_timeout = "24h"
max_login_attempts = 5
lockout_duration = "15m"
password_hash = ""
single_user = "your-unix-user"
```

Important behavior:

- RoamBench is single-user only
- `single_user` must exactly match the Unix account running the process
- `method` can be `password` or `pam`
- `password_hash` is only used in `password` mode
- if `password_hash` is empty, RoamBench also checks `ROAMBENCH_PASSWORD_HASH`
- if `single_user` is empty, RoamBench also checks `ROAMBENCH_USER`

More details:

- [Authentication Guide](authentication.md)

## Terminal

```toml
[terminal]
shell = "/bin/bash"
max_sessions = 0
scrollback = 10000
idle_timeout = "72h"
persist_max_bytes = 67108864
persist_dir = "/home/your-user/.local/state/roambench/terminals"

[terminal.resources]
min_system_available_bytes = 2147483648
max_system_swap_used_percent = 75
max_memory_pressure_avg10 = 10
session_memory_high_bytes = 3221225472
session_memory_max_bytes = 5368709120
session_swap_max_bytes = 536870912
session_pids_max = 256
session_cpu_weight = 80
session_io_weight = 80
```

Important behavior:

- `shell` is the command used for new terminal sessions
- `max_sessions = 0` means no fixed per-user session count cap
- `scrollback` controls tmux history size when tmux is enabled
- `idle_timeout` removes inactive sessions after the configured duration
- `persist_max_bytes` limits how much disk is used for terminal metadata
- `persist_dir` overrides the metadata storage directory
- the three admission settings reject new terminals when available memory,
  host swap use, or cgroup PSI indicates sustained pressure
- `session_memory_high_bytes` starts reclaim before a terminal reaches its
  hard `session_memory_max_bytes` limit
- `session_swap_max_bytes` and `session_pids_max` contain swap thrashing and
  runaway process trees within one terminal
- `session_cpu_weight` and `session_io_weight` set relative cgroup v2 weights
  from 1 to 10000; `0` leaves a controller unmanaged

All `[terminal.resources]` values are optional and `0` disables that check or
limit. Per-terminal limits require cgroup v2 plus `Delegate=` and
`DelegateSubgroup=supervisor` in the systemd unit. The example values target a
host with about 16 GiB RAM; tune them for smaller or larger machines.

Default metadata directory:

- `~/.local/state/roambench/terminals`

Persistence model:

- terminal runtime state is held by `tmux`
- RoamBench persists terminal metadata to disk
- browser workspaces are not stored here; workspace state is persisted separately on the server and cached in the browser

## UI

```toml
[ui]
title = "RoamBench"
motd = "Welcome"
```

Important behavior:

- `title` controls the main app title
- `motd` is available for lightweight UI messaging

## Environment Variables

RoamBench currently recognizes:

- `ROAMBENCH_PASSWORD_HASH`
- `ROAMBENCH_USER`

These are fallback sources when the config file omits the corresponding auth values.

## Command-Line Overrides

RoamBench supports:

- `--config`
- `--host`
- `--port`
- `--password-hash`

`--host` and `--port` override config file values at runtime.

## Recommended Self-Hosted Setup

For a typical personal deployment:

- keep `method = "password"`
- set `single_user` to the Unix account running the service
- generate a `bcrypt` password hash with `./roambench --password-hash`
- keep IP filtering enabled unless you are behind another trusted layer
- use TLS directly or terminate TLS at a trusted reverse proxy
- install `tmux` so terminal sessions survive refresh and restart

For the post-build and public-exposure checklist, see:

- [Deployment Hardening](deployment-hardening.md)

## Browser-Local And Server-Synced State

RoamBench keeps some state on the server and some state in each browser.

Server-synced for the current signed-in user:

- workspace tabs
- workspace names
- `1 / 2 / 4` workspace layouts
- terminal placement within workspaces

Browser-local:

- interface language
- terminal font and color settings
- editor view preferences

This is intentional: terminal session and workspace state can follow the same signed-in user across browsers, while personal UI preferences stay local to each browser.
