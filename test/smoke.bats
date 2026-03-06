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

@test "clai bootstraps config and asks for API key when missing" {
  run env \
    HOME="$TEST_HOME" \
    TMPDIR="$TEST_HOME/tmp" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    bash ./clai.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"please input your OpenAI key"* ]]
  [ -f "$TEST_HOME/.config/clai.cfg" ]
  [ -d "$TEST_HOME/.local/state/clai" ]
  grep -qx 'key=' "$TEST_HOME/.config/clai.cfg"
  [ "$(stat -c '%a' "$TEST_HOME/.config/clai.cfg")" = "600" ]
  [ "$(stat -c '%a' "$TEST_HOME/.local/state/clai")" = "700" ]
  [ "$(find "$TEST_HOME/tmp" -type f | wc -l)" -eq 0 ]
}

@test "clai cleans transient session files after API transport failure" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history=10
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
max_history=10
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
  [[ "$output" == *"stub answer"* ]]
  jq -e '.messages | length > 0' "$TEST_HOME/curl-request.json" >/dev/null
  jq -e '.response_format.type == "json_object"' "$TEST_HOME/curl-request.json" >/dev/null
}

@test "clai surfaces API error messages from non-2xx responses" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history=10
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
max_history=10
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
}

@test "command mode loads prior history into the next request payload" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history=10
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

@test "history persistence respects max_history trimming" {
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
  [ "$(jq 'length' "$TEST_HOME/.local/state/clai/history_com.json")" -eq 2 ]
}

@test "invalid max_history falls back safely during persistence" {
  write_config <<'EOF'
key=test-key
hi_contrast=false
expose_current_dir=true
max_history=2 # invalid inline comment
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
  [ "$(jq 'length' "$TEST_HOME/.local/state/clai/history_com.json")" -eq 1 ]
}
