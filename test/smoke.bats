#!/usr/bin/env bats

setup() {
  export TEST_HOME
  TEST_HOME="$(mktemp -d "${TMPDIR:-/tmp}/clai-test.XXXXXX")"
  mkdir -p "$TEST_HOME/.config"
}

teardown() {
  rm -rf "$TEST_HOME"
}

@test "clai bootstraps config and asks for API key when missing" {
  run env \
    HOME="$TEST_HOME" \
    USER="bats" \
    LANG="C" \
    LC_TIME="C" \
    bash ./clai.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"please input your OpenAI key"* ]]
  [ -f "$TEST_HOME/.config/clai.cfg" ]
  grep -qx 'key=' "$TEST_HOME/.config/clai.cfg"
}
