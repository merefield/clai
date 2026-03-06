#!/usr/bin/env bats

setup() {
  export TEST_HOME
  TEST_HOME="$(mktemp -d "${TMPDIR:-/tmp}/clai-test.XXXXXX")"
  mkdir -p "$TEST_HOME/.config"
  mkdir -p "$TEST_HOME/tmp"
}

teardown() {
  rm -rf "$TEST_HOME"
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
  [ "$(find "$TEST_HOME/tmp" -type f | wc -l)" -eq 0 ]
}

@test "clai cleans transient session files after API transport failure" {
  printf '%s\n' \
    'key=test-key' \
    'hi_contrast=false' \
    'expose_current_dir=true' \
    'max_history=10' \
    'api=http://127.0.0.1:1' \
    'model=gpt-4o-mini' \
    'json_mode=false' \
    'temp=0.1' \
    'tokens=500' \
    'exec_query=' \
    'question_query=' \
    'error_query=' > "$TEST_HOME/.config/clai.cfg"

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
