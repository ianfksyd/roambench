# RoamBench Deployment Hardening

Public product name: `RoamBench`  
Current technical identifiers: `roambench`

This guide explains what users should do after building or downloading the binary, especially before exposing RoamBench beyond a trusted local or LAN setup.

This is not a full security audit. It is the practical minimum hardening checklist for a single-user self-hosted deployment.

## 1. Choose The Unix User Deliberately

RoamBench is single-user only.

Important rule:

- `[auth].single_user` must exactly match the Unix account running the `roambench` process

That means the service account is also the shell account whose files and home directory the terminal will see.

Choose one of these models:

- Run as your normal Unix user if you want direct access to your existing `~/` projects, shell config, and tools.
- Run as a dedicated Unix user such as `roambench` if you want tighter isolation. In that case, keep the projects and tools you want RoamBench to access under that account.

Do not run RoamBench as `root`.

## 2. Start From The Example Config, Not Quickstart

Use:

```bash
cp configs/roambench.example.toml roambench.toml
```

Do not expose this file to the public Internet:

```bash
cp configs/roambench.quickstart.toml roambench.toml
```

Why:

- `quickstart` enables insecure HTTP
- `quickstart` disables IP filtering
- it is only meant for trusted local or LAN testing

## 3. Protect The Config File

Your config can contain:

- password hash
- allowed IPs
- deployment-specific paths
- TLS file paths

Recommended permissions:

```bash
chmod 700 /etc/roambench
chmod 600 /etc/roambench/roambench.toml
```

If you keep the password hash in an environment file instead, protect that file too:

```bash
chmod 600 /etc/roambench/roambench.env
```

Never commit a real deployment config to a public repo.

## 4. Minimum Safe Config For A Public Or Semi-Public Deployment

Use a config shape like this:

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

[auth]
method = "password"
password_hash = "$2a$10$..."
single_user = "your-unix-user"
```

Minimum rules:

- keep `allow_all_ips = false` unless the service is only reachable inside a trusted local network
- keep `allow_insecure_http = false` for public access
- keep `trust_proxy = false` unless a trusted reverse proxy is in front of RoamBench
- use `password` auth unless you specifically need PAM

## 5. Generate And Store The Password Safely

Generate a bcrypt hash:

```bash
./roambench --password-hash
```

Then use one of these two patterns:

- put the hash in `roambench.toml`
- or store it in a protected environment file and load it from systemd

Example environment file:

```bash
ROAMBENCH_USER=your-unix-user
ROAMBENCH_PASSWORD_HASH=$2a$10$...
```

If you use an environment file:

- keep it outside the repo
- set mode `600`
- do not put plaintext passwords in it

## 6. Use TLS Or A Trusted Reverse Proxy

Recommended public-access models:

### Model A: RoamBench terminates TLS directly

Set:

- `tls_cert`
- `tls_key`
- `allow_insecure_http = false`
- `trust_proxy = false`

### Model B: Reverse proxy terminates TLS

Put RoamBench behind a trusted reverse proxy such as Caddy or Nginx.

Then:

- keep RoamBench bound to an internal port
- enable TLS at the proxy
- set `trust_proxy = true` only if the proxy is trusted and correctly sets `X-Forwarded-Proto`

Do not enable `trust_proxy = true` when RoamBench is directly exposed to arbitrary clients.

## 7. Keep A Network Allowlist Or Firewall Layer

RoamBench already supports an application-layer IP allowlist.

Use it together with host or cloud firewall rules when possible.

Recommended mindset:

- public Internet: TLS plus firewall and/or `allowed_ips`
- trusted LAN: you may relax the IP layer, but only if you understand the tradeoff

## 8. Install `tmux`

RoamBench works without `tmux`, but persistence is weaker.

Install `tmux` if you want:

- sessions to survive refresh
- sessions to survive service restart
- safer long-running CLI workflows

Without `tmux`, restarting the service can interrupt active shells and tasks.

## 9. If You Download A Binary Release, Verify It

If the GitHub release provides checksums, verify them before installing:

```bash
sha256sum -c sha256sums.txt
```

Then install the binary to a stable path such as:

```bash
install -m 0755 roambench /usr/local/bin/roambench
```

## 10. Systemd Recommendations

Start from the template:

- [configs/roambench.service](../configs/roambench.service)

Before using it:

- set `User=...`
- set `Group=...`
- set `WorkingDirectory=...`
- set `ExecStart=...`
- point it at a real deployment config, not a LAN-only file

If you want to keep auth settings out of the main config, use an environment file:

```ini
EnvironmentFile=-/etc/roambench/roambench.env
```

Useful baseline service practices:

- run as a non-root user
- restart automatically on failure
- keep config and env files readable only by that service user
- cap the complete task pool with `MemoryHigh=`, `MemoryMax=`,
  `MemorySwapMax=`, and `TasksMax=`
- use `Delegate=cpu io memory pids` with `DelegateSubgroup=supervisor` when
  `[terminal.resources]` enables per-terminal limits

The template uses percentage-based pool memory limits. The host-specific unit
under `deploy/` uses a 6 GiB high watermark, 8 GiB hard limit, 1 GiB swap
limit, and 384-task ceiling for a machine with about 16 GiB RAM. Keep
`OOMPolicy=continue` so one terminal hitting its grouped cgroup OOM limit does
not terminate the RoamBench UI and unrelated terminals.

Be careful with aggressive systemd sandboxing options. RoamBench launches real terminal sessions, so filesystem and namespace restrictions can change what the terminal can access.

## 11. Post-Deploy Checks

After starting the service, verify:

1. the login page is reachable only through the URL and network path you intended
2. the service is not running as `root`
3. a new terminal starts in `~/`
4. the terminal backend shows `tmux` persistence if you expect restart-safe sessions
5. the config does not use `allow_all_ips = true` for public exposure
6. TLS or the trusted reverse proxy path is actually in use

## 12. Safe Default Mental Model

If you want the shortest safe summary:

- use the example config, not quickstart
- run as one non-root Unix user
- set `single_user` to that user
- generate a bcrypt hash
- keep the config private
- use `tmux`
- expose the service only behind TLS or a trusted reverse proxy
- keep IP filtering or firewall restrictions in place unless the deployment is truly local-only

## 13. Agent Takeover and Acceptance Trust Boundary

An agent running under the same Unix UID as RoamBench is inside the server's
trust boundary. It can potentially read the server's files and process data or
modify the JSON state and SQLite database directly. API approval gates and file
hashes do not isolate such a process. Use separate OS identities or containers
with restricted mounts for agents that must not have those privileges.

Acceptance commands receive only PATH, HOME, LANG, LC_ALL, LC_CTYPE, TMPDIR and TZ
from the service environment. Service credentials (including
ROAMBENCH_PASSWORD_HASH) and interpreter injection variables are not inherited.
HOME and PATH still refer to the operator's account and tools; this is not a
sandbox and does not prevent reads of files accessible to that account.

Human dispatch approval records a SHA-256 baseline of common acceptance inputs:
test/spec directories and filenames, shell scripts, executable files, common
build/dependency manifests, and local files named in the command arguments.
Legacy ready tasks without a baseline capture it before granting their first
lease. Before acceptance runs, added, deleted or changed inputs (including mode
and symlink changes) pause the task as acceptance_inputs_changed. The project
detail asks the operator to inspect the diff; after reviewing it, explicit
restart=true or a new dispatch approval establishes the reviewed baseline.
Inputs that cannot be read or exceed the scan limits also require human review.

The scan is deliberately conservative and bounded (10,000 selected files,
64 MiB); .git, node_modules, vendor, .venv and .roambench directories are excluded.
It cannot infer every dynamically loaded dependency or prevent concurrent
rewrites by a same-UID agent. Review the full test/build diff at milestone
acceptance and use an isolated, trusted validation environment when stronger
assurance is needed.

Project detail statistics separate human checkpoint decisions
(checkpointsByKind) from agent artifacts (artifactsByKind). They count distinct
sources in the most recent 4,096 retained activity records, not lifetime totals.
