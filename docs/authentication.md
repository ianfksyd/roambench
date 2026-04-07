# RoamBench Authentication

Public product name: `RoamBench`
Current technical identifiers: `roambench`

RoamBench supports two authentication modes:

- `password`: compare the submitted password against a configured `bcrypt` hash
- `pam`: authenticate through the host PAM stack when built with `-tags pam`

Most deployments use `password` mode.

## Password Mode

Password mode does not store a plaintext password. RoamBench stores only a `bcrypt` hash.

Configuration lives under `[auth]` in your config file:

```toml
[auth]
method = "password"
password_hash = ""
single_user = "your-unix-user"
```

Rules:

- `single_user` must exactly match the Unix account running the `roambench` process
- `password_hash` must be a valid `bcrypt` hash
- if `password_hash` is empty, RoamBench will also accept the `LITETERM_PASSWORD_HASH` environment variable

## Set Or Change The Password

1. Generate a new `bcrypt` hash:

   ```bash
   ./roambench --password-hash
   ```

2. Enter the new password at the prompt.

3. Copy the generated hash and use one of these methods:

   Method A: put it in the config file

   ```toml
   [auth]
   method = "password"
   password_hash = "$2a$10$..."
   single_user = "your-unix-user"
   ```

   Method B: export it as an environment variable before starting the service

   ```bash
   export LITETERM_PASSWORD_HASH='$2a$10$...'
   ```

4. Restart the service.

After restart, the old password stops working immediately and the new password takes effect.

## Current Validation Behavior

During login, RoamBench checks:

1. the username must equal `single_user`
2. the password must match the configured `bcrypt` hash

If either check fails, login is rejected.

## Related Files

- `hash` generation: `cmd/roambench/main.go`
- password verification: `internal/auth/auth.go`
- example config: `configs/roambench.example.toml`
