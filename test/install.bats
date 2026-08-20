#!/usr/bin/env bats

setup() {
  export TEST_ROOT
  TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/clai-install.XXXXXX")"
  mkdir -p "$TEST_ROOT/fakebin"
  mkdir -p "$TEST_ROOT/bin"
}

teardown() {
  rm -rf "$TEST_ROOT"
}

write_fake_go_builder() {
  cat > "$TEST_ROOT/fakebin/go" <<'EOF'
#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output=$2
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[ -n "$output" ] || exit 2
printf '%s\n' '#!/bin/sh' 'echo installed clai' > "$output"
chmod +x "$output"
EOF
  chmod +x "$TEST_ROOT/fakebin/go"
}

@test "Go installer reports a failed build" {
  cat > "$TEST_ROOT/fakebin/go" <<'EOF'
#!/bin/sh
echo "fake build failure" >&2
exit 1
EOF
  chmod +x "$TEST_ROOT/fakebin/go"

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    sh ./install.sh

  [ "$status" -eq 1 ]
  [[ "$output" == *"Building CLAI"* ]]
  [[ "$output" == *"fake build failure"* ]]
  [ ! -e "$TEST_ROOT/bin/clai" ]
}

@test "Go installer builds and installs clai into an overridden bin directory" {
  write_fake_go_builder

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    sh ./install.sh

  [ "$status" -eq 0 ]
  [ -x "$TEST_ROOT/bin/clai" ]
  [[ "$output" == *"Installed clai to $TEST_ROOT/bin/clai"* ]]

  run "$TEST_ROOT/bin/clai"
  [ "$status" -eq 0 ]
  [ "$output" = "installed clai" ]
}

@test "Go installer honours a temporary binary name override" {
  write_fake_go_builder

  run env \
    PATH="$TEST_ROOT/fakebin:$PATH" \
    CLAI_BIN_DIR="$TEST_ROOT/bin" \
    CLAI_BIN_NAME="clai-go-preview" \
    sh ./install.sh

  [ "$status" -eq 0 ]
  [ -x "$TEST_ROOT/bin/clai-go-preview" ]
  [ ! -e "$TEST_ROOT/bin/clai" ]
}
