# Manual 4-terminal test scenario: `codex-app-server` activity detection

Exercises the full local test loop for the `codex-app-server` `ActivitySource`:
a real `codex app-server` instance, its managed hooks writing a status file,
and `cli-watcher-tester` polling that status file to report activity ticks.

All commands below assume you're running from this directory
(`scripts/codex-app-server/`) inside your `cli-watcher-tester` checkout, and
that `cli-watcher-tester` is on your `PATH` (or adjust the path to wherever
you built it, e.g. `../../bin/cli-watcher-tester`).

Every terminal uses the same `CODEX_HOME` so they all agree on where things
live. `/tmp/codex-app-server` (the scripts' own default) is used here as an
example — swap in any path you like, as long as it's consistent across all
four terminals.

## Terminal 1 — install hooks + start the server

```sh
export CODEX_HOME=/tmp/codex-app-server
./install-codex-hooks.sh
./codex-app-server-ctl.sh start
```

`install-codex-hooks.sh` writes to `/etc/codex/` (needs `sudo`) and bakes
the resolved `$CODEX_HOME` in as a literal path in the installed hook
script — this only needs to be re-run when you want to point hooks at a
*different* `CODEX_HOME`, not every time you restart the server.

`codex-app-server-ctl.sh status` / `stop` work the same way once started.

## Terminal 2 — watch the status file

```sh
export CODEX_HOME=/tmp/codex-app-server
./codes-app-server-hooks-monitor.sh
```

Prints each status-file update as it happens. Useful to see raw hook
activity independent of what CLI Watcher itself decides to do with it.

## Terminal 3 — connect and drive a session

```sh
export CODEX_HOME=/tmp/codex-app-server
./codex-app-server-client.py
```

Then type one line at a time (Enter after each):

```
{"method": "initialize", "id": 0, "params": {"clientInfo": {"name": "codex_vscode", "title": "Codex VS Code Extension", "version": "0.1.0"}}}
{"method": "thread/start", "id": 1, "params": {}}
```

Copy the `id` from the `thread/start` response's `result.thread.id`, then
start a real turn — **this** is what actually fires the hooks, not
`thread/start` alone:

```
{"method": "turn/start", "id": 2, "params": {"threadId": "<paste-the-id-here>", "input": [{"type": "text", "text": "say hi"}]}}
```

Terminal 2 should print the status-file update within a second or two.

## Terminal 4 — watch CLI Watcher pick it up

```sh
export CODEX_HOME=/tmp/codex-app-server
cli-watcher-tester --enabled true --activitySources tty:disabled,codex-app-server --verbose true
```

`tty:disabled` isolates this test to the `codex-app-server` source only
(otherwise your own terminal's TTY activity would also tick). `--verbose
true` promotes CLI Watcher's activity-detection log lines to Info level so
you can watch it react to what Terminal 3 does, without needing the
tester's own `--logLevel debug`.
