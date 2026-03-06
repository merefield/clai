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

  run env REPO_ROOT="$PWD" bash -c '
    source "$REPO_ROOT/tools/find-wild.sh"
    execute "{\"path\":\"'"$search_dir"'\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"$search_dir/hello.txt"* ]]
}

@test "find-wildcard returns not found for missing directories" {
  run env REPO_ROOT="$PWD" bash -c '
    source "$REPO_ROOT/tools/find-wild.sh"
    execute "{\"path\":\"'"$TEST_ROOT"'/missing\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [ "$output" = "Not found" ]
}

@test "find-wildcard expands leading tilde paths" {
  mkdir -p "$TEST_ROOT/tilde-dir"
  touch "$TEST_ROOT/tilde-dir/hello.txt"

  run env HOME="$TEST_ROOT" REPO_ROOT="$PWD" bash -c '
    source "$REPO_ROOT/tools/find-wild.sh"
    execute "{\"path\":\"~/tilde-dir\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"$TEST_ROOT/tilde-dir/hello.txt"* ]]
}

@test "find-wildcard expands a bare tilde path to HOME" {
  touch "$TEST_ROOT/home-file.txt"

  run env HOME="$TEST_ROOT" REPO_ROOT="$PWD" bash -c '
    source "$REPO_ROOT/tools/find-wild.sh"
    execute "{\"path\":\"~\",\"name\":\"home-file\"}"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"$TEST_ROOT/home-file.txt"* ]]
}

@test "find-wildcard handles dash-prefixed relative directories safely" {
  mkdir -p "$TEST_ROOT/-dashdir"
  touch "$TEST_ROOT/-dashdir/hello.txt"

  run env TEST_ROOT="$TEST_ROOT" REPO_ROOT="$PWD" bash -c '
    cd "$TEST_ROOT" || exit 1
    source "$REPO_ROOT/tools/find-wild.sh"
    execute "{\"path\":\"-dashdir\",\"name\":\"hello\"}"
  '

  [ "$status" -eq 0 ]
  [[ "$output" == *"./-dashdir/hello.txt"* ]]
}
