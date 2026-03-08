#!/usr/bin/env bats

setup() {
  export TEST_HOME
  TEST_HOME="$(mktemp -d "${TMPDIR:-/tmp}/clai-test.XXXXXX")"
  mkdir -p "$TEST_HOME/fakebin"
  mkdir -p "$TEST_HOME/tmp"
}

teardown() {
  rm -rf "$TEST_HOME"
}

write_config() {
  mkdir -p "$TEST_HOME/.config"
  cat > "$TEST_HOME/.config/clai.cfg"
}

make_success_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
payload=""
status_code="200"
response_body='{"choices":[{"message":{"content":"{\"info\":\"stub answer\"}"},"finish_reason":"stop"}]}'
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      payload="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "$TEST_HOME" ]; then
  printf '%s' "$payload" > "$TEST_HOME/curl-request.json"
fi
printf '%s' "$response_body" > "$output"
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_tool_call_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
payload=""
status_code="200"
count_file="$TEST_HOME/curl-call-count"
if [ ! -f "$count_file" ]; then
  printf '0' > "$count_file"
fi
call_count=$(cat "$count_file")
call_count=$((call_count + 1))
printf '%s' "$call_count" > "$count_file"
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      payload="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s' "$payload" > "$TEST_HOME/curl-request-$call_count.json"
if [ "$call_count" -eq 1 ]; then
  printf '%s' '{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"record-note","arguments":"{\"value\":\"hello from tool\",\"tool_reason\":\"Need data from the helper tool.\"}"}}]},"finish_reason":"tool_calls"}]}' > "$output"
else
  printf '%s' '{"choices":[{"message":{"content":"{\"info\":\"tool flow complete\"}"},"finish_reason":"stop"}]}' > "$output"
fi
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_command_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
payload=""
status_code="200"
response_body='{"choices":[{"message":{"content":"{\"cmd\":\"printf executed > \\\"$HOME/cmd-ran.txt\\\"\",\"info\":\"run the stub command\"}"},"finish_reason":"stop"}]}'
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      payload="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "$TEST_HOME" ]; then
  printf '%s' "$payload" > "$TEST_HOME/curl-request.json"
fi
printf '%s' "$response_body" > "$output"
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_result_command_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
payload=""
status_code="200"
response_body='{"choices":[{"message":{"content":"{\"cmd\":\"printf \\\"one\\\\ntwo\\\\nthree\\\\nfour\\\\n\\\"; printf \\\"err-one\\\\nerr-two\\\\nerr-three\\\\n\\\" >&2\",\"info\":\"run the result-capture stub command\"}"},"finish_reason":"stop"}]}'
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      payload="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "$TEST_HOME" ]; then
  printf '%s' "$payload" > "$TEST_HOME/curl-request.json"
fi
printf '%s' "$response_body" > "$output"
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_no_reply_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
payload=""
status_code="200"
response_body='{"choices":[{"message":{},"finish_reason":"stop"}]}'
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      payload="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -n "$TEST_HOME" ]; then
  printf '%s' "$payload" > "$TEST_HOME/curl-request.json"
fi
printf '%s' "$response_body" > "$output"
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_marker_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
printf 'called' > "$TEST_HOME/curl-called"
exit 99
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_malformed_tool_call_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
count_file="$TEST_HOME/curl-call-count"
status_code="200"
if [ ! -f "$count_file" ]; then
  printf '0' > "$count_file"
fi
call_count=$(cat "$count_file")
call_count=$((call_count + 1))
printf '%s' "$call_count" > "$count_file"
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ "$call_count" -eq 1 ]; then
  printf '%s' '{"choices":[{"message":{"content":"","tool_calls":[{"id":"bad_1","type":"function","function":{"name":"record-note","arguments":"not-json"}}]},"finish_reason":"tool_calls"}]}' > "$output"
else
  printf '%s' '{"choices":[{"message":{"content":"{\"info\":\"tool fallback complete\"}"},"finish_reason":"stop"}]}' > "$output"
fi
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_truncated_json_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
status_code="200"
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s' '{"choices":[{"message":{"content":"{\"info\":\"trimmed response\""},"finish_reason":"length"}]}' > "$output"
printf '%s' "$status_code"
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

make_non_json_curl() {
  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s' '<html>not json</html>' > "$output"
printf '200'
EOF
  chmod +x "$TEST_HOME/fakebin/curl"
}

@test "clai bootstraps config and asks for API key when missing" {
  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    bash ./clai.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"CLAI setup"* ]]
  [[ "$output" == *"No API key provided. CLAI is not configured."* ]]
  [ -f "$TEST_HOME/.config/clai.cfg" ]
  [ -d "$TEST_HOME/.local/state/clai" ]
  grep -qx 'key=' "$TEST_HOME/.config/clai.cfg"
  [ "$(stat -c '%a' "$TEST_HOME/.config/clai.cfg")" = "600" ]
  [ "$(stat -c '%a' "$TEST_HOME/.local/state/clai")" = "700" ]
  [ "$(find "$TEST_HOME/tmp" -type f | wc -l)" -eq 0 ]
}

@test "missing key triggers setup wizard and continues the requested command" {
  make_success_curl

  run bash -lc '
    printf "wizard-key\n\n\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "what is the current time?"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"CLAI configuration updated."* ]]
  [[ "$output" == *"stub answer"* ]]
  grep -qx 'key=wizard-key' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'api=https://api.openai.com/v1/chat/completions' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'model=gpt-4.1' "$TEST_HOME/.config/clai.cfg"
}

@test "--install updates config without calling the API" {
  write_config <<'EOF'
key=old-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
json_mode=false
temp=0.1
tokens=500
store_command_results=false
result_lines=20
exec_query=
question_query=
error_query=
EOF

  make_marker_curl

  run bash -lc '
    printf "new-key\nhttps://example.invalid/v1/chat/completions\ncustom-model\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh --install
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"CLAI configuration updated."* ]]
  grep -qx 'key=new-key' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'api=https://example.invalid/v1/chat/completions' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'model=custom-model' "$TEST_HOME/.config/clai.cfg"
  [ ! -e "$TEST_HOME/curl-called" ]
}

@test "\"install\" command reruns setup wizard without calling the API" {
  write_config <<'EOF'
key=existing-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
json_mode=false
temp=0.1
tokens=500
store_command_results=false
result_lines=20
exec_query=
question_query=
error_query=
EOF

  make_marker_curl

  run bash -lc '
    printf "\n\n\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh install
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"CLAI configuration updated."* ]]
  grep -qx 'key=existing-key' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'api=https://api.openai.com/v1/chat/completions' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'model=gpt-4.1' "$TEST_HOME/.config/clai.cfg"
  [ ! -e "$TEST_HOME/curl-called" ]
}

@test "--install fails when updated config cannot be persisted" {
  write_config <<'EOF'
key=old-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
json_mode=false
temp=0.1
tokens=500
store_command_results=false
result_lines=20
exec_query=
question_query=
error_query=
EOF

  make_marker_curl

  cat > "$TEST_HOME/fakebin/mv" <<'EOF'
#!/bin/bash
exit 1
EOF
  chmod +x "$TEST_HOME/fakebin/mv"

  run bash -lc '
    printf "new-key\nhttps://example.invalid/v1/chat/completions\ncustom-model\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh --install
  '

  [ "$status" -eq 1 ]
  [[ "$output" == *"Failed to save CLAI configuration."* ]]
  grep -qx 'key=old-key' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'api=https://api.openai.com/v1/chat/completions' "$TEST_HOME/.config/clai.cfg"
  grep -qx 'model=gpt-4.1' "$TEST_HOME/.config/clai.cfg"
  [ ! -e "$TEST_HOME/curl-called" ]
}

@test "clai cleans transient session files after API transport failure" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=http://127.0.0.1:1
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 1 ]
  [ -d "$TEST_HOME/.local/state/clai" ]
  [ "$(find "$TEST_HOME/tmp" -type f | wc -l)" -eq 0 ]
}

@test "clai builds JSON payloads with jq and handles successful JSON responses" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=true
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  [ -f "$TEST_HOME/.local/state/clai/history_com.json" ]
  [[ "$output" == *"stub answer"* ]]
  jq -e '.messages | length > 0' "$TEST_HOME/curl-request.json" >/dev/null
  jq -e '.response_format.type == "json_object"' "$TEST_HOME/curl-request.json" >/dev/null
  jq -e '.messages | map(select(.role == "system" and (.content | contains("~/.config/clai.cfg")))) | length >= 1' \
    "$TEST_HOME/curl-request.json" >/dev/null
  jq -e '.messages | map(select(.role == "system" and (.content | contains("~/.clai_tools")))) | length >= 1' \
    "$TEST_HOME/curl-request.json" >/dev/null
}

@test "clai surfaces API error messages from non-2xx responses" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  cat > "$TEST_HOME/fakebin/curl" <<'EOF'
#!/bin/bash
output=""
while [ $# -gt 0 ]; do
  case "$1" in
    --output)
      output="$2"
      shift 2
      ;;
    --write-out)
      shift 2
      ;;
    -d)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s' '{"error":{"message":"bad auth"}}' > "$output"
printf '401'
EOF
  chmod +x "$TEST_HOME/fakebin/curl"

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 1 ]
  [[ "$output" == *"bad auth"* ]]
}

@test "interactive mode persists history on exit" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_success_curl

  run bash -lc '
    printf "what is the current time?\nexit\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh
  '

  [ "$status" -eq 0 ]
  [ -f "$TEST_HOME/.local/state/clai/history_com.json" ]
  jq -e 'map(select(.role == "user" and .content == "what is the current time?")) | length >= 1' \
    "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e 'map(select(.role == "system")) | length == 0' \
    "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "command mode loads prior history into the next request payload" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  cat > "$TEST_HOME/.local/state/clai/history_com.json" <<'EOF'
[{"role":"user","content":"remember me"}]
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  jq -e '.messages | map(select(.role == "user" and .content == "remember me")) | length >= 1' \
    "$TEST_HOME/curl-request.json" >/dev/null
}

@test "history persistence respects max_history_turns trimming" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=2
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  cat > "$TEST_HOME/.local/state/clai/history_com.json" <<'EOF'
[{"role":"user","content":"one"},{"role":"assistant","content":"{\"info\":\"two\"}"},{"role":"user","content":"three"},{"role":"assistant","content":"{\"info\":\"four\"}"}]
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  [ "$(jq 'length' "$TEST_HOME/.local/state/clai/history_com.json")" -eq 4 ]
  jq -e '.[0].content == "three"' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e 'map(select(.content == "one")) | length == 0' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "invalid max_history_turns falls back safely during persistence" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=2 # invalid inline comment
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  cat > "$TEST_HOME/.local/state/clai/history_com.json" <<'EOF'
[{"role":"user","content":"one"},{"role":"assistant","content":"{\"info\":\"two\"}"}]
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  [[ "$output" != *"syntax error"* ]]
  [ "$(jq 'length' "$TEST_HOME/.local/state/clai/history_com.json")" -eq 2 ]
  jq -e '.[0].content == "what is the current time?"' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e 'map(select(.content == "one")) | length == 0' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "legacy max_history key still works with a deprecation warning" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history=2
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  cat > "$TEST_HOME/.local/state/clai/history_com.json" <<'EOF'
[{"role":"user","content":"one"},{"role":"assistant","content":"{\"info\":\"two\"}"},{"role":"user","content":"three"},{"role":"assistant","content":"{\"info\":\"four\"}"}]
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  [[ "$output" == *"deprecated"* ]]
  [ "$(jq 'length' "$TEST_HOME/.local/state/clai/history_com.json")" -eq 4 ]
}

@test "invalid history files warn and reset to empty history" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  printf '{not json' > "$TEST_HOME/.local/state/clai/history_com.json"

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  [[ "$output" == *"Could not parse history file"* ]]
  jq -e 'map(select(.role == "user" and .content == "what is the current time?")) | length == 1' \
    "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "invalid history files are repaired on immediate exit" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.local/state/clai"
  printf '{not json' > "$TEST_HOME/.local/state/clai/history_com.json"

  run bash -lc '
    printf "exit\n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      bash ./clai.sh
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"Could not parse history file"* ]]
  [ "$(cat "$TEST_HOME/.local/state/clai/history_com.json")" = "[]" ]
}

@test "--clear-history removes persisted history without requiring API config" {
  mkdir -p "$TEST_HOME/.local/state/clai"
  printf '[]' > "$TEST_HOME/.local/state/clai/history_com.json"
  printf '[]' > "$TEST_HOME/.local/state/clai/history_vim.json"

  make_marker_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh --clear-history

  [ "$status" -eq 0 ]
  [[ "$output" == *"Cleared CLAI history."* ]]
  [ ! -e "$TEST_HOME/.local/state/clai/history_com.json" ]
  [ ! -e "$TEST_HOME/.local/state/clai/history_vim.json" ]
  [ ! -e "$TEST_HOME/curl-called" ]
  [ ! -e "$TEST_HOME/.config/clai.cfg" ]
}

@test "clear your history request is handled locally without an API call" {
  mkdir -p "$TEST_HOME/.local/state/clai"
  printf '[]' > "$TEST_HOME/.local/state/clai/history_com.json"
  printf '[]' > "$TEST_HOME/.local/state/clai/history_vim.json"

  make_marker_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "clear your history"

  [ "$status" -eq 0 ]
  [[ "$output" == *"Cleared CLAI history."* ]]
  [ ! -e "$TEST_HOME/.local/state/clai/history_com.json" ]
  [ ! -e "$TEST_HOME/.local/state/clai/history_vim.json" ]
  [ ! -e "$TEST_HOME/curl-called" ]
  [ ! -e "$TEST_HOME/.config/clai.cfg" ]
}

@test "--clear-history returns an error when persisted history cannot be removed" {
  mkdir -p "$TEST_HOME/.local/state/clai"
  mkdir -p "$TEST_HOME/.local/state/clai/history_com.json"

  make_marker_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh --clear-history

  [ "$status" -eq 1 ]
  [[ "$output" == *"Failed to clear CLAI history."* ]]
  [ -d "$TEST_HOME/.local/state/clai/history_com.json" ]
  [ ! -e "$TEST_HOME/curl-called" ]
}

@test "tool calls trigger tool execution and resume with tool output in history" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.clai_tools"
  cat > "$TEST_HOME/.clai_tools/record-note.sh" <<'EOF'
#!/bin/bash
init() {
  echo '{
    "type": "function",
    "function": {
      "name": "record-note",
      "description": "Record a note for testing.",
      "parameters": {
        "type": "object",
        "properties": {
          "value": {
            "type": "string"
          }
        },
        "required": [
          "value"
        ]
      }
    }
  }'
}

execute() {
  echo "tool said: $(echo "$1" | jq -r '.value')"
}
EOF
  chmod +x "$TEST_HOME/.clai_tools/record-note.sh"

  make_tool_call_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "use a tool"

  [ "$status" -eq 0 ]
  [[ "$output" == *"tool flow complete"* ]]
  [ -f "$TEST_HOME/curl-request-2.json" ]
  jq -e '.messages | map(select(.role == "tool" and .content == "tool said: hello from tool")) | length >= 1' \
    "$TEST_HOME/curl-request-2.json" >/dev/null
  jq -e '.messages | map(select(.role == "assistant" and (.tool_calls | length >= 1))) | length >= 1' \
    "$TEST_HOME/curl-request-2.json" >/dev/null
}

@test "tool calls work with associative-array fallback enabled" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.clai_tools"
  cat > "$TEST_HOME/.clai_tools/record-note.sh" <<'EOF'
#!/bin/bash
init() {
  echo '{
    "type": "function",
    "function": {
      "name": "record-note",
      "description": "Record a note for testing.",
      "parameters": {
        "type": "object",
        "properties": {
          "value": {
            "type": "string"
          }
        },
        "required": [
          "value"
        ]
      }
    }
  }'
}

execute() {
  echo "tool said: $(echo "$1" | jq -r '.value')"
}
EOF
  chmod +x "$TEST_HOME/.clai_tools/record-note.sh"

  make_tool_call_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    CLAI_FORCE_NO_ASSOC_ARRAY=true \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "use a tool"

  [ "$status" -eq 0 ]
  [[ "$output" == *"tool flow complete"* ]]
  [ -f "$TEST_HOME/curl-request-2.json" ]
  jq -e '.messages | map(select(.role == "tool" and .content == "tool said: hello from tool")) | length >= 1' \
    "$TEST_HOME/curl-request-2.json" >/dev/null
}

@test "command responses can be accepted and executed" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_command_curl

  run bash -lc '
    printf "y" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  [ -f "$TEST_HOME/cmd-ran.txt" ]
  [ "$(cat "$TEST_HOME/cmd-ran.txt")" = "executed" ]
  [[ "$output" == *"run the stub command"* ]]
}

@test "command results can be stored in history with configured line limits" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
store_command_results=true
result_lines=2
exec_query=
question_query=
error_query=
EOF

  make_result_command_curl

  run bash -lc '
    printf "y" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  jq -e '
    map(select(.role == "assistant" and ((.content | fromjson? // {}) | has("command_result")))) | length == 1
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.stdout == "[truncated to last 2 lines]\nthree\nfour"
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.stderr == "[truncated to last 2 lines]\nerr-two\nerr-three"
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.exit_code == 0
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.edited == false
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "command results are not stored when disabled" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
store_command_results=false
result_lines=2
exec_query=
question_query=
error_query=
EOF

  make_result_command_curl

  run bash -lc '
    printf "y" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  [ -f "$TEST_HOME/.local/state/clai/history_com.json" ]
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | length == 0
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "run_cmd stores the real non-zero exit code in command results" {
  run bash -lc '
    HISTORY_MESSAGES="[]"
    HISTORY_DIRTY=false
    STORE_COMMAND_RESULTS=true
    RESULT_LINES=5
    SESSION_TMPDIR="'"$TEST_HOME"'/tmp"

    create_secure_temp() {
      local template="$1"
      local tmpfile
      tmpfile=$(mktemp "$template") || return 1
      chmod 600 "$tmpfile" 2>/dev/null || true
      printf "%s\n" "$tmpfile"
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
        '"'"'$history + [{
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
        }]'"'"')
      HISTORY_DIRTY=true
    }

    read_result_output_file() {
      local output_file="$1"
      local max_lines="$2"
      local line_count
      local trimmed_output

      line_count=$(awk "END { print NR }" "$output_file")
      if [ -z "$line_count" ] || [ "$line_count" -le 0 ]; then
        return 0
      fi
      if [ "$line_count" -le "$max_lines" ]; then
        cat "$output_file"
        return 0
      fi
      trimmed_output=$(tail -n "$max_lines" "$output_file")
      printf "[truncated to last %s lines]\n%s" "$max_lines" "$trimmed_output"
    }

    maybe_store_command_result() {
      local command="$1"
      local exit_code="$2"
      local stdout_file="$3"
      local stderr_file="$4"
      local edited="$5"
      local trimmed_stdout
      local trimmed_stderr

      if [ "$STORE_COMMAND_RESULTS" != true ]; then
        return 0
      fi

      trimmed_stdout=$(read_result_output_file "$stdout_file" "$RESULT_LINES")
      trimmed_stderr=$(read_result_output_file "$stderr_file" "$RESULT_LINES")
      append_command_result_message "$command" "$exit_code" "$trimmed_stdout" "$trimmed_stderr" "$edited"
    }

    print_ok() { :; }
    print_error() { :; }
    print_cancel() { :; }
    restore_cursor() { :; }

    run_cmd() {
      local command="$1"
      local edited="${2:-false}"
      local stdout_tmp
      local stderr_tmp
      local exit_status
      local output

      stdout_tmp=$(create_secure_temp "${SESSION_TMPDIR}/clai-command-stdout.XXXXXX.log") || return 1
      stderr_tmp=$(create_secure_temp "${SESSION_TMPDIR}/clai-command-stderr.XXXXXX.log") || {
        rm -f "$stdout_tmp"
        return 1
      }

      if eval "$command" > >(tee "$stdout_tmp") 2> >(tee "$stderr_tmp" >&2); then
        maybe_store_command_result "$command" 0 "$stdout_tmp" "$stderr_tmp" "$edited"
        rm -f "$stdout_tmp" "$stderr_tmp"
        return 0
      else
        exit_status=$?
        output=$(cat "$stderr_tmp")
        maybe_store_command_result "$command" "$exit_status" "$stdout_tmp" "$stderr_tmp" "$edited"
        LAST_ERROR="${output#*"$0": line *: }"
        rm -f "$stdout_tmp" "$stderr_tmp"
        return 1
      fi
    }

    run_cmd "bash -lc \"printf \\\"failure-output\\\\n\\\"; exit 42\"" false >/dev/null 2>&1 || true
    printf "%s" "$HISTORY_MESSAGES"
  '

  [ "$status" -eq 0 ]
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.exit_code == 42
  ' <<< "$output" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.edited == false
  ' <<< "$output" >/dev/null
}

@test "float result_lines is coerced to an integer safely" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
store_command_results=true
result_lines=2.5
exec_query=
question_query=
error_query=
EOF

  make_result_command_curl

  run bash -lc '
    printf "y" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  [[ "$output" != *"integer expression expected"* ]]
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.stdout == "[truncated to last 2 lines]\nthree\nfour"
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "command responses can be declined without execution" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_command_curl

  run bash -lc '
    printf "n" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  [ ! -e "$TEST_HOME/cmd-ran.txt" ]
  [[ "$output" == *"[cancel]"* ]]
}

@test "command responses can be edited before execution" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
store_command_results=true
result_lines=2
exec_query=
question_query=
error_query=
EOF

  make_command_curl

  run bash -lc '
    printf "e" | env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      CLAI_EDIT_COMMAND_OVERRIDE="printf edited > \"'"$TEST_HOME"'/cmd-edited.txt\"" \
      TEST_HOME="'"$TEST_HOME"'" \
      bash ./clai.sh "run the command"
  '

  [ "$status" -eq 0 ]
  [ ! -e "$TEST_HOME/cmd-ran.txt" ]
  [ -f "$TEST_HOME/cmd-edited.txt" ]
  [ "$(cat "$TEST_HOME/cmd-edited.txt")" = "edited" ]
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | length == 1
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.exit_code == 0
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
  jq -e '
    map(select(.role == "assistant"))
    | map(.content | fromjson? // empty)
    | map(select(has("command_result")))
    | .[0].command_result.edited == true
  ' "$TEST_HOME/.local/state/clai/history_com.json" >/dev/null
}

@test "command responses can be edited through a real PTY session" {
  if [ "${CLAI_ENABLE_PTY_TESTS:-false}" != "true" ]; then
    skip "PTY-backed edit test is disabled in this environment"
  fi

  if ! command -v script >/dev/null 2>&1; then
    skip "script command is unavailable"
  fi

  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_command_curl

  # Send "e" to enter the edit path; the replacement command comes from CLAI_EDIT_COMMAND_OVERRIDE.
  printf 'e' > "$TEST_HOME/edit-input.txt"

  run bash -lc '
    env \
      HOME="'"$TEST_HOME"'" \
      TMPDIR="'"$TEST_HOME"'/tmp" \
      PATH="'"$TEST_HOME"'/fakebin:$PATH" \
      USER="bats" \
      LANG="C" \
      LC_TIME="C" \
      CLAI_EDIT_COMMAND_OVERRIDE="printf edited-pty > \"'"$TEST_HOME"'/cmd-edited-pty.txt\"" \
      TEST_HOME="'"$TEST_HOME"'" \
      script -qec "bash ./clai.sh \"run the command\"" /dev/null < "'"$TEST_HOME"'/edit-input.txt"
  '

  [ "$status" -eq 0 ]
  [ ! -e "$TEST_HOME/cmd-ran.txt" ]
  [ -f "$TEST_HOME/cmd-edited-pty.txt" ]
  [ "$(cat "$TEST_HOME/cmd-edited-pty.txt")" = "edited-pty" ]
}

@test "missing assistant message content falls back to an unknown error" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_no_reply_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "show malformed response"

  [ "$status" -eq 0 ]
  [[ "$output" == *"An unknown error occurred."* ]]
}

@test "malformed tool call arguments do not crash the session" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  mkdir -p "$TEST_HOME/.clai_tools"
  cat > "$TEST_HOME/.clai_tools/record-note.sh" <<'EOF'
#!/bin/bash
init() {
  echo '{
    "type": "function",
    "function": {
      "name": "record-note",
      "description": "Record a note for testing.",
      "parameters": {
        "type": "object",
        "properties": {
          "value": {
            "type": "string"
          }
        },
        "required": [
          "value"
        ]
      }
    }
  }'
}

execute() {
  echo "tool said: $(echo "$1" | jq -r '.value')"
}
EOF
  chmod +x "$TEST_HOME/.clai_tools/record-note.sh"

  make_malformed_tool_call_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "use a tool"

  [ "$status" -eq 0 ]
  [[ "$output" == *"Using tool \"record-note\""* ]]
  [[ "$output" == *"tool fallback complete"* ]]
}

@test "invalid temp and tokens fall back in the request payload" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=not-a-number
tokens=also-bad
exec_query=
question_query=
error_query=
EOF

  make_success_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "what is the current time?"

  [ "$status" -eq 0 ]
  jq -e '.temperature == 0.1' "$TEST_HOME/curl-request.json" >/dev/null
  jq -e '.max_tokens == 500' "$TEST_HOME/curl-request.json" >/dev/null
}

@test "truncated JSON responses are repaired before display" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_truncated_json_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "show truncated response"

  [ "$status" -eq 0 ]
  [[ "$output" == *"trimmed response"* ]]
}

@test "non-JSON API success responses fail clearly" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://example.invalid/v1/chat/completions
model=gpt-4o-mini
json_mode=false
temp=0.1
tokens=500
exec_query=
question_query=
error_query=
EOF

  make_non_json_curl

  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    PATH="$TEST_HOME/fakebin:$PATH" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    TEST_HOME="$TEST_HOME" \
    bash ./clai.sh "show malformed response"

  [ "$status" -eq 1 ]
  [[ "$output" == *"non-JSON response"* ]]
}
