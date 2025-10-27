#!/bin/bash
# -*- Mode: sh; coding: utf-8; indent-tabs-mode: t; tab-width: 4 -*-

# Bash AI
# https://github.com/Hezkore/bash-ai

# Make sure required tools are installed
if [ ! -x "$(command -v jq)" ]; then
	echo "ERROR: Bash AI requires jq to be installed."
	exit 1
fi
if [ ! -x "$(command -v curl)" ]; then
	echo "ERROR: Bash AI requires curl to be installed."
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

# Version of Bash AI
VERSION="1.0.5"

# Global variables
PRE_TEXT="  "  # Prefix for text output
NO_REPLY_TEXT="¯\_(ツ)_/¯"  # Text for no reply
INTERACTIVE_INFO="Hi! Feel free to ask me anything or give me a task. Type \"exit\" when you're done."  # Text for interactive mode intro
PROGRESS_TEXT="Thinking..."
PROGRESS_ANIM="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
HISTORY_MESSAGES=""  # Placeholder for history messages, this will be updated later

# Theme colors
CMD_BG_COLOR="\e[48;5;236m"  # Background color for cmd suggestions
CMD_TEXT_COLOR="\e[38;5;203m"  # Text color for cmd suggestions
INFO_TEXT_COLOR="\e[90;3m"  # Text color for all information messages
ERROR_TEXT_COLOR="\e[91m"  # Text color for cmd errors messages
CANCEL_TEXT_COLOR="\e[93m"  # Text color cmd for cancellation message
OK_TEXT_COLOR="\e[92m"  # Text color for cmd success message
TITLE_TEXT_COLOR="\e[1m"  # Text color for the Bash AI title

# Terminal control constants
CLEAR_LINE="\033[2K\r"
HIDE_CURSOR="\e[?25l"
SHOW_CURSOR="\e[?25h"
RESET_COLOR="\e[0m"

# Default query constants, these are used as default values for different types of queries
DEFAULT_EXEC_QUERY="Return only a single compact JSON object containing 'cmd' and 'info' fields. 'cmd' must always contain one or multiple commands to perform the task specified in the user query. 'info' must always contain a single-line string detailing the actions 'cmd' will perform and the purpose of all command flags. 'cmd' may output a shell script to perform complex tasks. 'cmd' may be omittied as a last resort if no command can be suggested."
DEFAULT_QUESTION_QUERY="Return only a single compact JSON object containing a 'info' field. 'info' must always contain a single-line string terminal-related answer to the user query."
DEFAULT_ERROR_QUERY="Return only a single compact JSON object containing 'cmd' and 'info' fields. 'cmd' is optional. 'cmd' must always contain a suggestion on how to fix, solve or repair the error in the user query. 'info' must always be a single-line string explaining what the error in the user query means, why it happened, and why 'cmd' might fix it. Use your tools to find out why the error occured and offer alternatives."
DYNAMIC_SYSTEM_QUERY="" # After most user queries, we'll add some dynamic system information to the query

# Global query variable, this will be updated with specific user and system information
GLOBAL_QUERY="You are Bash AI (bai) v${VERSION}. You are an advanced Bash shell script. You are located at \"$0\". You do not have feelings or emotions, do not convey them. Please give precise curt answers. Please do not include any sign off phrases or platitudes, only respond precisely to the user. Bash AI is made by Hezkore. You execute the tasks the user asks from you by utilizing the terminal and shell commands. No task is too big. Always assume the query is terminal and shell related. You support user plugins called \"tools\" that extends your capabilities, more info and plugins can be found on the Bash AI homepage. The Bash AI homepage is \"https://github.com/hezkore/bash-ai\". You always respond with a single JSON object containing 'cmd' and 'info' fields. We are always in the terminal. The user is using \"$UNIX_NAME\" and specifically distribution \"$DISTRO_INFO\". The users username is \"$USER\" with home \"$HOME\". You must always use LANG $LANG and LC_TIME $LC_TIME."

# Configuration file path
CONFIG_FILE=~/.config/bai.cfg
#GLOBAL_QUERY+=" Your configuration file path \"$CONFIG_FILE\"."

# Test if we're in Vim
if [ -n "$VIMRUNTIME" ]; then
	CMD_BG_COLOR=""
	CMD_TEXT_COLOR=""
	INFO_TEXT_COLOR=""
	ERROR_TEXT_COLOR=""
	CANCEL_TEXT_COLOR=""
	OK_TEXT_COLOR=""
	TITLE_TEXT_COLOR=""
	CLEAR_LINE=""
	HIDE_CURSOR=""
	SHOW_CURSOR=""
	RESET_COLOR=""
	
	# Make sure system message reflects that we're in Vim
	DYNAMIC_SYSTEM_QUERY+="User is inside \"$VIM\". You are in the Vim terminal."
	
	# Use the Vim history file
	HISTORY_FILE=/tmp/baihistory_vim.txt
else
	# Use the default history file
	HISTORY_FILE=/tmp/baihistory_com.txt
fi

# Update info about history file
#GLOBAL_QUERY+=" Your message history file path is \"$HISTORY_FILE\"."

# Tools
OPENAI_TOOLS=""
TOOLS_PATH=~/.bai_tools

# Create the directory only if it doesn't exist
if [ ! -d "$TOOLS_PATH" ]; then
	mkdir -p "$TOOLS_PATH"
fi
echo "" > /tmp/bai_tool_output.txt

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

# Bash ≥4 or any zsh support associative arrays.  We attempt a silent test that
# works for both.
if (unset TEST 2>/dev/null; declare -A TEST 2>/dev/null); then
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
		init_output=$(source "$tool"; init 2>/dev/null)
		
		# Check the exit status of the last command
		if [ $? -ne 0 ]; then
			echo "WARNING: $tool does not contain an init function."
		else
			# Test if the output is a valid JSON and pretty-print it
			pretty_json=$(echo "$init_output" | jq . 2>/dev/null)
			
			if [ $? -ne 0 ]; then
				echo "ERROR: $tool init function has JSON syntax errors."
				exit 1
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
						exit 1
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
trap 'printf "%b" "$SHOW_CURSOR"' EXIT # Make sure the cursor is shown when the script exits
printf "%b" "$HIDE_CURSOR"

# Check for configuration file existence
if [ ! -f "$CONFIG_FILE" ]; then
	# Initialize configuration file with default values
	{
		echo "key="
		echo ""
		echo "hi_contrast=false"
		echo "expose_current_dir=true"
		echo "max_history=10"
		echo "api=https://api.openai.com/v1/chat/completions"
		echo "model=gpt-4o-mini"
		echo "json_mode=false"
		echo "temp=0.1"
		echo "tokens=500"
		echo "exec_query="
		echo "question_query="
		echo "error_query="
	} >> "$CONFIG_FILE"
fi

# Read configuration file
config=$(cat "$CONFIG_FILE")

# Helper to extract values from config without using GNU grep -P
cfg_val() {
    # $1 -> key name
    # outputs the value (may be empty)
    local key="$1"
    echo "$config" | grep -E "^${key}=" | head -n1 | cut -d'=' -f2-
}

# API Key
OPENAI_KEY=$(cfg_val "key")
if [ -z "$OPENAI_KEY" ]; then
	 # Prompt user to input OpenAI key if not found
	echo "To use Bash AI, please input your OpenAI key into the config file located at $CONFIG_FILE"
	printf "%b" "$SHOW_CURSOR"
	exit 1
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

# Test if high contrast mode is set in configuration
HI_CONTRAST=$(cfg_val "hi_contrast")
if [ "$HI_CONTRAST" = true ]; then
	INFO_TEXT_COLOR="$RESET_COLOR"
fi

# Test if we should expose current dir
EXPOSE_CURRENT_DIR=$(cfg_val "expose_current_dir")

# Extract maximum history message count from configuration
MAX_HISTORY_COUNT=$(cfg_val "max_history")

# Test if GPT JSON mode is set in configuration
JSON_MODE=$(cfg_val "json_mode")
if [ "$JSON_MODE" = true ]; then
	JSON_MODE="\"response_format\": { \"type\": \"json_object\" },"
else
	JSON_MODE=""
fi

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

json_safe() {
        # FIX this is a bad way of doing this, and it misses many unsafe characters
        echo "$1" | perl -pe 's/\\/\\\\/g; s/"/\\"/g; s/\033/\\\\033/g; s/\n/ /g; s/\r/\\r/g; s/\t/\\t/g'
}

repair_truncated_json() {
        local reply="$1"
        local repaired="$reply"

        # Ensure we have an even number of double quotes by appending a closing quote
        if (( $(tr -cd '"' <<< "$repaired" | wc -c) % 2 != 0 )); then
                repaired+="\""
        fi

        # Ensure that opening braces have matching closing braces
        while [[ $(tr -cd '{' <<< "$repaired" | wc -c) -gt $(tr -cd '}' <<< "$repaired" | wc -c) ]]; do
                repaired+="}"
        done

        # Only use the repaired string if it parses as valid JSON
        if echo "$repaired" | jq -e . >/dev/null 2>&1; then
                echo "$repaired"
        else
                echo "$reply"
        fi
}

run_cmd() {
	tmpfile=$(mktemp)
	if eval "$1" 2>"$tmpfile"; then
		# OK
		print_ok "[ok]"
		rm "$tmpfile"
		return 0
	else
		# ERROR
		output=$(cat "$tmpfile")
		LAST_ERROR="${output#*"$0": line *: }"
		echo "$LAST_ERROR"
		rm "$tmpfile"
		
		# Ask if we should examine the error
		if [ ${#LAST_ERROR} -gt 1 ]; then
			print_error "[error]"
			echo -n "${PRE_TEXT}examine error? [y/N]: "
			printf "%b" "$SHOW_CURSOR"
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
		
		echo "$TOOL_NAME" >> /tmp/bai_tool_output.txt
		echo "$TOOL_ARGS_READABLE" >> /tmp/bai_tool_output.txt
		
		# Run the execute function from the TOOL_SCRIPT
		TOOL_OUTPUT=$(source "$TOOL_SCRIPT"; execute "$TOOL_ARGS")
		echo "$TOOL_OUTPUT" >> /tmp/bai_tool_output.txt
		echo "" >> /tmp/bai_tool_output.txt
		# Trim the output to 1000 characters
		TOOL_OUTPUT=${TOOL_OUTPUT:0:1000}
		# Make it JSON safe
		TOOL_OUTPUT=$(json_safe "$TOOL_OUTPUT")
	fi
	
	# Apply tool output to message history
	HISTORY_MESSAGES+=',{
		"role": "tool",
		"content": "'"$TOOL_OUTPUT"'",
		"tool_call_id": "'"$TOOL_ID"'"
	}'
	
	# Prepare the next run
	NEEDS_TO_RUN=true
	SKIP_USER_QUERY=true
	SKIP_USER_QUERY_RESET=true
	SKIP_SYSTEM_MSG=true
}

# Make sure all queries are JSON safe
DEFAULT_EXEC_QUERY=$(json_safe "$DEFAULT_EXEC_QUERY")
DEFAULT_QUESTION_QUERY=$(json_safe "$DEFAULT_QUESTION_QUERY")
DEFAULT_ERROR_QUERY=$(json_safe "$DEFAULT_ERROR_QUERY")
GLOBAL_QUERY=$(json_safe "$GLOBAL_QUERY")
DYNAMIC_SYSTEM_QUERY=$(json_safe "$DYNAMIC_SYSTEM_QUERY")

# User AI query and Interactive Mode
USER_QUERY=$*

# Are we entering interactive mode?
if [ -z "$USER_QUERY" ]; then
	INTERACTIVE_MODE=true
	print "🤖 ${TITLE_TEXT_COLOR}Bash AI v${VERSION}${RESET_COLOR}"
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

# We're ready to run
RUN_COUNT=0

# Run as long as we're oin interactive mode, needs to run, or awaiting tool reponse
while [ "$INTERACTIVE_MODE" = true ] || [ "$NEEDS_TO_RUN" = true ] || [ "$AWAIT_TOOL_REPONSE" = true ]; do
	# Ask for user query if we're in Interactive Mode
	if [ "$SKIP_USER_QUERY" != true ]; then
		while [ -z "$USER_QUERY" ]; do
			# No query, prompt user for query
			printf "%b" "$SHOW_CURSOR"
			read -e -r -p "Bash AI> " USER_QUERY
			printf "%b" "$HIDE_CURSOR"
			
			# Check if user wants to quit
			if [ "$USER_QUERY" == "exit" ]; then
					printf "%b" "$SHOW_CURSOR"
				print_info "Bye!"
				exit 0
			fi
		done
		
		# Make sure the query is JSON safe
		USER_QUERY=$(json_safe "$USER_QUERY")
	fi
	
	printf "%b" "$HIDE_CURSOR"
	
	# Pretty up user query
	USER_QUERY=$(echo "$USER_QUERY" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
	
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
		# QUESTION
		CURRENT_QUERY_TYPE_MSG="${OPENAI_QUESTION_QUERY}"
		OPENAI_TEMPLATE_MESSAGES='{
			"role": "system",
			"content": "'"${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}"'"
		},
		{
			"role": "user",
			"content": "how do I list all files?"
		},
		{
			"role": "assistant",
			"content": "{ \"info\": \"Use the \\\"ls\\\" command to with the \\\"-a\\\" flag to list all files, including hidden ones, in the current directory.\" }"
		},
		{
			"role": "user",
			"content": "how do I recursively list all the files?"
		},
		{
			"role": "assistant",
			"content": "{ \"info\": \"Use the \\\"ls\\\" command to with the \\\"-aR\\\" flag to list all files recursively, including hidden ones, in the current directory.\" }"
		},
		{
			"role": "user",
			"content": "how do I print hello world?"
		},
		{
			"role": "assistant",
			"content": "{ \"info\": \"Use the \\\"echo\\\" command to print text, and \\\"echo \\\"hello world\\\"\\\" to print your specified text.\" }"
		},
		{
			"role": "user",
			"content": "how do I autocomplete commands?"
		},
		{
			"role": "assistant",
			"content": "{ \"info\": \"Press the Tab key to autocomplete commands, file names, and directories.\" }"
		}'
	elif [ "$QUERY_TYPE" == "error" ]; then
		# ERROR
		CURRENT_QUERY_TYPE_MSG="${OPENAI_ERROR_QUERY}"
		OPENAI_TEMPLATE_MESSAGES='{
			"role": "system",
			"content": "'"${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}"'"
		},
		{
			"role": "user",
			"content": "You executed \\\"start avidemux\\\". Which returned error \\\"avidemux: command not found\\\"."
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"sudo install avidemux\", \"info\": \"This means that the application \\\"avidemux\\\" was not found. Try installing it.\" }"
		},
		{
			"role": "user",
			"content": "You executed \\\"cd \\\"hell word\\\"\\\". Which returned error \\\"cd: hell word: No such file or directory\\\"."
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"cd \\\"wORLD helloz\\\"\", \"info\": \"The error indicates that the \\\"wORLD helloz\\\" directory does not exist. However, the current directory contains a \\\"hello world\\\" directory we can try instead.\" }"
		},
		{
			"role": "user",
			"content": "You executed \\\"cat \\\"in .sh.\\\"\\\". Which returned error \\\"cat: in .sh: No such file or directory\\\"."
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"cat \\\"install.sh\\\"\", \"info\": \"The cat command could not find the \\\"in .sh\\\" file in the current directory. However, the current directory contains a file called \\\"install.sh\\\".\" }"
		}'
	else
		# COMMAND
		CURRENT_QUERY_TYPE_MSG="${OPENAI_EXEC_QUERY}"
		OPENAI_TEMPLATE_MESSAGES='{
			"role": "system",
			"content": "'"${GLOBAL_QUERY}${CURRENT_QUERY_TYPE_MSG}"'"
		},
		{
			"role": "user",
			"content": "list all files"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"ls -a\", \"info\": \"\\\"ls\\\" with the flag \\\"-a\\\" will list all files, including hidden ones, in the current directory\" }"
		},
		{
			"role": "user",
			"content": "start avidemux"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"avidemux\", \"info\": \"start the Avidemux video editor, if it is installed on the system and available for the current user\" }"
		},
		{
			"role": "user",
			"content": "print hello world"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"echo \\\"hello world\\\"\", \"info\": \"\\\"echo\\\" will print text, while \\\"echo \\\"hello world\\\"\\\" will print your text\" }"
		},
		{
			"role": "user",
			"content": "remove the hello world folder"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"rm -r  \\\"hello world\\\"\", \"info\": \"\\\"rm\\\" with the \\\"-r\\\" flag will remove the \\\"hello world\\\" folder and its contents recursively\" }"
		},
		{
			"role": "user",
			"content": "move into the hello world folder"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"cd \\\"hello world\\\"\", \"info\": \"\\\"cd\\\" will let you change directory to \\\"hello world\\\"\" }"
		},
		{
			"role": "user",
			"content": "add /home/user/.local/bin to PATH"
		},
		{
			"role": "assistant",
			"content": "{ \"cmd\": \"export PATH=/home/user/.local/bin:PATH\", \"info\": \"\\\"export\\\" has the ability to add \\\"/some/path\\\" to your PATH environment variable for the current session. the specified path already exists in your PATH environment variable since before\" }"
		}'
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
	
	# If this is the first run we apply history
	if [ $RUN_COUNT -eq 0 ]; then
		# Check if the history file exists
		if [ -f "$HISTORY_FILE" ]; then
			# Read the history file
			HISTORY_MESSAGES=$(sed 's/^\[\(.*\)\]$/,\1/' $HISTORY_FILE)
		fi
	fi
	
	# Prepare system message
	if [ "$SKIP_SYSTEM_MSG" != true ]; then
		sys_msg=""
		# Directory and content exposure
		# Check if EXPOSE_CURRENT_DIR is true
		if [ "$EXPOSE_CURRENT_DIR" = true ]; then
			sys_msg+="User is working from directory \\\"$(json_safe "$(pwd)")\\\"."
		fi
		# Apply date
		sys_msg+=" The current date is Y-m-d H:M \\\"$(date "+%Y-%m-%d %H:%M")\\\"."
		# Apply dynamic system query
		sys_msg+="$DYNAMIC_SYSTEM_QUERY"
		# Apply the system message to history
		LAST_HISTORY_MESSAGE=',{
			"role": "system",
			"content": "'"${sys_msg}"'"
		}'
		HISTORY_MESSAGES+="$LAST_HISTORY_MESSAGE"
	fi
	
	# Apply the user to the message history
	if [ ${#USER_QUERY} -gt 0 ]; then
		HISTORY_MESSAGES+=',{
			"role": "user",
			"content": "'${USER_QUERY}'"
		}'
	fi
	
	# Construct the JSON payload if we don't already have one
	if [ -z "$JSON_PAYLOAD" ]; then
		JSON_PAYLOAD='{
			"model": "'"$OPENAI_MODEL"'",
			"max_tokens": '"$OPENAI_TOKENS"',
			"temperature": '"$OPENAI_TEMP"',
			'"$JSON_MODE"'
			"messages": ['"$OPENAI_TEMPLATE_MESSAGES $HISTORY_MESSAGES
				,{\"role\": \"system\", \"content\": \"$CURRENT_QUERY_TYPE_MSG Respond in less than $OPENAI_TOKENS tokens.\"}
			"']'
		
		# Apply tools to payload
		if [ ${#OPENAI_TOOLS} -gt 0 ]; then
			JSON_PAYLOAD+=', "tools": ['"$OPENAI_TOOLS"'], "tool_choice": "auto"'
		fi
		
		# Close the JSON payload
		JSON_PAYLOAD+='}'
	fi
	
	# Prettify the JSON payload and verify it
	JSON_PAYLOAD=$(echo "$JSON_PAYLOAD" | jq .)
	
	# Do we have a special URL?
	if [ -z "$SPECIAL_API_URL" ]; then
		URL="$OPENAI_URL"
	else
		URL="$SPECIAL_API_URL"
	fi
	
	# Save the payload to a tmp JSON file
	echo "$JSON_PAYLOAD" > /tmp/bai_payload.json
	
	# Send request to OpenAI API
	RESPONSE=$(curl -s -X POST -H "Authorization:Bearer $OPENAI_KEY" -H "Content-Type:application/json" -d "$JSON_PAYLOAD" "$URL")
	
	# Save reponse to a tmp JSON file
	echo "$RESPONSE" > /tmp/bai_response.json
	
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
	
	# Is response empty?
	if [ -z "$RESPONSE" ]; then
		# We didn't get a reply
		print_info "$NO_REPLY_TEXT"
		printf "%b" "$SHOW_CURSOR"
		exit 1
	fi
	
	# Extract the reply from the JSON response
	REPLY=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // ""')
	
	# Was there an error?
	if [ ${#REPLY} -le 1 ]; then
		REPLY=$(echo "$RESPONSE" | jq -r '.error.message // "An unknown error occurred."')
	fi
	
	echo -ne "$CLEAR_LINE\r"
	
	# Check if there was a reason for stopping
	FINISH_REASON=$(echo "$RESPONSE" | jq -r '.choices[0].finish_reason // ""')
	
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
			TOOL_CALLS_COUNT=$(echo "$RESPONSE" | jq '.choices[0].message.tool_calls | length')
			
			for ((i=0; i<$TOOL_CALLS_COUNT; i++)); do
				TOOL_ID=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls['"$i"'].id')
				TOOL_NAME=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls['"$i"'].function.name')
				TOOL_ARGS=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls['"$i"'].function.arguments')
				
				# Get return from run_tool and apply to our history
				HISTORY_MESSAGES+=',{
					"role": "assistant",
					"content": null,
					"tool_calls": [
						{
							"id": "'"$TOOL_ID"'",
							"type": "function",
							"function": {
								"name": "'"$TOOL_NAME"'",
								"arguments": "'"$(json_safe "$TOOL_ARGS")"'"
							}
						}
					]
				}'
				
				run_tool "$TOOL_ID" "$TOOL_NAME" "$TOOL_ARGS"
			done
			REPLY=""
		fi
	fi
	
	# If we still have a reply
	if [ ${#REPLY} -gt 1 ]; then
		# Try to assemble a JSON object from the REPLY
		JSON_CONTENT=$(echo "$REPLY" | perl -0777 -pe 's/.*?(\{.*?\})(\n| ).*/$1/s')
		JSON_CONTENT=$(echo "$JSON_CONTENT" | jq -r . 2>/dev/null)
		
		# Was there JSON content?
		if [ ${#JSON_CONTENT} -le 1 ]; then
			# No JSON content, use the REPLY as is
			JSON_CONTENT="{\"info\": \"$REPLY\"}"
		fi
		
		# Apply the message to history
		HISTORY_MESSAGES+=',{
			"role": "assistant",
			"content": "'"$(json_safe "$JSON_CONTENT")"'"
		}'
		
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
			printf "%b" "$SHOW_CURSOR"
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
			printf "%b" "$SHOW_CURSOR"
			read -n 1 -r -s answer
			
			# Did the user want to edit the command?
			if [ "$answer" == "Y" ] || [ "$answer" == "y" ]; then
				# RUN
				echo "yes";echo
				run_cmd "$CMD"
			elif [ "$answer" == "E" ] || [ "$answer" == "e" ]; then
				# EDIT
				echo -ne "$CLEAR_LINE\r"
				read -e -r -p "${PRE_TEXT}edit command: " -i "$CMD" CMD
				echo
				run_cmd "$CMD"
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
	
	RUN_COUNT=$((RUN_COUNT+1))
done

# Save the history messages
if [ "$INTERACTIVE_MODE" = false ]; then
	# Add a dummy message at the beginning to make HISTORY_MESSAGES a valid JSON array
	HISTORY_MESSAGES_JSON="[null$HISTORY_MESSAGES]"
	
	# Get the number of messages
	HISTORY_COUNT=$(echo "$HISTORY_MESSAGES_JSON" | jq 'length')
	
	# Convert MAX_HISTORY_COUNT to an integer
	MAX_HISTORY_COUNT_INT=$((MAX_HISTORY_COUNT))
	
	# If the history is too long, remove the oldest messages
	if (( HISTORY_COUNT > MAX_HISTORY_COUNT_INT )); then
		HISTORY_MESSAGES_JSON=$(echo "$HISTORY_MESSAGES_JSON" | jq ".[-$MAX_HISTORY_COUNT_INT:]")
	fi
	
	# Remove the dummy message and write the history to the file
	echo "$HISTORY_MESSAGES_JSON" | jq '.[1:]' | jq -c . > $HISTORY_FILE
fi

# We're done
exit 0
