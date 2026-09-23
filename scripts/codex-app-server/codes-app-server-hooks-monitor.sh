#!/bin/sh
# Watches the codex-app-server activity status file for changes and prints
# each update as it happens (poll-based - checked once per second).
#
# Uses the same CODEX_HOME default as codex-app-server-ctl.sh /
# codex-app-server-client.py, so all three scripts point at the same
# instance out of the box: /tmp/codex-app-server, or $CODEX_HOME if set.

CODEX_HOME="${CODEX_HOME:-/tmp/codex-app-server}"
STATUS_DIR="$CODEX_HOME/app-server-control"
STATUS_FILE="$STATUS_DIR/codex-app-server-activity-status.yaml"

echo "Watching $STATUS_FILE for changes (Ctrl-C to stop) ..." >&2

last_mtime=""
waiting_logged=""
while :; do
    mtime=$(stat -c %Y "$STATUS_FILE" 2>/dev/null)
    if [ -z "$mtime" ]; then
        if [ -z "$waiting_logged" ]; then
            echo "Status file does not exist yet - waiting for the first hook event ..." >&2
            waiting_logged=1
        fi
        sleep 1
        continue
    fi
    waiting_logged=""
    if [ "$mtime" != "$last_mtime" ]; then
        last_mtime="$mtime"
        printf '%s ' "$(date -d "@$mtime" '+%m/%d/%Y %I:%M:%S %p')"
        cat "$STATUS_FILE"
        printf '\n'
    fi
    sleep 1
done
