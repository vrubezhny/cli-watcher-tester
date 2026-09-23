#!/bin/sh
# Start/stop a local codex app-server instance for testing.
# Usage: codex-app-server-ctl.sh {start|stop|status}

CODEX_HOME="${CODEX_HOME:-/tmp/codex-app-server}"
CONTROL_DIR="$CODEX_HOME/app-server-control"
SOCK="$CONTROL_DIR/app-server-control.sock"
LOG="$CONTROL_DIR/app-server.log"

start() {
    mkdir -p "$CONTROL_DIR"
    rm -f "$SOCK"
    CODEX_HOME="$CODEX_HOME" setsid nohup codex -c features.code_mode_host=true \
        app-server --listen "unix://$SOCK" >"$LOG" 2>&1 < /dev/null &
    disown
    sleep 1
    echo "Started codex app-server:"
    echo "  CODEX_HOME = $CODEX_HOME"
    echo "  socket     = $SOCK"
    echo "  log        = $LOG"
    pgrep -af -- "--listen unix://$SOCK"
}

stop() {
    if pgrep -f -- "--listen unix://$SOCK" >/dev/null 2>&1; then
        pkill -f -- "--listen unix://$SOCK"
        echo "Stopped codex app-server on $SOCK"
    else
        echo "No codex app-server running on $SOCK"
    fi
}

status() {
    pgrep -af -- "--listen unix://$SOCK" || echo "Not running ($SOCK)"
}

case "$1" in
    start)  start ;;
    stop)   stop ;;
    status) status ;;
    *) echo "Usage: $0 {start|stop|status}"; exit 1 ;;
esac
