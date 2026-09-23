# bash completion for cli-watcher-tester / cwt (any path).
#
# Install (bash):
#   source /path/to/cli-watcher-tester/completions/cli-watcher-tester.bash
#
# Registered as the shell-wide default (-D). The function only handles
# commands whose basename is cli-watcher-tester or cwt; everything else
# is chained to whatever default was registered before this script
# (typically bash-completion's loader and/or python-argcomplete), so
# git and other lazy-loaded completions keep working.

# Remember the default completer we are about to replace (skip ourselves
# if this file is sourced more than once).
_cli_watcher_tester_spec="$(complete -p -D 2>/dev/null)" || true
_cli_watcher_tester_re=' -F[[:space:]]+([^[:space:]]+)'
if [[ -n "$_cli_watcher_tester_spec" && "$_cli_watcher_tester_spec" == *"-F _cli_watcher_tester_complete"* ]]; then
	# Already installed; keep the previously captured chain target.
	:
elif [[ -n "$_cli_watcher_tester_spec" && "$_cli_watcher_tester_spec" =~ $_cli_watcher_tester_re ]]; then
	_cli_watcher_tester_prev_default="${BASH_REMATCH[1]}"
else
	_cli_watcher_tester_prev_default=""
fi
unset _cli_watcher_tester_spec _cli_watcher_tester_re

_cli_watcher_tester_resolve_bin() {
	local cmd="$1"
	if command -v "$cmd" >/dev/null 2>&1; then
		command -v "$cmd"
		return 0
	fi
	local dir
	dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../bin" 2>/dev/null && pwd)" || return 1
	if [[ -x "$dir/cli-watcher-tester" ]]; then
		printf '%s\n' "$dir/cli-watcher-tester"
		return 0
	fi
	return 1
}

_cli_watcher_tester_complete() {
	local cmd="${COMP_WORDS[0]}"
	local base="${cmd##*/}"

	case "$base" in
	cli-watcher-tester | cwt)
		# Words after the command, including the partial word being completed.
		local -a args=("${COMP_WORDS[@]:1}")
		local IFS=$'\n'
		local out
		local bin
		bin="$(_cli_watcher_tester_resolve_bin "$cmd")" || {
			COMPREPLY=()
			return 0
		}
		out="$("$bin" __complete "${args[@]}" 2>/dev/null)" || true
		if [[ -n "$out" ]]; then
			COMPREPLY=($(printf '%s\n' "$out"))
		else
			# No candidates → filename fallback (matches -o default).
			COMPREPLY=()
		fi
		return 0
		;;
	esac

	# Not ours: chain to the previous -D default so lazy loaders
	# (bash-completion, python-argcomplete, …) still run.
	if [[ -n "${_cli_watcher_tester_prev_default-}" ]] &&
		declare -F "$_cli_watcher_tester_prev_default" >/dev/null 2>&1; then
		"$_cli_watcher_tester_prev_default" "$@"
		return $?
	fi
	# Fallbacks if there was no previous default to save.
	if declare -F _completion_loader >/dev/null 2>&1; then
		_completion_loader "${1:-$cmd}"
		return $?
	fi
	if declare -F _comp_complete_load >/dev/null 2>&1; then
		_comp_complete_load "${1:-$cmd}"
		return $?
	fi
	COMPREPLY=()
	return 0
}

complete -o default -F _cli_watcher_tester_complete -D
