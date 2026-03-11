#!/bin/bash
# -*- Mode: sh; coding: utf-8; indent-tabs-mode: t; tab-width: 4 -*-

# CLAI
# https://github.com/merefield/clai

# Make sure required tools are installed
if [ ! -x "$(command -v jq)" ]; then
	echo "ERROR: CLAI requires jq to be installed."
	exit 1
fi
if [ ! -x "$(command -v curl)" ]; then
	echo "ERROR: CLAI requires curl to be installed."
	exit 1
fi

# Determine the user's environment
UNIX_NAME=$(uname -srp)
# Attempt to fetch distro info from lsb_release or /etc/os-release
if [ -x "$(command -v lsb_release)" ]; then
	DISTRO_INFO=$(lsb_release -ds | sed 's/^"//;s/"$//')
elif [ -f "/etc/os-release" ]; then
	# Avoid GNU grep -P which is unavailable on macOS; use sed to extract value.
	DISTRO_INFO=$(grep '^PRETTY_NAME=' /etc/os-release | head -n1 | sed 's/^PRETTY_NAME="\?//;s/"$//')
fi
# If we failed to fetch distro info, we'll mark it as unknown
if [ ${#DISTRO_INFO} -le 1 ]; then
	DISTRO_INFO="Unknown"
fi

# Version of CLAI
VERSION="1.0.5"

# Global variables
PRE_TEXT="  "  # Prefix for text output
NO_REPLY_TEXT="¯\_(ツ)_/¯"  # Text for no reply
INTERACTIVE_INFO="Hi! Feel free to ask me anything or give me a task. Type \"exit\" when you're done."  # Text for interactive mode intro
PROGRESS_TEXT="Thinking..."
PROGRESS_ANIM="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
HISTORY_MESSAGES="[]"  # Message history stored as a JSON array
HISTORY_LOADED=false
HISTORY_DIRTY=false

# Theme colors
CMD_BG_COLOR="\e[48;5;236m"  # Background color for cmd suggestions
CMD_TEXT_COLOR="\e[38;5;203m"  # Text color for cmd suggestions
INFO_TEXT_COLOR="\e[90;3m"  # Text color for all information messages
ERROR_TEXT_COLOR="\e[91m"  # Text color for cmd errors messages
CANCEL_TEXT_COLOR="\e[93m"  # Text color cmd for cancellation message
OK_TEXT_COLOR="\e[92m"  # Text color for cmd success message
	TITLE_TEXT_COLOR="\e[1m"  # Text color for the CLAI title

# Terminal control constants
CLEAR_LINE="\033[2K\r"
HIDE_CURSOR="\e[?25l"
SHOW_CURSOR="\e[?25h"
RESET_COLOR="\e[0m"

# Filesystem paths
STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
STATE_DIR="${STATE_HOME}/clai"
SESSION_TMPDIR="${TMPDIR:-/tmp}"
TEMP_FILES=()
CURSOR_HIDDEN=false
HISTORY_FILES=()

# Default query constants, these are used as default values for different types of queries
DEFAULT_EXEC_QUERY="Return only a single compact JSON object containing 'cmd' and 'info' fields. 'cmd' must always contain one or multiple commands to perform the task specified in the user query. 'info' must always contain a single-line string detailing the actions 'cmd' will perform and the purpose of all command flags. 'cmd' may output a shell script to perform complex tasks. 'cmd' may be omittied as a last resort if no command can be suggested."
DEFAULT_QUESTION_QUERY="Return only a single compact JSON object containing a 'info' field. 'info' must always contain a single-line string terminal-related answer to the user query."
DEFAULT_ERROR_QUERY="Return only a single compact JSON object containing 'cmd' and 'info' fields. 'cmd' is optional. 'cmd' must always contain a suggestion on how to fix, solve or repair the error in the user query. 'info' must always be a single-line string explaining what the error in the user query means, why it happened, and why 'cmd' might fix it. Use your tools to find out why the error occured and offer alternatives."
DYNAMIC_SYSTEM_QUERY="" # After most user queries, we'll add some dynamic system information to the query

# Global query variable, this will be updated with specific user and system information
GLOBAL_QUERY="You are CLAI (clai) v${VERSION}. You are an advanced Bash shell script. You are located at \"$0\". You do not have feelings or emotions, do not convey them. Please give precise curt answers. Please do not include any sign off phrases or platitudes, only respond precisely to the user. CLAI is made by Hezkore. You execute the tasks the user asks from you by utilizing the terminal and shell commands. No task is too big. Always assume the query is terminal and shell related. You support user plugins called \"tools\" that extends your capabilities, more info and plugins can be found on the CLAI homepage. The CLAI homepage is \"https://github.com/merefield/clai\". You always respond with a single JSON object containing 'cmd' and 'info' fields. We are always in the terminal. The user is using \"$UNIX_NAME\" and specifically distribution \"$DISTRO_INFO\". The users username is \"$USER\" with home \"$HOME\". You must always use LANG $LANG and LC_TIME $LC_TIME."

# Configuration file path
CONFIG_FILE=~/.config/clai.cfg
CONFIG_DIR="${CONFIG_FILE%/*}"

create_private_dir() {
	local dir_path="$1"
	mkdir -p "$dir_path"
	chmod 700 "$dir_path" 2>/dev/null || true
}

ensure_dir_exists() {
	local dir_path="$1"
	mkdir -p "$dir_path"
}

write_private_file() {
	local target="$1"
	local old_umask
	local write_status

	old_umask=$(umask)
	umask 077
	cat > "$target"
	write_status=$?
	umask "$old_umask"
	if [ "$write_status" -ne 0 ]; then
		return "$write_status"
	fi
	chmod 600 "$target" 2>/dev/null || true
	return 0
}

create_secure_temp() {
	local template="${1:-${SESSION_TMPDIR}/clai.XXXXXX}"
	local tmpfile

	tmpfile=$(mktemp "$template") || return 1
	chmod 600 "$tmpfile" 2>/dev/null || true
	printf "%s\n" "$tmpfile"
}

warn() {
	printf "WARNING: %s\n" "$1" >&2
}

write_private_file_atomic() {
	local target="$1"
	local target_dir
	local tmp_target
	local old_umask

	target_dir="${target%/*}"
	ensure_dir_exists "$target_dir"

	old_umask=$(umask)
	umask 077
	tmp_target=$(mktemp "${target}.tmp.XXXXXX") || {
		umask "$old_umask"
		return 1
	}
	if ! cat > "$tmp_target"; then
		umask "$old_umask"
		rm -f "$tmp_target"
		return 1
	fi
	umask "$old_umask"
	chmod 600 "$tmp_target" 2>/dev/null || true

	if ! mv -f "$tmp_target" "$target"; then
		rm -f "$tmp_target"
		return 1
	fi

	return 0
}

load_history() {
	local loaded_history
	local normalized_history

	if [ -f "$HISTORY_FILE" ]; then
		if loaded_history=$(cat "$HISTORY_FILE" 2>/dev/null) && normalized_history=$(jq -ce 'if type == "array" then map(select(.role != "system")) else error("history must be an array") end' <<< "$loaded_history" 2>/dev/null); then
			if [ "$loaded_history" != "$normalized_history" ]; then
				HISTORY_DIRTY=true
			fi
			HISTORY_MESSAGES="$normalized_history"
		else
			warn "Could not parse history file at $HISTORY_FILE; starting with empty history."
			HISTORY_MESSAGES='[]'
			HISTORY_DIRTY=true
		fi
	else
		HISTORY_MESSAGES='[]'
	fi
	HISTORY_LOADED=true
}

clear_history_runtime() {
	local history_path
	local failed=false

	for history_path in "${HISTORY_FILES[@]}"; do
		if [ -n "$history_path" ] && ! rm -f -- "$history_path"; then
			failed=true
		fi
	done

	HISTORY_MESSAGES='[]'
	HISTORY_DIRTY=false
	HISTORY_LOADED=true

	if [ "$failed" = true ]; then
		return 1
	fi

	return 0
}

save_history() {
	local max_history_count_int
	local persisted_history

	if [ "$HISTORY_LOADED" != true ]; then
		return 0
	fi

	if [ "$HISTORY_DIRTY" != true ]; then
		return 0
	fi

	max_history_count_int=$((MAX_HISTORY_COUNT))
	if [ "$max_history_count_int" -lt 1 ]; then
		max_history_count_int=1
	fi

	if ! persisted_history=$(jq -cn \
		--argjson history "$HISTORY_MESSAGES" \
		--argjson max_history "$max_history_count_int" '
		def persisted_entries:
			map(select(.role != "system"));
		def trim_turns($limit):
			. as $messages
			| ([range(0; ($messages | length)) | select($messages[.].role == "user")]) as $user_indexes
			| if ($messages | length) == 0 then []
			  elif ($user_indexes | length) == 0 then
				if ($messages | length) > $limit then $messages[-$limit:] else $messages end
			  elif ($user_indexes | length) > $limit then
				$messages[$user_indexes[-$limit]:]
			  else
				$messages
			  end;
		$history
		| persisted_entries
		| trim_turns($max_history)' 2>/dev/null); then
		warn "Failed to prepare history for persistence."
		return 1
	fi

	if ! write_private_file_atomic "$HISTORY_FILE" <<< "$persisted_history"; then
		warn "Failed to write history file at $HISTORY_FILE."
		return 1
	fi

	HISTORY_MESSAGES="$persisted_history"
	HISTORY_DIRTY=false
	return 0
}

exit_clai() {
	local status="${1:-0}"

	save_history
	exit "$status"
}

handle_clear_history() {
	if clear_history_runtime; then
		printf "%b%s%b\n\n" "${PRE_TEXT}${INFO_TEXT_COLOR}" "Cleared CLAI history." "${RESET_COLOR}"
		return 0
	fi

	printf "%b%s%b\n\n" "${ERROR_TEXT_COLOR}" "Failed to clear CLAI history." "${RESET_COLOR}"
	return 1
}

show_history_runtime() {
	local history_json
	local verbose="${1:-false}"

	if [ ! -f "$HISTORY_FILE" ]; then
		echo "No CLAI history."
		return 0
	fi

	if ! history_json=$(jq -ce 'if type == "array" then map(select(.role != "system")) else error("history must be an array") end' "$HISTORY_FILE" 2>/dev/null); then
		echo "Failed to read CLAI history." >&2
		return 1
	fi

	if [ "$(jq 'length' <<< "$history_json")" -eq 0 ]; then
		echo "No CLAI history."
		return 0
	fi

	jq -r '
		def indent2:
			split("\n") | map("  " + .) | join("\n");
		def indent4:
			split("\n") | map("    " + .) | join("\n");
		def preview_block($name; $text):
			if ($text | length) == 0 then
				"  \($name): empty"
			else
				($text | split("\n")) as $lines
				| if ($lines | length) <= 3 then
					"  \($name):\n" + (($lines | join("\n")) | indent4)
				  else
					"  \($name):\n"
					+ (($lines[0:3] | join("\n")) | indent4)
					+ "\n    [truncated after first 3 lines]"
				  end
			end;
		def content_text:
			if . == null then ""
			elif type == "string" then .
			else tostring
			end;
		to_entries[]
		| (.key + 1) as $i
		| .value as $m
		| if $m.role == "user" then
			"[\($i)] user\n" + (($m.content | content_text) | indent2)
		  elif $m.role == "tool" then
			"[\($i)] tool \($m.tool_call_id // "")\n" + (($m.content | content_text) | indent2)
		  elif (($m.tool_calls? // []) | length) > 0 then
			"[\($i)] assistant tool call\n"
			+ (
				$m.tool_calls
				| map(
					.function.name as $name
					| (.function.arguments | fromjson? // .function.arguments) as $args
					| "  name: \($name)\n  arguments:\n" + (($args | tostring) | indent4)
				)
				| join("\n")
			)
		  elif $m.role == "assistant" then
			($m.content | fromjson? // $m.content) as $content
			| if ($content | type) == "object" and ($content.command_result? != null) then
				($content.command_result) as $cr
				| "[\($i)] command result\n"
				+ "  command: \($cr.command // "")\n"
				+ "  exit_code: \($cr.exit_code // "")\n"
				+ "  edited: \($cr.edited // false)"
				+ if $verbose then
					(if (($cr.stdout // "") | length) > 0 then "\n  stdout:\n" + (($cr.stdout | content_text) | indent4) else "" end)
					+ (if (($cr.stderr // "") | length) > 0 then "\n  stderr:\n" + (($cr.stderr | content_text) | indent4) else "" end)
				  else
					"\n" + preview_block("stdout"; ($cr.stdout // ""))
					+ "\n" + preview_block("stderr"; ($cr.stderr // ""))
				  end
			  elif ($content | type) == "object" and (($content.info? != null) or ($content.cmd? != null)) then
				"[\($i)] assistant\n"
				+ (if $content.info? != null then "  info: \($content.info)\n" else "" end)
				+ (if $content.cmd? != null then "  cmd: \($content.cmd)" else "" end)
				| sub("\n$"; "")
			  else
				"[\($i)] assistant\n" + (($m.content | content_text) | indent2)
			  end
		  else
			"[\($i)] \($m.role // "unknown")\n" + (($m.content | content_text) | indent2)
		  end
		+ "\n"' --argjson verbose "$verbose" <<< "$history_json"
}

handle_show_history() {
	if show_history_runtime "$SHOW_HISTORY_VERBOSE"; then
		return 0
	fi

	return 1
}

is_clear_history_request() {
	local normalized_query

	normalized_query=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/[[:punct:]]*$//')

	case "$normalized_query" in
		"clear history"|"clear your history"|"clear our history"|"forget history"|"forget your history"|"forget our history"|"reset history"|"flush history")
			return 0
			;;
	esac

	return 1
}

conceal_cursor() {
	if [ -t 1 ] && [ -n "$HIDE_CURSOR" ] && [ "$CURSOR_HIDDEN" != true ]; then
		printf "%b" "$HIDE_CURSOR"
		CURSOR_HIDDEN=true
	fi
}

restore_cursor() {
	if [ -t 1 ] && [ -n "$SHOW_CURSOR" ] && [ "$CURSOR_HIDDEN" = true ]; then
		printf "%b" "$SHOW_CURSOR"
		CURSOR_HIDDEN=false
	fi
}

# cleanup is invoked indirectly via trap EXIT.
# shellcheck disable=SC2317
cleanup() {
	local path

	save_history

	for path in "${TEMP_FILES[@]}"; do
		[ -n "$path" ] && [ -e "$path" ] && rm -f "$path"
	done

	restore_cursor
}

trap cleanup EXIT

create_private_dir "$STATE_DIR"
ensure_dir_exists "$CONFIG_DIR"

HISTORY_FILE="${STATE_DIR}/history_com.json"
HISTORY_FILES=("$HISTORY_FILE")

# Built-in request handling
USER_QUERY=$*
USER_QUERY_ARGC=$#
FIRST_USER_ARG="$1"
SETUP_REQUESTED=false
SHOW_HISTORY_REQUESTED=false
SHOW_HISTORY_VERBOSE=false
SHOW_RESULTS_SHARING_REQUESTED=false
TOGGLE_RESULTS_SHARING_REQUESTED=false
if [ "$FIRST_USER_ARG" = "--clear-history" ] && [ "$USER_QUERY_ARGC" -eq 1 ]; then
	handle_clear_history
	clear_history_status=$?
	restore_cursor
	if [ "$clear_history_status" -eq 0 ]; then
		exit_clai 0
	fi
	exit_clai 1
fi
if [ "$FIRST_USER_ARG" = "--setup" ] && [ "$USER_QUERY_ARGC" -eq 1 ]; then
	SETUP_REQUESTED=true
fi
if [ "$USER_QUERY" = "setup" ] && [ "$USER_QUERY_ARGC" -eq 1 ]; then
	SETUP_REQUESTED=true
fi
if [ "$FIRST_USER_ARG" = "--show-history" ]; then
	if [ "$USER_QUERY_ARGC" -eq 1 ]; then
		SHOW_HISTORY_REQUESTED=true
	elif [ "$USER_QUERY_ARGC" -eq 2 ] && [ "$2" = "--verbose" ]; then
		SHOW_HISTORY_REQUESTED=true
		SHOW_HISTORY_VERBOSE=true
	else
		echo "ERROR: --show-history only supports the optional --verbose flag." >&2
		exit 1
	fi
fi
if [ "$FIRST_USER_ARG" = "--show-results-sharing" ] && [ "$USER_QUERY_ARGC" -eq 1 ]; then
	SHOW_RESULTS_SHARING_REQUESTED=true
fi
if [ "$FIRST_USER_ARG" = "--toggle-results-sharing" ] && [ "$USER_QUERY_ARGC" -eq 1 ]; then
	TOGGLE_RESULTS_SHARING_REQUESTED=true
fi
if [ -n "$USER_QUERY" ] && is_clear_history_request "$USER_QUERY"; then
	handle_clear_history
	clear_history_status=$?
	restore_cursor
	if [ "$clear_history_status" -eq 0 ]; then
		exit_clai 0
	fi
	exit_clai 1
fi

# Update info about history file
#GLOBAL_QUERY+=" Your message history file path is \"$HISTORY_FILE\"."

# Tools
OPENAI_TOOLS=""
TOOLS_PATH=~/.clai_tools

GLOBAL_QUERY+=" CLAI's canonical config file path is \"~/.config/clai.cfg\" (expanded path \"$CONFIG_FILE\")."
GLOBAL_QUERY+=" CLAI's tools directory is \"~/.clai_tools\" (expanded path \"$TOOLS_PATH\")."
GLOBAL_QUERY+=" CLAI persists state under \"\${XDG_STATE_HOME:-~/.local/state}/clai\" (expanded path \"$STATE_DIR\")."

# Create the directory only if it doesn't exist
create_private_dir "$TOOLS_PATH"
TOOL_LOG_FILE=$(create_secure_temp "${SESSION_TMPDIR}/clai-tool-output.XXXXXX.log") || exit_clai 1
TEMP_FILES+=("$TOOL_LOG_FILE")
PAYLOAD_FILE=$(create_secure_temp "${SESSION_TMPDIR}/clai-payload.XXXXXX.json") || exit_clai 1
TEMP_FILES+=("$PAYLOAD_FILE")
RESPONSE_FILE=$(create_secure_temp "${SESSION_TMPDIR}/clai-response.XXXXXX.json") || exit_clai 1
TEMP_FILES+=("$RESPONSE_FILE")
CURL_ERROR_FILE=$(create_secure_temp "${SESSION_TMPDIR}/clai-curl-error.XXXXXX.log") || exit_clai 1
TEMP_FILES+=("$CURL_ERROR_FILE")
: > "$TOOL_LOG_FILE"

# -------------------------------------------------------------
# Cross-shell compatibility helpers (Bash <4, zsh support)
# -------------------------------------------------------------

# Bash <4 (macOS default) does not support associative arrays (declare -A).
# zsh does support them (typeset -A) but this script is executed by Bash via
# the she-bang.  We therefore provide a thin abstraction layer that offers a
# handful of helper functions so the rest of the script can **pretend** an
# associative array exists no matter which shell/version is executing.  If the
# current shell supports real associative arrays we will use one, otherwise we
# fall back to two parallel indexed arrays.

# Determine associative-array support
HAS_ASSOC_ARRAY=false

# Bash ≥4 or any zsh support associative arrays. We only care whether declare
# succeeds; the probe variable is intentionally throwaway.
# shellcheck disable=SC2034
if [ "$CLAI_FORCE_NO_ASSOC_ARRAY" != true ] && (unset TEST 2>/dev/null; declare -A TEST 2>/dev/null); then
    HAS_ASSOC_ARRAY=true
fi

if [ "$HAS_ASSOC_ARRAY" = true ]; then
    # Native implementation
    declare -A TOOL_MAP
else
    # Fallback implementation using two parallel indexed arrays
    TOOL_MAP_KEYS=()
    TOOL_MAP_VALS=()
fi

# --- helper wrappers -------------------------------------------------------
# tool_map_exists  <key>
# tool_map_get     <key>   -> echoes value (empty string if missing)
# tool_map_set     <key> <value>  (fails silently if duplicate)
# tool_map_keys    -> echoes all keys separated by spaces
# tool_map_size    -> echoes number of stored pairs

tool_map_exists() {
    local k="$1"
    if [ "$HAS_ASSOC_ARRAY" = true ]; then
        [[ -n "${TOOL_MAP[$k]+x}" ]]
    else
        local i
        for i in "${!TOOL_MAP_KEYS[@]}"; do
            [ "${TOOL_MAP_KEYS[$i]}" = "$k" ] && return 0
        done
        return 1
    fi
}

tool_map_get() {
    local k="$1"
    if [ "$HAS_ASSOC_ARRAY" = true ]; then
        echo "${TOOL_MAP[$k]}"
    else
        local i
        for i in "${!TOOL_MAP_KEYS[@]}"; do
            if [ "${TOOL_MAP_KEYS[$i]}" = "$k" ]; then
                echo "${TOOL_MAP_VALS[$i]}"
                return 0
            fi
        done
    fi
}

tool_map_set() {
    local k="$1"; local v="$2"
    # Do nothing if key already exists (caller prints error).
    if tool_map_exists "$k"; then
        return 1
    fi

    if [ "$HAS_ASSOC_ARRAY" = true ]; then
        TOOL_MAP[$k]="$v"
    else
        TOOL_MAP_KEYS+=("$k")
        TOOL_MAP_VALS+=("$v")
    fi
    return 0
}

tool_map_keys() {
    if [ "$HAS_ASSOC_ARRAY" = true ]; then
        echo "${!TOOL_MAP[@]}"
    else
        echo "${TOOL_MAP_KEYS[@]}"
    fi
}

tool_map_size() {
    if [ "$HAS_ASSOC_ARRAY" = true ]; then
        echo "${#TOOL_MAP[@]}"
    else
        echo "${#TOOL_MAP_KEYS[@]}"
    fi
}

# Iterate over all files in the tools directory
for tool in "$TOOLS_PATH"/*.sh
do
	# Check if the file exists before sourcing it
	if [ -f "$tool" ]; then
			# For each file, run it in a subshell and call its `init` function
			# shellcheck disable=SC1090
			if ! init_output=$(source "$tool"; init 2>/dev/null); then
				echo "WARNING: $tool does not contain an init function."
			else
				# Test if the output is a valid JSON and pretty-print it
				if ! pretty_json=$(echo "$init_output" | jq . 2>/dev/null); then
					echo "ERROR: $tool init function has JSON syntax errors."
					exit_clai 1
				else
				# Extract the type from the JSON
				type=$(echo "$pretty_json" | jq -r '.type')
				
				# If the type is "function", extract the function name and store it in the array
				if [ "$type" = "function" ]; then
					# Extract the function name from the JSON.
					function_name=$(echo "$pretty_json" | jq -r '.function.name')
					
					# Check if the function name already exists in the map
					if tool_map_exists "$function_name"; then
						echo "ERROR: $tool tried to claim function name \"$function_name\" which is already claimed"
						exit_clai 1
					else
						# It's a valid function name, append the tool_reason
						# These go into .function.parameters.properties as a tool_reason JSON object, which has type and description
						# And also add .function.parameters.required tool_reason
						
						# Define the tool_reason JSON object
						tool_reason='{"tool_reason": {"type": "string", "description": "Reason why this tool must be used. e.g. \"This will help me ensure that the command runs without errors, by allowing me to verify that the system is in order. If I do not check the system I cannot find an alternative if there are errors.\""}}'
						
						# Add the tool_reason object to the parameters object in the pretty_json JSON
						pretty_json=$(echo "$pretty_json" | jq --argjson new_param "$tool_reason" '.function.parameters.properties += $new_param')
						
						# Add tool_reason to the required array
						pretty_json=$(echo "$pretty_json" | jq --arg new_param "tool_reason" '.function.parameters.required += [$new_param]')
						
						tool_map_set "$function_name" "$tool"
						OPENAI_TOOLS+="$pretty_json,"
					fi
				else
					echo "Unknown tool type \"$type\"."
				fi
			fi
		fi
	fi
done

# Strip the ending , from OPENAI_TOOLS
OPENAI_TOOLS="${OPENAI_TOOLS%,}"

# Hide the cursor while we're working
conceal_cursor

# Check for configuration file existence
if [ ! -f "$CONFIG_FILE" ]; then
	# Initialize configuration file with default values
	write_private_file "$CONFIG_FILE" <<'EOF'
key=

hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
json_mode=false
temp=0.1
tokens=500
share_command_results=false
result_lines=20
exec_query=
question_query=
error_query=
EOF
fi
chmod 600 "$CONFIG_FILE" 2>/dev/null || true

# Read configuration file
config=$(cat "$CONFIG_FILE")

# Helper to extract values from config without using GNU grep -P
cfg_val() {
    # $1 -> key name
    # outputs the value (may be empty)
    local key="$1"
    echo "$config" | grep -E "^${key}=" | head -n1 | cut -d'=' -f2-
}

set_cfg_val() {
	local key="$1"
	local value="$2"
	local updated_config

	updated_config=$(printf '%s\n' "$config" | awk -v key="$key" -v value="$value" '
		BEGIN { updated=0 }
		index($0, key "=") == 1 {
			if (!updated) {
				print key "=" value
				updated=1
			}
			next
		}
		{ print }
		END {
			if (!updated) {
				print key "=" value
			}
		}
	')
	config="$updated_config"
}

save_config() {
	write_private_file_atomic "$CONFIG_FILE" <<< "$config"
}

toggle_results_sharing() {
	local share_command_results

	share_command_results=$(cfg_val "share_command_results")
	if [ "$share_command_results" = true ]; then
		set_cfg_val "share_command_results" "false"
		if ! save_config; then
			echo "Failed to save CLAI configuration." >&2
			return 1
		fi
		config=$(cat "$CONFIG_FILE")
		echo "Command result sharing is now disabled."
	else
		set_cfg_val "share_command_results" "true"
		if ! save_config; then
			echo "Failed to save CLAI configuration." >&2
			return 1
		fi
		config=$(cat "$CONFIG_FILE")
		warn "Shared command results may contain sensitive stdout/stderr, will be stored in history, and may be sent back to CLAI in later context."
		echo "Command result sharing is now enabled."
	fi

	return 0
}

show_results_sharing() {
	local share_command_results

	share_command_results=$(cfg_val "share_command_results")
	if [ "$share_command_results" = true ]; then
		echo "Command result sharing is enabled."
	else
		echo "Command result sharing is disabled."
	fi

	return 0
}

run_setup_wizard() {
	local key_prompt_value
	local api_prompt_value
	local model_prompt_value
	local key_input
	local api_input
	local model_input
	local setup_input_path="${CLAI_SETUP_INPUT:-/dev/tty}"
	local setup_output_path="${CLAI_SETUP_OUTPUT:-/dev/tty}"

	key_prompt_value=$(cfg_val "key")
	api_prompt_value=$(cfg_val "api")
	model_prompt_value=$(cfg_val "model")

	[ -z "$api_prompt_value" ] && api_prompt_value="https://api.openai.com/v1/chat/completions"
	[ -z "$model_prompt_value" ] && model_prompt_value="gpt-4.1"

	if ! exec 3<"$setup_input_path" 4>"$setup_output_path"; then
		echo "CLAI setup requires an interactive terminal. Run 'clai setup' in a TTY." >&2
		return 1
	fi

	restore_cursor
	printf "CLAI setup\n" >&4
	if [ -n "$key_prompt_value" ]; then
		printf "Press Enter on API key to keep the current value.\n" >&4
	fi
	printf "API key: " >&4
	read -r -s -u 3 key_input
	printf "\n" >&4
	if [ -n "$key_input" ]; then
		key_prompt_value="$key_input"
	fi
	if [ -z "$key_prompt_value" ]; then
		echo "No API key provided. CLAI is not configured." >&2
		exec 3<&-
		exec 4>&-
		return 1
	fi

	printf "API base URL [%s]: " "$api_prompt_value" >&4
	read -r -u 3 api_input
	if [ -n "$api_input" ]; then
		api_prompt_value="$api_input"
	fi

	printf "Model [%s]: " "$model_prompt_value" >&4
	read -r -u 3 model_input
	if [ -n "$model_input" ]; then
		model_prompt_value="$model_input"
	fi

	set_cfg_val "key" "$key_prompt_value"
	set_cfg_val "api" "$api_prompt_value"
	set_cfg_val "model" "$model_prompt_value"
	if ! save_config; then
		echo "Failed to save CLAI configuration." >&2
		exec 3<&-
		exec 4>&-
		return 1
	fi

	config=$(cat "$CONFIG_FILE")
	echo "CLAI configuration updated."
	exec 3<&-
	exec 4>&-
	return 0
}

# API Key
if [ "$SHOW_HISTORY_REQUESTED" = true ]; then
	if handle_show_history; then
		exit_clai 0
	fi
	exit_clai 1
fi

if [ "$SHOW_RESULTS_SHARING_REQUESTED" = true ]; then
	if show_results_sharing; then
		exit_clai 0
	fi
	exit_clai 1
fi

if [ "$TOGGLE_RESULTS_SHARING_REQUESTED" = true ]; then
	if toggle_results_sharing; then
		exit_clai 0
	fi
	exit_clai 1
fi

OPENAI_KEY=$(cfg_val "key")
if [ "$SETUP_REQUESTED" = true ]; then
	run_setup_wizard || exit_clai 1
	if [ "$1" = "--setup" ] || [ "$USER_QUERY" = "setup" ]; then
		exit_clai 0
	fi
	OPENAI_KEY=$(cfg_val "key")
fi
if [ -z "$OPENAI_KEY" ]; then
	run_setup_wizard || exit_clai 1
	OPENAI_KEY=$(cfg_val "key")
fi

# Extract OpenAI URL from configuration
OPENAI_URL=$(cfg_val "api")

# Extract OpenAI model from configuration
OPENAI_MODEL=$(cfg_val "model")

# Extract OpenAI temperature from configuration
OPENAI_TEMP=$(cfg_val "temp")

# Extract OpenAI system execution query from configuration
OPENAI_EXEC_QUERY=$(cfg_val "exec_query")

# Extract OpenAI system question query from configuration
OPENAI_QUESTION_QUERY=$(cfg_val "question_query")

# Extract OpenAI system error query from configuration
OPENAI_ERROR_QUERY=$(cfg_val "error_query")

# Extract maximum token count from configuration
OPENAI_TOKENS=$(cfg_val "tokens")
#GLOBAL_QUERY+=" All your messages must be less than \"$OPENAI_TOKENS\" tokens."

SHARE_COMMAND_RESULTS=$(cfg_val "share_command_results")
if [ "$SHARE_COMMAND_RESULTS" = true ]; then
	SHARE_COMMAND_RESULTS=true
else
	SHARE_COMMAND_RESULTS=false
fi

RESULT_LINES=$(cfg_val "result_lines")
RESULT_LINES=$(jq -Rn --arg value "$RESULT_LINES" '$value | (tonumber? // 20) | floor')
if [ "$RESULT_LINES" -lt 1 ]; then
	RESULT_LINES=20
fi

# Test if high contrast mode is set in configuration
HI_CONTRAST=$(cfg_val "hi_contrast")
if [ "$HI_CONTRAST" = true ]; then
	INFO_TEXT_COLOR="$RESET_COLOR"
fi

# Test if we should expose current dir
EXPOSE_CURRENT_DIR=$(cfg_val "expose_current_dir")

# Extract the maximum number of persisted conversation turns from configuration
MAX_HISTORY_COUNT=$(cfg_val "max_history_turns")
MAX_HISTORY_COUNT=$(jq -Rn --arg value "$MAX_HISTORY_COUNT" '$value | tonumber? // 10')

load_history

# Test if GPT JSON mode is set in configuration
JSON_MODE_ENABLED=$(cfg_val "json_mode")
if [ "$JSON_MODE_ENABLED" = true ]; then
	RESPONSE_FORMAT_JSON='{"type":"json_object"}'
else
	RESPONSE_FORMAT_JSON='null'
fi

OPENAI_TEMP_JSON=$(jq -Rn --arg value "$OPENAI_TEMP" '$value | tonumber? // 0.1')
OPENAI_TOKENS_JSON=$(jq -Rn --arg value "$OPENAI_TOKENS" '$value | tonumber? // 500')

# Set default query if not provided in configuration
if [ -z "$OPENAI_EXEC_QUERY" ]; then
	OPENAI_EXEC_QUERY="$DEFAULT_EXEC_QUERY"
fi
if [ -z "$OPENAI_QUESTION_QUERY" ]; then
	OPENAI_QUESTION_QUERY="$DEFAULT_QUESTION_QUERY"
fi
if [ -z "$OPENAI_ERROR_QUERY" ]; then
	OPENAI_ERROR_QUERY="$DEFAULT_ERROR_QUERY"
fi

# Helper functions
print_info() {
	[ -z "$1" ] && return
	printf "%b%s%b\n\n" "${PRE_TEXT}${INFO_TEXT_COLOR}" "$1" "${RESET_COLOR}"
}

print_ok() {
	[ -z "$1" ] && return
	printf "%b%s%b\n\n" "${OK_TEXT_COLOR}" "$1" "${RESET_COLOR}"
}

print_error() {
	[ -z "$1" ] && return
	printf "%b%s%b\n\n" "${ERROR_TEXT_COLOR}" "$1" "${RESET_COLOR}"
}

print_cancel() {
	[ -z "$1" ] && return
	printf "%b%s%b\n\n" "${CANCEL_TEXT_COLOR}" "$1" "${RESET_COLOR}"
}

print_cmd() {
	[ -z "$1" ] && return
	printf "%b %s %b\n\n" "${PRE_TEXT}${CMD_BG_COLOR}${CMD_TEXT_COLOR}" "$1" "${RESET_COLOR}"
}

print() {
	printf "%b%b%b\n" "${PRE_TEXT}" "$1" "${RESET_COLOR}"
}

append_history_message() {
	local role="$1"
	local content="$2"

	HISTORY_MESSAGES=$(jq -cn \
		--argjson history "$HISTORY_MESSAGES" \
		--arg role "$role" \
		--arg content "$content" \
		'$history + [{"role": $role, "content": $content}]')
	HISTORY_DIRTY=true
}

append_history_tool_message() {
	local tool_call_id="$1"
	local content="$2"

	HISTORY_MESSAGES=$(jq -cn \
		--argjson history "$HISTORY_MESSAGES" \
		--arg tool_call_id "$tool_call_id" \
		--arg content "$content" \
		'$history + [{"role": "tool", "content": $content, "tool_call_id": $tool_call_id}]')
	HISTORY_DIRTY=true
}

append_history_assistant_tool_call() {
	local tool_call_id="$1"
	local tool_name="$2"
	local tool_args="$3"

	HISTORY_MESSAGES=$(jq -cn \
		--argjson history "$HISTORY_MESSAGES" \
		--arg tool_call_id "$tool_call_id" \
		--arg tool_name "$tool_name" \
		--arg tool_args "$tool_args" \
		'$history + [{
			"role": "assistant",
			"content": null,
			"tool_calls": [{
				"id": $tool_call_id,
				"type": "function",
				"function": {
					"name": $tool_name,
					"arguments": $tool_args
				}
			}]
		}]')
	HISTORY_DIRTY=true
}

append_command_result_message() {
	local command="$1"
	local exit_code="$2"
	local stdout_text="$3"
	local stderr_text="$4"
	local edited="$5"

	HISTORY_MESSAGES=$(jq -cn \
		--argjson history "$HISTORY_MESSAGES" \
		--arg command "$command" \
		--argjson exit_code "$exit_code" \
		--arg stdout_text "$stdout_text" \
		--arg stderr_text "$stderr_text" \
		--argjson edited "$edited" \
		'$history + [{
			"role": "assistant",
			"content": (
				{
					"command_result": {
						"command": $command,
						"exit_code": $exit_code,
						"stdout": $stdout_text,
						"stderr": $stderr_text,
						"edited": $edited
					}
				} | tojson
			)
		}]')
	HISTORY_DIRTY=true
}

read_result_output_file() {
	local output_file="$1"
	local max_lines="$2"
	local line_count
	local trimmed_output

	line_count=$(awk 'END { print NR }' "$output_file")
	if [ -z "$line_count" ] || [ "$line_count" -le 0 ]; then
		return 0
	fi

	if [ "$line_count" -le "$max_lines" ]; then
		cat "$output_file"
		return 0
	fi

	trimmed_output=$(tail -n "$max_lines" "$output_file")
	printf '[truncated to last %s lines]\n%s' "$max_lines" "$trimmed_output"
}

maybe_store_command_result() {
	local command="$1"
	local exit_code="$2"
	local stdout_file="$3"
	local stderr_file="$4"
	local edited="$5"
	local trimmed_stdout
	local trimmed_stderr

	if [ "$SHARE_COMMAND_RESULTS" != true ]; then
		return 0
	fi

	trimmed_stdout=$(read_result_output_file "$stdout_file" "$RESULT_LINES")
	trimmed_stderr=$(read_result_output_file "$stderr_file" "$RESULT_LINES")
	append_command_result_message "$command" "$exit_code" "$trimmed_stdout" "$trimmed_stderr" "$edited"
}

repair_truncated_json() {
	local reply="$1"
	local repaired="$reply"

	if (( $(tr -cd '"' <<< "$repaired" | wc -c) % 2 != 0 )); then
		repaired+="\""
	fi

	while [[ $(tr -cd '{' <<< "$repaired" | wc -c) -gt $(tr -cd '}' <<< "$repaired" | wc -c) ]]; do
		repaired+="}"
	done

	if echo "$repaired" | jq -e . >/dev/null 2>&1; then
		echo "$repaired"
	else
		echo "$reply"
	fi
}

build_template_messages() {
	local query_type="$1"
	local system_content="$2"

	case "$query_type" in
		question)
			jq -cn --arg system "$system_content" '[
				{"role": "system", "content": $system},
				{"role": "user", "content": "how do I list all files?"},
				{"role": "assistant", "content": "{ \"info\": \"Use the \\\"ls\\\" command to with the \\\"-a\\\" flag to list all files, including hidden ones, in the current directory.\" }"},
				{"role": "user", "content": "how do I recursively list all the files?"},
				{"role": "assistant", "content": "{ \"info\": \"Use the \\\"ls\\\" command to with the \\\"-aR\\\" flag to list all files recursively, including hidden ones, in the current directory.\" }"},
				{"role": "user", "content": "how do I print hello world?"},
				{"role": "assistant", "content": "{ \"info\": \"Use the \\\"echo\\\" command to print text, and \\\"echo \\\"hello world\\\"\\\" to print your specified text.\" }"},
				{"role": "user", "content": "how do I autocomplete commands?"},
				{"role": "assistant", "content": "{ \"info\": \"Press the Tab key to autocomplete commands, file names, and directories.\" }"}
			]'
			;;
		error)
			jq -cn --arg system "$system_content" '[
				{"role": "system", "content": $system},
				{"role": "user", "content": "You executed \\\"start avidemux\\\". Which returned error \\\"avidemux: command not found\\\"."},
				{"role": "assistant", "content": "{ \"cmd\": \"sudo install avidemux\", \"info\": \"This means that the application \\\"avidemux\\\" was not found. Try installing it.\" }"},
				{"role": "user", "content": "You executed \\\"cd \\\"hell word\\\"\\\". Which returned error \\\"cd: hell word: No such file or directory\\\"."},
				{"role": "assistant", "content": "{ \"cmd\": \"cd \\\"wORLD helloz\\\"\", \"info\": \"The error indicates that the \\\"wORLD helloz\\\" directory does not exist. However, the current directory contains a \\\"hello world\\\" directory we can try instead.\" }"},
				{"role": "user", "content": "You executed \\\"cat \\\"in .sh.\\\"\\\". Which returned error \\\"cat: in .sh: No such file or directory\\\"."},
				{"role": "assistant", "content": "{ \"cmd\": \"cat \\\"install.sh\\\"\", \"info\": \"The cat command could not find the \\\"in .sh\\\" file in the current directory. However, the current directory contains a file called \\\"install.sh\\\".\" }"}
			]'
			;;
		*)
			jq -cn --arg system "$system_content" '[
				{"role": "system", "content": $system},
				{"role": "user", "content": "list all files"},
				{"role": "assistant", "content": "{ \"cmd\": \"ls -a\", \"info\": \"\\\"ls\\\" with the flag \\\"-a\\\" will list all files, including hidden ones, in the current directory\" }"},
				{"role": "user", "content": "start avidemux"},
				{"role": "assistant", "content": "{ \"cmd\": \"avidemux\", \"info\": \"start the Avidemux video editor, if it is installed on the system and available for the current user\" }"},
				{"role": "user", "content": "print hello world"},
				{"role": "assistant", "content": "{ \"cmd\": \"echo \\\"hello world\\\"\", \"info\": \"\\\"echo\\\" will print text, while \\\"echo \\\"hello world\\\"\\\" will print your text\" }"},
				{"role": "user", "content": "remove the hello world folder"},
				{"role": "assistant", "content": "{ \"cmd\": \"rm -r  \\\"hello world\\\"\", \"info\": \"\\\"rm\\\" with the \\\"-r\\\" flag will remove the \\\"hello world\\\" folder and its contents recursively\" }"},
				{"role": "user", "content": "move into the hello world folder"},
				{"role": "assistant", "content": "{ \"cmd\": \"cd \\\"hello world\\\"\", \"info\": \"\\\"cd\\\" will let you change directory to \\\"hello world\\\"\" }"},
				{"role": "user", "content": "add /home/user/.local/bin to PATH"},
				{"role": "assistant", "content": "{ \"cmd\": \"export PATH=/home/user/.local/bin:PATH\", \"info\": \"\\\"export\\\" has the ability to add \\\"/some/path\\\" to your PATH environment variable for the current session. the specified path already exists in your PATH environment variable since before\" }"}
			]'
			;;
	esac
}

build_payload() {
	local messages_json="$1"
	local tools_json='[]'

	if [ -n "$OPENAI_TOOLS" ]; then
		tools_json=$(printf '[%s]' "$OPENAI_TOOLS")
	fi

	jq -cn \
		--arg model "$OPENAI_MODEL" \
		--argjson max_tokens "$OPENAI_TOKENS_JSON" \
		--argjson temperature "$OPENAI_TEMP_JSON" \
		--argjson messages "$messages_json" \
		--argjson response_format "$RESPONSE_FORMAT_JSON" \
		--argjson tools "$tools_json" '
		{
			"model": $model,
			"max_tokens": $max_tokens,
			"temperature": $temperature,
			"messages": $messages
		}
		+ (if $response_format == null then {} else {"response_format": $response_format} end)
		+ (if ($tools | length) == 0 then {} else {"tools": $tools, "tool_choice": "auto"} end)'
}

run_cmd() {
	local command="$1"
	local edited="${2:-false}"
	local stdout_tmp
	local stderr_tmp
	local exit_status

	stdout_tmp=$(create_secure_temp "${SESSION_TMPDIR}/clai-command-stdout.XXXXXX.log") || return 1
	stderr_tmp=$(create_secure_temp "${SESSION_TMPDIR}/clai-command-stderr.XXXXXX.log") || {
		rm -f "$stdout_tmp"
		return 1
	}

	if eval "$command" > >(tee "$stdout_tmp") 2> >(tee "$stderr_tmp" >&2); then
		maybe_store_command_result "$command" 0 "$stdout_tmp" "$stderr_tmp" "$edited"
		# OK
		print_ok "[ok]"
		rm -f "$stdout_tmp" "$stderr_tmp"
		return 0
	else
		# ERROR
		exit_status=$?
		output=$(cat "$stderr_tmp")
		maybe_store_command_result "$command" "$exit_status" "$stdout_tmp" "$stderr_tmp" "$edited"
		LAST_ERROR="${output#*"$0": line *: }"
		echo "$LAST_ERROR"
		rm -f "$stdout_tmp" "$stderr_tmp"
		
		# Ask if we should examine the error
		if [ ${#LAST_ERROR} -gt 1 ]; then
			print_error "[error]"
			echo -n "${PRE_TEXT}examine error? [y/N]: "
			restore_cursor
			read -n 1 -r -s answer
			
			# Did the user want to examine the error?
			if [ "$answer" == "Y" ] || [ "$answer" == "y" ]; then
				echo "yes";echo
				USER_QUERY="You executed \"$1\". Which returned error \"$LAST_ERROR\"."
				QUERY_TYPE="error"
				NEEDS_TO_RUN=true
				SKIP_USER_QUERY_RESET=true
			else
				echo "no";echo
			fi
		else
			print_cancel "[cancel]"
		fi
		return 1
	fi
}

run_tool() {
	TOOL_ID="$1"
	TOOL_NAME="$2"
	TOOL_ARGS="$3"
	TOOL_OUTPUT=""
	
	# Get the function TOOL_NAME from TOOL_MAP IF IT EXISTS!
	if ! tool_map_exists "$TOOL_NAME"; then
		TOOL_SCRIPT=""
		TOOL_OUTPUT=""
	else
		TOOL_SCRIPT="$(tool_map_get "$TOOL_NAME")"
		
		TOOL_REASON=$(echo "$TOOL_ARGS" | jq -r '.tool_reason')
		TOOL_ARGS_READABLE=$(echo "$TOOL_ARGS" | jq -r 'del(.tool_reason)|to_entries|map("\(.key): \(.value)")|.[]' | paste -sd ',' - | awk '{gsub(/,/, ", "); print}')
		print_info "$TOOL_REASON"
		print_info "Using tool \"$TOOL_NAME\" $TOOL_ARGS_READABLE"
		
				echo "$TOOL_NAME" >> "$TOOL_LOG_FILE"
				echo "$TOOL_ARGS_READABLE" >> "$TOOL_LOG_FILE"
		
			# Run the execute function from the TOOL_SCRIPT
			# shellcheck disable=SC1090
			TOOL_OUTPUT=$(source "$TOOL_SCRIPT"; execute "$TOOL_ARGS")
				echo "$TOOL_OUTPUT" >> "$TOOL_LOG_FILE"
				echo "" >> "$TOOL_LOG_FILE"
			# Trim the output to 1000 characters
			TOOL_OUTPUT=${TOOL_OUTPUT:0:1000}
		fi

		# Apply tool output to message history
		append_history_tool_message "$TOOL_ID" "$TOOL_OUTPUT"
		
		# Prepare the next run
		NEEDS_TO_RUN=true
	SKIP_USER_QUERY=true
	SKIP_USER_QUERY_RESET=true
	SKIP_SYSTEM_MSG=true
}

# Are we entering interactive mode?
if [ -z "$USER_QUERY" ]; then
	INTERACTIVE_MODE=true
	print "🤖 ${TITLE_TEXT_COLOR}CLAI v${VERSION}${RESET_COLOR}"
	# List all tools loaded in TOOL_MAP
	# Get number of tools
	if [ "$(tool_map_size)" -gt 0 ]; then
		echo
		print "🔧 ${TITLE_TEXT_COLOR}Activated Tools${RESET_COLOR}"
		for tool in $(tool_map_keys); do
			tool_path="$(tool_map_get "$tool")"
			print "${TITLE_TEXT_COLOR}$tool${RESET_COLOR} from ${tool_path##*/}"
		done
	fi
	echo
	print_info "$INTERACTIVE_INFO"
else
	INTERACTIVE_MODE=false
	NEEDS_TO_RUN=true
fi

# Run as long as we're oin interactive mode, needs to run, or awaiting tool reponse
while [ "$INTERACTIVE_MODE" = true ] || [ "$NEEDS_TO_RUN" = true ] || [ "$AWAIT_TOOL_REPONSE" = true ]; do
	# Ask for user query if we're in Interactive Mode
	if [ "$SKIP_USER_QUERY" != true ]; then
			while [ -z "$USER_QUERY" ]; do
				# No query, prompt user for query
				restore_cursor
					read -e -r -p "CLAI> " USER_QUERY
				conceal_cursor
			
				# Check if user wants to quit
				if [ "$USER_QUERY" == "exit" ]; then
						restore_cursor
					print_info "Bye!"
					exit_clai 0
				fi
		done
		
		fi
	
	conceal_cursor
	
	# Pretty up user query
	USER_QUERY=$(echo "$USER_QUERY" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

	if is_clear_history_request "$USER_QUERY"; then
		clear_history_status=0
		echo -ne "$CLEAR_LINE\r"
		handle_clear_history
		clear_history_status=$?
		QUERY_TYPE=""
		USER_QUERY=""
		SKIP_USER_QUERY=false
		SKIP_USER_QUERY_RESET=false
		restore_cursor
		if [ "$INTERACTIVE_MODE" != true ]; then
			if [ "$clear_history_status" -eq 0 ]; then
				exit_clai 0
			fi
			exit_clai 1
		fi
		if [ "$clear_history_status" -ne 0 ]; then
			continue
		fi
		continue
	fi
	
	# Determine if we should use the question query or the execution query
	if [ -z "$QUERY_TYPE" ]; then
		if [ ${#USER_QUERY} -gt 0 ]; then
			if [[ "$USER_QUERY" == *"?"* ]]; then
				QUERY_TYPE="question"
			else
				QUERY_TYPE="execute"
			fi
		fi
	fi
	
		# Apply the correct query message history
		# The options are "execute", "question" and "error"
		if [ "$QUERY_TYPE" == "question" ]; then
			CURRENT_QUERY_TYPE_MSG="${OPENAI_QUESTION_QUERY}"
			OPENAI_TEMPLATE_MESSAGES=$(build_template_messages "question" "${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}")
		elif [ "$QUERY_TYPE" == "error" ]; then
			CURRENT_QUERY_TYPE_MSG="${OPENAI_ERROR_QUERY}"
			OPENAI_TEMPLATE_MESSAGES=$(build_template_messages "error" "${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}")
		else
			CURRENT_QUERY_TYPE_MSG="${OPENAI_EXEC_QUERY}"
			OPENAI_TEMPLATE_MESSAGES=$(build_template_messages "execute" "${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}")
		fi
	
	# Notify the user about our progress
	echo -ne "${PRE_TEXT}  $PROGRESS_TEXT"
	
	# Start the spinner in the background
	spinner() {
		while :; do
			for (( i=0; i<${#PROGRESS_ANIM}; i++ )); do
				sleep 0.1
				# Print a carriage return (\r) and then the spinner character
				echo -ne "\r${PRE_TEXT}${PROGRESS_ANIM:$i:1}"
			done
		done
	}
	spinner & # Start the spinner
	spinner_pid=$! # Save the spinner's PID
	
		# Prepare system message
	if [ "$SKIP_SYSTEM_MSG" != true ]; then
		sys_msg=""
		# Directory and content exposure
		# Check if EXPOSE_CURRENT_DIR is true
			if [ "$EXPOSE_CURRENT_DIR" = true ]; then
				sys_msg+="User is working from directory \"$(pwd)\"."
			fi
			# Apply date
			sys_msg+=" The current date is Y-m-d H:M $(date "+%Y-%m-%d %H:%M")."
		# Apply dynamic system query
		sys_msg+="$DYNAMIC_SYSTEM_QUERY"
			append_history_message "system" "$sys_msg"
		fi
		
		# Apply the user to the message history
		if [ ${#USER_QUERY} -gt 0 ]; then
			append_history_message "user" "$USER_QUERY"
		fi
		
		# Construct the JSON payload if we don't already have one
		if [ -z "$JSON_PAYLOAD" ]; then
			MESSAGES_JSON=$(jq -cn \
				--argjson template "$OPENAI_TEMPLATE_MESSAGES" \
				--argjson history "$HISTORY_MESSAGES" \
				--arg extra_prompt "$CURRENT_QUERY_TYPE_MSG Respond in less than $OPENAI_TOKENS tokens." \
				'$template + $history + [{"role": "system", "content": $extra_prompt}]')
			JSON_PAYLOAD=$(build_payload "$MESSAGES_JSON")
		fi
		
		# Do we have a special URL?
		if [ -z "$SPECIAL_API_URL" ]; then
			URL="$OPENAI_URL"
	else
		URL="$SPECIAL_API_URL"
	fi
	
		# Save the payload to a tmp JSON file
		echo "$JSON_PAYLOAD" > "$PAYLOAD_FILE"
		
		# Send request to OpenAI API
		: > "$CURL_ERROR_FILE"
		HTTP_CODE=$(curl \
			--silent \
			--show-error \
			--output "$RESPONSE_FILE" \
			--write-out '%{http_code}' \
			-X POST \
			-H "Authorization:Bearer $OPENAI_KEY" \
			-H "Content-Type:application/json" \
			-d "$JSON_PAYLOAD" \
			"$URL" 2>"$CURL_ERROR_FILE")
		CURL_STATUS=$?
		
		RESPONSE=$(cat "$RESPONSE_FILE" 2>/dev/null)
		
		# Stop the spinner
		kill $spinner_pid
	wait $spinner_pid 2>/dev/null
	
	# Reset the JSON_PAYLOAD
	JSON_PAYLOAD=""
	
	# Reset the needs to run flag
	NEEDS_TO_RUN=false
	
	# Reset SKIP_USER_QUERY flag
	SKIP_USER_QUERY=false
	
	# Reset SKIP_SYSTEM_MSG flag
	SKIP_SYSTEM_MSG=false
	
		# Reset user query
		USER_QUERY=""

			if [ $CURL_STATUS -ne 0 ]; then
				echo -ne "$CLEAR_LINE\r"
				CURL_ERROR=$(cat "$CURL_ERROR_FILE" 2>/dev/null)
				if [ -z "$CURL_ERROR" ]; then
					CURL_ERROR="Request failed before the API responded."
				fi
				print_error "$CURL_ERROR"
				restore_cursor
				exit_clai 1
			fi

			if ! jq -e . "$RESPONSE_FILE" >/dev/null 2>&1; then
				echo -ne "$CLEAR_LINE\r"
				print_error "The API returned a non-JSON response (HTTP $HTTP_CODE)."
				restore_cursor
				exit_clai 1
			fi

		if [ "$HTTP_CODE" -lt 200 ] || [ "$HTTP_CODE" -ge 300 ]; then
			echo -ne "$CLEAR_LINE\r"
			API_ERROR=$(jq -r '.error.message // empty' "$RESPONSE_FILE")
				if [ -z "$API_ERROR" ]; then
					API_ERROR="The API request failed with HTTP status $HTTP_CODE."
				fi
				print_error "$API_ERROR"
				restore_cursor
				exit_clai 1
			fi
		
		# Is response empty?
		if [ -z "$RESPONSE" ]; then
			# We didn't get a reply
			print_info "$NO_REPLY_TEXT"
			restore_cursor
			exit_clai 1
		fi
	
		# Extract the reply from the JSON response
		REPLY=$(jq -r '.choices[0].message.content // ""' "$RESPONSE_FILE")
		
		# Was there an error?
		if [ ${#REPLY} -le 1 ]; then
			REPLY=$(jq -r '.error.message // "An unknown error occurred."' "$RESPONSE_FILE")
		fi
	
	echo -ne "$CLEAR_LINE\r"
	
	# Check if there was a reason for stopping
		FINISH_REASON=$(jq -r '.choices[0].finish_reason // ""' "$RESPONSE_FILE")
	
	# If the reason IS NOT stop
	if [ "$FINISH_REASON" != "stop" ]; then
		if [ "$FINISH_REASON" == "length" ]; then
			
                        REPLY=$(repair_truncated_json "$REPLY")

                        # Replace any unescaped single backslashes with double backslashes
                        REPLY="${REPLY//\\\\/\\\\\\\\}"
		elif [ "$FINISH_REASON" == "content_filter" ]; then
			REPLY="Your query was rejected."
		elif [ "$FINISH_REASON" == "tool_calls" ]; then
			# One or multiple tools were called for
				TOOL_CALLS_COUNT=$(jq '.choices[0].message.tool_calls | length' "$RESPONSE_FILE")
				
				for ((i=0; i<TOOL_CALLS_COUNT; i++)); do
					TOOL_ID=$(jq -r '.choices[0].message.tool_calls['"$i"'].id' "$RESPONSE_FILE")
					TOOL_NAME=$(jq -r '.choices[0].message.tool_calls['"$i"'].function.name' "$RESPONSE_FILE")
					TOOL_ARGS=$(jq -r '.choices[0].message.tool_calls['"$i"'].function.arguments' "$RESPONSE_FILE")
					
					# Get return from run_tool and apply to our history
					append_history_assistant_tool_call "$TOOL_ID" "$TOOL_NAME" "$TOOL_ARGS"
					
					run_tool "$TOOL_ID" "$TOOL_NAME" "$TOOL_ARGS"
				done
			REPLY=""
		fi
	fi
	
		# If we still have a reply
		if [ ${#REPLY} -gt 1 ]; then
			JSON_CONTENT=$(printf '%s' "$REPLY" | jq -c . 2>/dev/null)
			
			# Was there JSON content?
			if [ ${#JSON_CONTENT} -le 1 ]; then
				# No JSON content, use the REPLY as structured info text
				JSON_CONTENT=$(jq -cn --arg info "$REPLY" '{"info": $info}')
			fi
			
			# Apply the message to history
			append_history_message "assistant" "$JSON_CONTENT"
		
		# Extract cmd
		CMD=$(echo "$JSON_CONTENT" | jq -r '.cmd // ""' 2>/dev/null)
		
		# Extract info
		INFO=$(echo "$JSON_CONTENT" | jq -r '.info // ""' 2>/dev/null)
		
		# Check if CMD is empty
		if [ ${#CMD} -le 0 ]; then
			# Not a command
			if [ ${#INFO} -le 0 ]; then
				# No info
				print_info "$REPLY"
				else
					# Print info
					print_info "$INFO"
				fi
				restore_cursor
			else
			# Make sure we have some info
			if [ ${#INFO} -le 0 ]; then
				INFO="warning: no information"
			fi
			
			# Print command and information
			print_cmd "$CMD"
			print_info "$INFO"
			
				# Ask for user command confirmation
				echo -n "${PRE_TEXT}execute command? [y/e/N]: "
				restore_cursor
				read -n 1 -r -s answer
			
			# Did the user want to edit the command?
			if [ "$answer" == "Y" ] || [ "$answer" == "y" ]; then
				# RUN
				echo "yes";echo
				run_cmd "$CMD" false
			elif [ "$answer" == "E" ] || [ "$answer" == "e" ]; then
				# EDIT
				echo -ne "$CLEAR_LINE\r"
				if [ -n "$CLAI_EDIT_COMMAND_OVERRIDE" ]; then
					CMD="$CLAI_EDIT_COMMAND_OVERRIDE"
				else
					read -e -r -p "${PRE_TEXT}edit command: " -i "$CMD" CMD
				fi
				echo
				run_cmd "$CMD" true
			else
				# CANCEL
				echo "no";echo
				print_cancel "[cancel]"
			fi
		fi
	fi
	
	# Reset user query type unless SKIP_USER_QUERY_RESET is true
	if [ "$SKIP_USER_QUERY_RESET" != true ]; then
		QUERY_TYPE=""
	fi
	SKIP_USER_QUERY_RESET=false
	
done

# We're done
exit_clai 0
