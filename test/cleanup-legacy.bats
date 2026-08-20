#!/usr/bin/env bats

setup() {
  export TEST_ROOT
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/clai-cleanup.XXXXXX")"
  export ACTIVE_CLAI="$TEST_ROOT/bin/clai"
  export LEGACY_INSTALL_DIR="$TEST_ROOT/usr-local-lib-clai"
  export LEGACY_TOOLS_DIR="$TEST_ROOT/home/.clai_tools"
  export CONFIG_PATH="$TEST_ROOT/home/.config/clai.cfg"
  export HISTORY_PATH="$TEST_ROOT/home/.local/state/clai/history_com.json"
  export MCP_TOOLS_DIR="$TEST_ROOT/home/.config/clai/tools.d"

  mkdir -p "$(dirname "$ACTIVE_CLAI")" "$LEGACY_INSTALL_DIR" "$LEGACY_TOOLS_DIR"
  mkdir -p "$(dirname "$CONFIG_PATH")" "$(dirname "$HISTORY_PATH")" "$MCP_TOOLS_DIR"
  go build -buildvcs=false -trimpath -o "$ACTIVE_CLAI" ./cmd/clai
  printf 'key=preserved\n' > "$CONFIG_PATH"
  printf '[]\n' > "$HISTORY_PATH"
  printf 'new MCP tool\n' > "$MCP_TOOLS_DIR/example"
}

teardown() {
  rm -rf "$TEST_ROOT"
}

fixture_hash() {
  sha256sum "$1" | awk '{print $1}'
}

run_cleanup() {
  stock_script_sha=b1ca2b12f2e83291e32b14a618c863387040299e97ca18c6c32ed02fd2147778
  stock_cat_sha=ef4853963ed4d0fceb8886e92cd2d1279d8f220197b23a0b6760d77cf283cb85
  test_script="$TEST_ROOT/cleanup-legacy.sh"
  sed \
    -e "s/$stock_script_sha/${LEGACY_SCRIPT_SHA:-$stock_script_sha}/" \
    -e "s/$stock_cat_sha/${LEGACY_CAT_SHA:-$stock_cat_sha}/" \
    ./scripts/cleanup-legacy.sh > "$test_script"
  run env \
    HOME="$TEST_ROOT/home" \
    CLAI_ACTIVE_PATH="$ACTIVE_CLAI" \
    CLAI_LEGACY_INSTALL_DIR="$LEGACY_INSTALL_DIR" \
    CLAI_LEGACY_TOOLS_DIR="$LEGACY_TOOLS_DIR" \
    CLAI_CONFIG="$CONFIG_PATH" \
    CLAI_HISTORY="$HISTORY_PATH" \
    CLAI_TOOLS_DIR="$MCP_TOOLS_DIR" \
    sh "$test_script" "$@"
}

@test "legacy cleanup dry-run lists but does not remove recognized files" {
  printf 'stock legacy script\n' > "$LEGACY_INSTALL_DIR/clai.sh"
  LEGACY_SCRIPT_SHA=$(fixture_hash "$LEGACY_INSTALL_DIR/clai.sh")

  run_cleanup

  [ "$status" -eq 0 ]
  [ -f "$LEGACY_INSTALL_DIR/clai.sh" ]
  [[ "$output" == *"Would remove Bash payload"* ]]
  [[ "$output" == *"Dry run only"* ]]
}

@test "legacy cleanup removes only exact stock files and preserves reused data" {
  printf 'stock legacy script\n' > "$LEGACY_INSTALL_DIR/clai.sh"
  printf 'stock cat tool\n' > "$LEGACY_TOOLS_DIR/cat.sh"
  printf 'modified stock-name tool\n' > "$LEGACY_TOOLS_DIR/ls.sh"
  printf 'custom tool\n' > "$LEGACY_TOOLS_DIR/custom.sh"
  LEGACY_SCRIPT_SHA=$(fixture_hash "$LEGACY_INSTALL_DIR/clai.sh")
  LEGACY_CAT_SHA=$(fixture_hash "$LEGACY_TOOLS_DIR/cat.sh")

  run_cleanup --apply

  [ "$status" -eq 0 ]
  [ ! -e "$LEGACY_INSTALL_DIR/clai.sh" ]
  [ ! -d "$LEGACY_INSTALL_DIR" ]
  [ ! -e "$LEGACY_TOOLS_DIR/cat.sh" ]
  [ -f "$LEGACY_TOOLS_DIR/ls.sh" ]
  [ -f "$LEGACY_TOOLS_DIR/custom.sh" ]
  [ -f "$CONFIG_PATH" ]
  [ -f "$HISTORY_PATH" ]
  [ -f "$MCP_TOOLS_DIR/example" ]
  [ -x "$ACTIVE_CLAI" ]
  [[ "$output" == *"Preserving modified or unrecognized stock ls tool"* ]]
  [[ "$output" == *"Removed 2 recognized legacy file(s)"* ]]
}

@test "legacy cleanup refuses to remove the active command target" {
	  mv "$ACTIVE_CLAI" "$LEGACY_INSTALL_DIR/clai.sh"
	  ln -s "$LEGACY_INSTALL_DIR/clai.sh" "$ACTIVE_CLAI"

  run_cleanup --apply

  [ "$status" -eq 1 ]
  [ -f "$LEGACY_INSTALL_DIR/clai.sh" ]
  [[ "$output" == *"active clai still resolves to the legacy payload"* ]]
}

@test "legacy cleanup refuses a native executable that is not CLAI" {
  cp "$(type -P true)" "$ACTIVE_CLAI"

  run_cleanup --apply

  [ "$status" -eq 1 ]
	  [[ "$output" == *"does not identify itself as CLAI with --version"* ]]
}
