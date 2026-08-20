#!/bin/sh
set -eu

apply=false
active_path=${CLAI_ACTIVE_PATH:-}

usage() {
	printf '%s\n' "Usage: $0 [--apply] [--clai-path PATH]"
	printf '%s\n' ""
	printf '%s\n' "Dry-runs by default. --apply removes only recognized Bash-era CLAI files."
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--apply)
		apply=true
		shift
		;;
	--clai-path)
		[ "$#" -ge 2 ] || {
			usage >&2
			exit 2
		}
		active_path=$2
		shift 2
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		printf 'Unknown argument: %s\n' "$1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if [ -z "$active_path" ]; then
	active_path=$(command -v clai 2>/dev/null || true)
fi
if [ -z "$active_path" ] || [ ! -f "$active_path" ] || [ ! -x "$active_path" ]; then
	printf '%s\n' "Refusing cleanup: install the Go clai binary first or pass --clai-path PATH." >&2
	exit 1
fi

binary_magic=$(LC_ALL=C od -An -tx1 -N4 "$active_path" 2>/dev/null | tr -d ' \n')
case "$binary_magic" in
7f454c46 | cffaedfe | feedfacf | cafebabe | bebafeca) ;;
*)
	printf 'Refusing cleanup: %s is not a recognized Linux or macOS native binary.\n' "$active_path" >&2
	exit 1
	;;
esac

version_output=$("$active_path" --version 2>/dev/null || true)
case "$version_output" in
"clai version "?*) ;;
*)
	printf 'Refusing cleanup: %s does not identify itself as CLAI with --version.\n' "$active_path" >&2
	exit 1
	;;
esac

legacy_install_dir=${CLAI_LEGACY_INSTALL_DIR:-/usr/local/lib/clai}
legacy_script=$legacy_install_dir/clai.sh
legacy_tools_dir=${CLAI_LEGACY_TOOLS_DIR:-${HOME:?HOME is required}/.clai_tools}

same_file() {
	[ -e "$1" ] && [ -e "$2" ] || return 1
	left_identity=$(stat -Lc '%d:%i' "$1" 2>/dev/null || stat -Lf '%d:%i' "$1" 2>/dev/null || true)
	right_identity=$(stat -Lc '%d:%i' "$2" 2>/dev/null || stat -Lf '%d:%i' "$2" 2>/dev/null || true)
	[ -n "$left_identity" ] && [ "$left_identity" = "$right_identity" ]
}

if same_file "$active_path" "$legacy_script"; then
	printf 'Refusing cleanup: active clai still resolves to the legacy payload %s.\n' "$legacy_script" >&2
	exit 1
fi

# Hashes of the final stock Bash files on pre-migration commit 970dbc2. A
# changed file is treated as user-owned and is never removed automatically.
legacy_script_sha=b1ca2b12f2e83291e32b14a618c863387040299e97ca18c6c32ed02fd2147778
legacy_cat_sha=ef4853963ed4d0fceb8886e92cd2d1279d8f220197b23a0b6760d77cf283cb85
legacy_find_sha=b877ecad4b3307814fd9b4b1b1068a84c38f717b3362dedb3c304488ed9ea88a
legacy_ls_sha=fad7f445b79f9fc31456fb9bde288f4a25e0b5e3a074131fbcc9b2dce8377bf2

hash_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
		return
	fi
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
		return
	fi
	printf '%s\n' ""
}

removed=0
matched=0
failures=0

delete_file() {
	delete_path=$1
	delete_label=$2
	matched=$((matched + 1))
	if [ "$apply" != true ]; then
		printf 'Would remove %s: %s\n' "$delete_label" "$delete_path"
		return
	fi
	if rm -f -- "$delete_path" 2>/dev/null; then
		printf 'Removed %s: %s\n' "$delete_label" "$delete_path"
		removed=$((removed + 1))
		return
	fi
	if command -v sudo >/dev/null 2>&1 && sudo rm -f -- "$delete_path"; then
		printf 'Removed %s: %s\n' "$delete_label" "$delete_path"
		removed=$((removed + 1))
		return
	fi
	printf 'Could not remove %s: %s\n' "$delete_label" "$delete_path" >&2
	failures=$((failures + 1))
}

directory_is_empty() {
	[ -d "$1" ] || return 1
	for directory_entry in "$1"/* "$1"/.[!.]* "$1"/..?*; do
		if [ -e "$directory_entry" ] || [ -L "$directory_entry" ]; then
			return 1
		fi
	done
	return 0
}

remove_empty_dir() {
	empty_path=$1
	directory_is_empty "$empty_path" || return 0
	if rmdir -- "$empty_path" 2>/dev/null; then
		printf 'Removed empty legacy directory: %s\n' "$empty_path"
		return
	fi
	if command -v sudo >/dev/null 2>&1 && sudo rmdir -- "$empty_path"; then
		printf 'Removed empty legacy directory: %s\n' "$empty_path"
		return
	fi
	printf 'Could not remove empty legacy directory: %s\n' "$empty_path" >&2
	failures=$((failures + 1))
}

consider_file() {
	consider_path=$1
	consider_hash=$2
	consider_label=$3
	if [ ! -e "$consider_path" ] && [ ! -L "$consider_path" ]; then
		return
	fi
	actual_hash=$(hash_file "$consider_path")
	if [ -z "$actual_hash" ]; then
		printf 'Preserving %s because no SHA-256 utility is available: %s\n' "$consider_label" "$consider_path"
		return
	fi
	if [ "$actual_hash" != "$consider_hash" ]; then
		printf 'Preserving modified or unrecognized %s: %s\n' "$consider_label" "$consider_path"
		return
	fi
	delete_file "$consider_path" "$consider_label"
}

printf 'Active Go CLAI: %s\n' "$active_path"
if [ "$apply" = true ]; then
	printf '%s\n' "Cleanup mode: apply"
else
	printf '%s\n' "Cleanup mode: dry run"
fi

consider_file "$legacy_script" "$legacy_script_sha" "Bash payload"
consider_file "$legacy_tools_dir/cat.sh" "$legacy_cat_sha" "stock cat tool"
consider_file "$legacy_tools_dir/find-wild.sh" "$legacy_find_sha" "stock find-wild tool"
consider_file "$legacy_tools_dir/ls.sh" "$legacy_ls_sha" "stock ls tool"

if [ "$apply" = true ]; then
	remove_empty_dir "$legacy_install_dir"
	remove_empty_dir "$legacy_tools_dir"
fi

config_path=${CLAI_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/clai.cfg}
history_path=${CLAI_HISTORY:-${XDG_STATE_HOME:-$HOME/.local/state}/clai/history_com.json}
mcp_path=${CLAI_TOOLS_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/clai/tools.d}
printf '%s\n' "Preserved by design:"
printf '  active command: %s\n' "$active_path"
printf '  configuration: %s\n' "$config_path"
printf '  history: %s\n' "$history_path"
printf '  Go MCP tools: %s\n' "$mcp_path"
printf '  modified/custom legacy tools (if present): %s\n' "$legacy_tools_dir"

if [ "$apply" != true ]; then
	if [ "$matched" -eq 0 ]; then
		printf '%s\n' "No recognized legacy files were found."
	else
		printf '%s\n' "Dry run only; re-run with --apply to remove the listed files."
	fi
fi
if [ "$failures" -ne 0 ]; then
	exit 1
fi
if [ "$apply" = true ]; then
	printf 'Removed %d recognized legacy file(s).\n' "$removed"
fi
