#!/usr/bin/env bats

setup() {
  export TEST_ROOT
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/clai-find-wild.XXXXXX")"
}

teardown() {
  rm -rf "$TEST_ROOT"
}

@test "find-wildcard finds matches in directories with spaces" {
  local search_dir="$TEST_ROOT/dir with spaces"
  mkdir -p "$search_dir"
  touch "$search_dir/hello.txt"

  run bash -lc '
    source ./tools/find-wild.sh
    execute "{\"path\":\"'"$search_dir"'\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"$search_dir/hello.txt"* ]]
}

@test "find-wildcard returns not found for missing directories" {
  run bash -lc '
    source ./tools/find-wild.sh
    execute "{\"path\":\"'"$TEST_ROOT"'/missing\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [ "$output" = "Not found" ]
}
