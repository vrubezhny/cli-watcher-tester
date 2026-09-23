#!/bin/sh
# Installs the che-machine-exec codex-app-server managed hooks for LOCAL
# TESTING, targeting the instance codex-app-server-ctl.sh starts.
#
# The handler script normally resolves its status-file directory via
# ${CODEX_HOME:-$HOME/.codex} at hook-invocation time. For local testing
# where codex-app-server-ctl.sh starts the server with
# CODEX_HOME=/tmp/codex-app-server (not the real $HOME), that dynamic
# fallback is fragile to rely on for a quick test loop - this installer
# instead bakes the intended CODEX_HOME in as a literal path in the
# installed copy, so the status file location is unambiguous and doesn't
# depend on exactly what environment the hook subprocess sees at runtime.
#
# Requires sudo (writes to /etc/codex/).
#
# Usage: install-codex-hooks.sh [codex_home]
#   (defaults to $CODEX_HOME, or /tmp/codex-app-server if unset - matching
#   codex-app-server-ctl.sh's own default, so running this with no
#   arguments matches a plain `codex-app-server-ctl.sh start`)

set -e

CODEX_HOME_FOR_HOOKS="${1:-${CODEX_HOME:-/tmp/codex-app-server}}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHE_MACHINE_EXEC_DIR="${CHE_MACHINE_EXEC_DIR:-$SCRIPT_DIR/../../../che-machine-exec}"
HOOKS_SRC_DIR="$CHE_MACHINE_EXEC_DIR/timeout/codex-hooks"
REQUIREMENTS_SRC="$HOOKS_SRC_DIR/requirements.toml"
HANDLER_SRC="$HOOKS_SRC_DIR/managed-hooks/codex-app-server-activity-status.sh"

if [ ! -f "$REQUIREMENTS_SRC" ] || [ ! -f "$HANDLER_SRC" ]; then
    echo "error: hooks source not found under $HOOKS_SRC_DIR" >&2
    echo "  set CHE_MACHINE_EXEC_DIR to your che-machine-exec checkout if it isn't a sibling of cli-watcher-tester" >&2
    exit 1
fi

echo "Installing managed hooks, baking in CODEX_HOME=$CODEX_HOME_FOR_HOOKS ..."

sudo mkdir -p /etc/codex/managed-hooks
sudo cp "$REQUIREMENTS_SRC" /etc/codex/requirements.toml
sudo chmod 0644 /etc/codex/requirements.toml

# Replace the dynamic ${CODEX_HOME:-$HOME/.codex} fallback with the
# resolved literal path, so this installed copy always writes the status
# file under $CODEX_HOME_FOR_HOOKS regardless of what environment the hook
# subprocess actually receives.
sed "s#\${CODEX_HOME:-\$HOME/.codex}#$CODEX_HOME_FOR_HOOKS#" "$HANDLER_SRC" \
    | sudo tee /etc/codex/managed-hooks/codex-app-server-activity-status.sh > /dev/null
sudo chmod 0755 /etc/codex/managed-hooks/codex-app-server-activity-status.sh

echo "Installed:"
echo "  /etc/codex/requirements.toml"
echo "  /etc/codex/managed-hooks/codex-app-server-activity-status.sh"
echo "    -> status file will be written under: $CODEX_HOME_FOR_HOOKS/app-server-control/"
echo
echo "Verify with: grep STATUS_DIR /etc/codex/managed-hooks/codex-app-server-activity-status.sh"
