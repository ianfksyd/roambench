# Lightweight Evidence

This page records concrete runtime evidence for RoamBench instead of relying on vague "lightweight" claims.

Public product name: `RoamBench`
Current technical identifiers: `roambench`

## Snapshot

Measured on `2026-04-04` UTC in a live session with:

- `6` terminal sessions
- `3` workspace views
- `tmux` backend enabled
- RoamBench running from the built local binary

Binary:

- `roambench`: about `9.5 MB`

Runtime memory snapshot:

| Component | RSS | Notes |
| --- | --- | --- |
| RoamBench main process | about `13.3 MiB` | Go web server and API layer only |
| `tmux` server process | about `24.8 MiB` | session backend |
| `6` interactive shells | about `25.7 MiB` total | about `4.3 MiB` average per shell |
| Active `tmux attach` clients | about `8.3 MiB` total when attached | this rises and falls with live web attachments |

Practical reading:

- RoamBench service process itself is small compared with the terminal workload it is hosting.
- Most memory comes from the terminal backend and the actual shell sessions, not from the web wrapper alone.
- In this `6` terminal / `3` view snapshot, the always-on server-side footprint stayed in the "tens of MiB" range rather than the hundreds.

## Measurement Method

Commands used:

```bash
ls -lh roambench
ps -o pid,ppid,stat,rss,vsz,%cpu,etime,cmd -p <pids>
tmux ls
```

Representative process snapshot:

```text
PID      RSS   CMD
3536562  13592 ./roambench --config ./roambench.toml
2241326  25396 tmux new-session ...
2266507   4220 /bin/bash --rcfile /tmp/.roambench-bashrc -i
2379694   3372 /bin/bash --rcfile /tmp/.roambench-bashrc -i
2480299   3804 /bin/bash --rcfile /tmp/.roambench-bashrc -i
3449060   5052 /bin/bash --rcfile /tmp/.roambench-bashrc -i
3679053   4952 /bin/bash --rcfile /tmp/.roambench-bashrc -i
3722798   4948 /bin/bash --rcfile /tmp/.roambench-bashrc -i
3752482   4276 tmux attach-session -t ...
3752492   4272 tmux attach-session -t ...
```

## What This Does And Does Not Prove

This is useful evidence for:

- low overhead of the RoamBench service process itself
- practical runtime size for a multi-terminal self-hosted session
- showing that RoamBench is much closer to a thin control layer over terminals than to a heavy browser IDE stack

This does not prove:

- browser tab memory usage
- heavy task memory inside the shells themselves
- comparative performance against raw SSH or a plain terminal

That comparison would be misleading anyway. The correct comparison target is a heavier browser IDE or remote workspace stack, not a plain terminal with no web UI.

## Recommended Launch Wording

Use wording like this:

- `RoamBench keeps its own server-side overhead small. In a live 6-terminal session, the RoamBench process itself stayed around 13 MiB RSS, while most memory came from tmux and the shell sessions being hosted.`
- `Compared with heavier browser IDE setups, RoamBench keeps terminal persistence, split views, file tools, and phone access with much lower setup and runtime overhead.`

Avoid wording like this:

- `lighter than a terminal`
- `near-zero overhead`
- `uses almost no memory`

## Next Measurements To Add

To strengthen this page later, add:

- refresh-to-reconnect time
- restart-to-recover time with `tmux`
- mobile reconnect time
- side-by-side comparison against one heavier browser IDE workflow
