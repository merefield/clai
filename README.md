# CLAI

[![CI](https://img.shields.io/github/actions/workflow/status/merefield/clai/ci.yml?branch=main&label=CI&logo=github&style=flat-square)](https://github.com/merefield/clai/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/merefield/clai?display_name=tag&sort=semver&style=flat-square&cacheSeconds=300)](https://github.com/merefield/clai/releases/latest)
[![Go 1.27.3](https://img.shields.io/badge/Go-1.27.3-00ADD8?logo=go&logoColor=white&style=flat-square)](go.mod)
[![License](https://img.shields.io/github/license/merefield/clai?style=flat-square&cacheSeconds=300)](LICENSE.txt)

CLAI is an AI-powered terminal assistant that turns natural-language requests into shell commands you can inspect, edit, approve, or reject before they run.

```text
clai list files by size
clai how much is 3 times pi
```

The primary implementation is now Go. It keeps the existing `clai <request...>` interface, configuration and history locations, and risk controls while moving API clients, orchestration, persistence, terminal UI, and LLM tools into typed Go packages.

> [!NOTE]
> This release migrates CLAI's runtime from the former Bash script to a compiled Go binary. Existing configuration, history, command syntax, and shell integration continue to be used; legacy `~/.clai_tools` Bash plugins do not. See [Migration from the Bash version](#migration-from-the-bash-version) before removing old files.

![CLAI command suggestion and confirmation flow](docs/assets/examples.png)

## Features

- Direct prompts without wrapping the whole request in quotes
- Interactive sessions when `clai` is run without a request
- OpenAI, OpenAI-compatible, Ollama, Anthropic, and Gemini API adapters
- Structured command, explanation, risk, and missing-variable responses
- Green, amber, and red command risk levels
- Review, edit, approval, and additional danger-zone confirmation
- Live command stdout and stderr
- Optional error analysis after a command fails
- Persistent conversation history with configurable retention
- Opt-in command-result sharing with output truncation
- Hot-loaded MCP tool servers exposed through provider function calling
- A standalone Go Wikipedia server that doubles as a copyable plugin tutorial
- Shell integration for unquoted `*` and `?` in Bash and zsh
- Layered `AGENTS.md` guidance and portable Agent Skills with local selection and on-demand documentation

## Requirements

To run a compiled CLAI binary:

- Linux, macOS, or Windows on AMD64 or ARM64
- Bash on `PATH`, used to execute approved commands (Git Bash is suitable on Windows)
- An API key, provider credential, or non-empty placeholder required by a local service configuration

The Go binary implements HTTP, JSON, and tool handling itself. It does not require `curl` or `jq`.

Building from source requires the Go toolchain declared by [`go.mod`](go.mod): Go 1.27.3 for this branch.

## Quickstart

The recommended installer downloads the pre-built release for the current operating system and architecture, verifies it against the release SHA-256 checksum, checks the binary's reported version, and installs it as `/usr/local/bin/clai`:

```bash
curl -fsSL https://raw.githubusercontent.com/merefield/clai/main/install-release.sh | sh
```

The bootstrap requires `curl` or `wget`, `tar`, and either `sha256sum` (Linux) or `shasum` (macOS). It uses `sudo` only when the destination is not writable.

For a user-local installation that does not require `sudo`:

```bash
curl -fsSL https://raw.githubusercontent.com/merefield/clai/main/install-release.sh |
  CLAI_BIN_DIR="$HOME/.local/bin" sh
```

Ensure `$HOME/.local/bin` is on `PATH` if you use that location. To install a particular release reproducibly, replace `vX.Y.Z` with a tag from [Releases](https://github.com/merefield/clai/releases):

```bash
release_tag=vX.Y.Z
curl -fsSL https://raw.githubusercontent.com/merefield/clai/main/install-release.sh |
  sh -s -- --version "$release_tag"
```

On Windows, download and inspect the PowerShell installer, then run it for the current process without changing the machine-wide execution policy:

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/merefield/clai/main/install-release.ps1 -OutFile install-release.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\install-release.ps1
```

The Unix installer supports `--help`, `--version TAG`, and `--bin-dir DIR`; the PowerShell installer accepts `-Version`, `-BinDir`, and `-Repository`. Both also support the corresponding `CLAI_*` environment variables.

The Windows installer defaults to `%LOCALAPPDATA%\Programs\clai\bin` and reports when that directory must be added to `PATH`. CLAI still invokes Bash for approved commands, so install Git Bash or another Windows Bash and ensure `bash.exe` is on `PATH`.

After installation, configure a provider and try a request:

```bash
clai setup
clai list files by size
```

Setup also offers an optional command name for voice input, such as `computer`. Press Enter to skip it. On Bash, zsh, and PowerShell, setup checks for existing commands, aliases, functions, and shell keywords, then asks before saving shell integration to your profile. See [Calling CLAI by another name](#calling-clai-by-another-name).

## Install from source

Clone the repository and run the installer from its root:

```bash
git clone https://github.com/merefield/clai.git
cd clai
./install.sh
```

The installer builds `./cmd/clai` in a temporary directory and installs the result as `/usr/local/bin/clai`. It uses `sudo` only when the destination is not writable.

For a user-local installation:

```bash
CLAI_BIN_DIR="$HOME/.local/bin" ./install.sh
```

Ensure `$HOME/.local/bin` is on `PATH` if you use that location.

For development, build and run without installing:

```bash
make build
./clai --version
./clai setup
./clai list files by size
```

`make install` is also available and honours `DESTDIR` and `PREFIX`:

```bash
make install PREFIX="$HOME/.local"
```

Release archives are configured through [GoReleaser](.goreleaser.yaml) for Linux, macOS, and Windows on AMD64 and ARM64. Release binaries report the release tag as their version. Local builds read the maintained version from [`internal/app/VERSION`](internal/app/VERSION); the release workflow refuses a tag that does not match it.

## First run and setup

Run the setup wizard explicitly:

```bash
clai setup
```

`clai --setup` is an equivalent compatibility form. If no API key is configured, an ordinary invocation starts setup automatically.

The wizard asks for:

- API key or local-service token
- API endpoint
- model
- risk appetite

The defaults are:

```ini
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
risk_appetite=0
use_tools=false
```

Configuration is stored in `~/.config/clai.cfg` with mode `0600`. The API key is stored in that plain-text local file, so protect the account and filesystem that contain it.

## Usage

Pass a request as separate shell arguments:

```bash
clai show the ten largest files under this directory
```

Run `clai` without arguments for an interactive session. Enter `exit` to leave it.

A request containing `?` uses question mode and returns an explanation without proposing a command:

```bash
clai "how do I show hidden files?"
```

### Built-in commands

| Command | Purpose |
| --- | --- |
| `clai setup` | Run the configuration wizard. |
| `clai --setup` | Compatibility alias for `setup`. |
| `clai --question <question>` | Answer-only mode using `question_query`, with or without System One; proposed shell commands are not executed. |
| `clai --show-history` | Render persisted conversation history. |
| `clai --show-history --verbose` | Include full stored command stdout and stderr. |
| `clai --clear-history` | Remove persisted conversation history. |
| `clai --show-results-sharing` | Report whether command results are shared, interpreted, and stored. |
| `clai --toggle-results-sharing` | Enable or disable command-result sharing and interpretation. |
| `clai shell-init bash` | Print Bash integration for literal glob characters. |
| `clai shell-init zsh` | Print zsh integration for literal glob characters. |
| `clai shell-init pwsh` | Print PowerShell integration (`powershell` is also accepted). |
| `clai --version` | Print the CLAI version. |
| `clai --help` | Print command help. |

Natural-language requests such as `clai clear your history` are recognized locally for common history-clearing phrases and do not call the model.

## Shell parsing and unquoted prompts

CLAI joins all request arguments with spaces, so ordinary prompts do not need surrounding quotes:

```bash
clai create a directory named reports
```

Your parent shell still interprets syntax such as `*`, `?`, `$`, quotes, redirections, and command substitutions before CLAI starts. Without shell integration, reword, escape, or quote those characters:

```bash
clai how much is 3 times pi
clai how much is 3 \* pi
clai "how much is 3 * pi"
```

To support the exact unquoted form, add the appropriate integration to your shell profile:

```bash
# ~/.bashrc
eval "$(clai shell-init bash)"
```

```zsh
# ~/.zshrc
eval "$(clai shell-init zsh)"
```

Start a new shell or reload its profile. Glob characters following `clai` will then be passed literally:

```bash
clai how much is 3 * pi
```

The integration affects only the `clai` invocation and restores Bash globbing afterwards.

### Calling CLAI by another name

Run `clai setup` and enter `computer` (or another lowercase command name) at the optional command-alias prompt. A familiar word can be easier to dictate than `clai`; test your choice with your voice recognizer, including whether it adds capitalization or punctuation.

Setup checks for conflicts in a fresh shell and asks before updating your shell profile:

- **Bash:** `~/.bashrc` on Linux, or the first existing login profile (`~/.bash_profile`, `~/.bash_login`, or `~/.profile`) on macOS, creating `~/.bash_profile` if needed.
- **zsh:** `${ZDOTDIR:-$HOME}/.zshrc`, including `ZDOTDIR` set in `.zshenv`.
- **PowerShell 7 or Windows PowerShell 5.1:** the shell's reported `$PROFILE.CurrentUserAllHosts`, so redirected Documents folders and the separate PowerShell editions use the correct location. Setup saves a native alias directly to the executable, preserving PowerShell's normal argument handling and native exit status. Existing UTF-8/UTF-16 profile encodings and line endings are retained; UTF-8 profiles receive a byte-order mark for Windows PowerShell compatibility. Your execution policy must permit loading the profile; setup does not change it.

It saves one managed block and checks for conflicts when the profile loads. Bash/zsh integration preserves unquoted `*` and `?` arguments. PowerShell retains native command parsing: Windows passes those characters literally; on macOS/Linux, quote wildcard characters as you would when calling `clai` directly (for example, `computer how much is 3 '*' pi`).

Setup detects a PowerShell parent process on Windows, macOS, and Linux; otherwise it uses the login shell (or an available PowerShell edition on Windows). Set `CLAI_SHELL` to `bash`, `zsh`, `pwsh`, `powershell`, or the shell executable's path to select it explicitly. Other shells are left unchanged.

Start a new shell or run the activation command printed by setup (`source 'profile-path'` in Bash/zsh, `. 'profile-path'` in PowerShell), then use the new name:

```bash
computer show the ten largest files
computer how much is 3 * pi
```

Running setup again lets you keep, replace, or remove the managed alias. Enter keeps the current alias; `-` removes it after confirmation. Start a new shell after replacing or removing an alias to clear its previous definition.

The saved alias invokes the installed executable by its absolute path. Run setup again if you move the executable. Both names use the same configuration and history.

You can also configure an alias manually. For example, add the following to `~/.bashrc` to use `ask`:

```bash
eval "$(clai shell-init bash)"
alias ask='clai'
```

For zsh, add this to `~/.zshrc` instead:

```zsh
eval "$(clai shell-init zsh)"
alias ask='clai'
```

If shell initialization is already configured, add only the alias after it. This order preserves CLAI's handling of unquoted `*` and `?` through the alias.

Start a new shell or reload your profile with `source ~/.bashrc` (Bash) or `source ~/.zshrc` (zsh), then pass requests as usual:

```bash
ask show the ten largest files
ask how much is 3 * pi
```

The alias forwards the same command-line arguments to CLAI. Both command names use the same configuration and history.

## Command safety workflow

The provider returns a structured response containing:

- `cmd`: the proposed Bash command, or empty for an informational answer
- `info`: a short explanation
- `risk`: `none`, `reversible change`, or `danger zone`
- `variables`: values CLAI must collect before showing the final command

If a value is missing, CLAI prompts for it and shell-quotes it before replacing its `{{variable_name}}` placeholder. CLAI refuses to execute a response that still contains unresolved placeholders.

Suggested commands are displayed by risk:

| Risk | Colour | Meaning |
| --- | --- | --- |
| `none` | Green | Read-only inspection. |
| `reversible change` | Amber | A change that is normally undoable. |
| `danger zone` | Red | Deletion, overwrite, reset, force, or another hard-to-reverse action. |

When confirmation is required, CLAI prompts with `execute command? [y/e/N]:`:

- `y` runs the command.
- `e` lets you replace the proposed command before it runs.
- Enter or any other answer cancels.

`risk_appetite` controls automatic execution after the command and explanation have been displayed:

| Value | Behaviour |
| --- | --- |
| `0` | Confirm every proposed command. |
| `1` | Automatically run green commands; confirm amber and red commands. |
| `2` | Automatically run green and amber commands; confirm red commands. |

Danger-zone commands always receive the normal confirmation. When `confirm_dangerous_commands=true`, accepting one produces a second `danger zone command, are you sure? [y/N]:` prompt.

Approved commands run through `bash -o errexit -o pipefail -c`. Stdout and stderr stream to the terminal without truncation, while CLAI retains only the most recent 256 KiB of each stream. Cancellation terminates the command's process group, and detached descendants cannot keep CLAI blocked on inherited output pipes. After a failed command in an interactive terminal, CLAI can send the command and its stderr back to the provider to request an explanation and possible repair.

Approved commands inherit terminal input, so prompts from tools such as `apt-get` can be answered normally after CLAI's own confirmations. On Unix, CLAI hands foreground terminal ownership to the command and restores it and the terminal settings afterward, including on cancellation. Redirected file input is also passed through; when no input file is available, commands receive EOF. Output still streams and is captured as described above; this is not a full pseudo-terminal wrapper for screen-oriented applications.

Risk is model-generated guidance, not a security boundary. Read every proposed or edited command before approving it.

If `system_one_key`, `system_one_api`, and `system_one_model` are configured, CLAI uses that System One-compatible API for typed intent routing and command-risk auditing. The main LLM still generates commands and explanations, but System One decides whether a request is a command task, question, or history-clear request, and independently audits proposed command risk before `risk_appetite` can auto-run it. A higher System One risk label overrides the LLM risk label; low-confidence risk audits force a confirmation prompt.

Requests are interpreted by meaning rather than question marks. Questions needing live data, such as `what is the time?` or `how much disk space is free?`, should produce a command to obtain the answer and follow the usual risk and confirmation rules. General knowledge and how-to questions can be answered directly without execution. With `share_command_results=true`, CLAI also interprets the resulting command output. These decisions use System One when configured, or the main LLM otherwise.

## Providers

CLAI selects its native adapter from the configured `api` URL:

| Provider | Typical endpoint | Authentication and behavior |
| --- | --- | --- |
| OpenAI Chat Completions | `https://api.openai.com/v1/chat/completions` | Bearer token; supports native JSON Schema and local tool calls. |
| OpenAI Responses | `https://api.openai.com/v1/responses` | Bearer token; supports native JSON Schema, reasoning effort, and local tool calls through the Responses API. |
| OpenAI-compatible | Provider-specific Chat Completions URL | Bearer token; uses the OpenAI-compatible message and tool format. |
| Ollama | `http://localhost:11434/api/chat` | Uses the configured model, disables streaming, and supports Ollama tool-call responses. Keep `key` non-empty even if the service ignores it. |
| Anthropic | `https://api.anthropic.com/v1/messages` | Uses `x-api-key` and the Messages payload. Local CLAI tools are not sent. |
| Gemini | A `generativelanguage.googleapis.com` `generateContent` URL | Uses `x-goog-api-key`; the configured model replaces the model segment in the request URL. Local CLAI tools are not sent. |

Provider detection is URL-based. Use `/v1/responses` for OpenAI models that require or work best with the Responses API. Other URLs use the generic OpenAI-compatible adapter, so use a Chat Completions-compatible endpoint and model unless the provider explicitly implements the Responses request and response shape.

When `json_mode=true`, CLAI requests provider-enforced structured JSON:

- OpenAI Chat Completions uses JSON Schema through `response_format`.
- OpenAI Responses uses JSON Schema through `text.format`.
- Generic OpenAI-compatible endpoints request a JSON object.
- Ollama sends `format: "json"`.
- Anthropic uses structured output configuration.
- Gemini uses a JSON response schema.

For OpenAI Responses, `reasoning` is sent as `reasoning.effort` only for known reasoning-model families; CLAI also omits `temperature` for those reasoning models because they do not accept sampling parameters. For OpenAI Chat Completions, `reasoning` is sent as `reasoning_effort` only for known reasoning-model families; generic compatible completion endpoints receive the configured value directly. For Ollama, any non-empty `reasoning` value enables `think: true` and also requests JSON output.

Example OpenAI Responses configuration:

```ini
key=sk-...
api=https://api.openai.com/v1/responses
model=gpt-5.2
json_mode=true
reasoning=medium
```

Example Ollama configuration:

```ini
key=ollama
api=http://localhost:11434/api/chat
model=gemma4:latest
json_mode=true
reasoning=true
```

Example System One configuration for TypeSafe Jev:

```ini
system_one_key=ts-...
system_one_api=https://api.typesafe.ai/v1/systemone
system_one_model=jev-latest
```

## Local context quickstart

This walkthrough uses the portable OpenAI CLI example. Install a CLAI build containing this feature first; `clai skills --help` should display the local context commands. Provider setup is only needed when making a model request, not for inspecting guidance or skills.

1. **Add project preferences.** Create `AGENTS.md` at your repository root, for example:

   ```markdown
   # Project guidance
   Prefer concise explanations and show commands before running them.
   Use the project's documented development and test commands.
   ```

   Add more specific guidance in a nested directory when needed. Invoke CLAI from that directory to apply its instructions. Use `clai context show` to check the chain before making a request.

2. **Choose a skill scope.** Use `~/.agents/skills/` for skills shared across projects, or `<project>/.agents/skills/` for a project's skills. A nested project's `.agents/skills/` applies only when invoking CLAI from that directory or below it. Existing skills in these locations are discovered automatically.

3. **Install a whole skill folder.** For a first installation of the bundled example, run from a CLAI source checkout with no existing `openai-cli` destination folder:

   Bash/zsh:

   ```bash
   mkdir -p "$HOME/.agents/skills"
   cp -R examples/skills/openai-cli "$HOME/.agents/skills/"
   ```

   PowerShell:

   ```powershell
   $skillRoot = Join-Path $HOME '.agents/skills'
   New-Item -ItemType Directory -Path $skillRoot -Force | Out-Null
   Copy-Item -LiteralPath ./examples/skills/openai-cli -Destination $skillRoot -Recurse
   ```

   Copy the whole folder, including `references/`, rather than just `SKILL.md`. To keep the skill project-local, use the project's `.agents/skills/` as the destination instead. No CLAI-specific package installation is needed.

4. **Inspect before using it.** These commands do not call a provider:

   ```bash
   clai skills list
   clai skills show openai-cli
   clai skills show openai-cli --body
   clai skills list --all
   clai context show
   ```

   `show` prints the selected source path. Use `clai skills validate <that-path>` to check an installed skill, or `clai skills validate examples/skills/openai-cli` to check the bundled source. `list --all` explains invalid or shadowed copies. `--body` and `context show --verbose` deliberately display document contents.

5. **Make a request.** After `clai setup` if needed, try either local automatic matching or explicit selection:

   ```bash
   clai list the OpenAI API models I have access to
   clai --skill openai-cli list my available OpenAI models
   clai --question --skill openai-cli explain how to list models
   ```

   An activation line such as `skill: openai-cli (local relevance match)` identifies the skill in use. CLAI can then propose `openai models list` through its normal review flow. The separate `openai` executable and its credentials must already be configured to execute that command. A request such as `clai list the files in this directory` does not activate this skill.

6. **Use or change skills interactively.** Start `clai`, then enter:

   ```text
   /skills list
   /skills use openai-cli
   list my available OpenAI models
   /skills off
   /context show
   exit
   ```

   Explicit selections last for that session. `/skills off` clears them and disables automatic activation for the rest of the session, until `/skills use` selects a skill again. Edit files and make another request to pick up changes; `/skills reload` also performs a fresh scan. Set `skills_auto_activate=false` for explicit-only selection, `skills_enabled=false` to disable skills, or `agents_enabled=false` to disable AGENTS guidance.

If a skill is missing, inspect `clai skills list --all`, its filename/frontmatter, the current working directory and the search limits below. If a skill is listed but not selected automatically, improve its description or use `--skill`. If references are unavailable, the main instructions still work; check the provider's tool-calling support and the documentation path rules. External MCP does not need to be enabled for skill references.

## AGENTS.md guidance

CLAI discovers [AGENTS.md](https://agents.md/) instructions locally on every request:

1. `<os.UserConfigDir()>/clai/AGENTS.md` for global preferences (`~/.config/clai/AGENTS.md` on typical Linux systems).
2. The nearest Git project root, recognized by either a `.git` directory or a worktree `.git` file.
3. Each directory from that root down to the invocation's current directory.

Without a Git root, only the current directory supplies project guidance. Siblings, descendants and ancestors above the root are not searched. In each applicable directory, the first non-empty valid `AGENTS.override.md` or `AGENTS.md` is used, in that order. Closer project documents take precedence within the guidance layer; skills supplement those instructions. The user's request remains the task, and CLAI's built-in JSON contract, tool restrictions and approval rules retain priority.

Inspect the chain without a provider key:

```bash
clai context show
clai context show --verbose
```

Default inspection shows paths, scopes, byte counts and diagnostics. `--verbose` displays file contents. UTF-8 files exceeding a per-file or aggregate budget are omitted with a warning. Existing documents are refreshed on the next request, including in interactive sessions.

## Agent Skills

CLAI reads the published [Agent Skills format](https://agentskills.io/specification). Skills live in directories containing YAML-frontmatter `SKILL.md` files:

```text
~/.agents/skills/<name>/SKILL.md
<project-root>/.agents/skills/<name>/SKILL.md
<applicable-nested-directory>/.agents/skills/<name>/SKILL.md
```

Collections may also use grouping folders:

```text
.agents/skills/
├── openai-cli/                     # direct skill
│   ├── SKILL.md
│   └── references/commands.md     # read only on demand
└── api-tools/                      # organizational group, no SKILL.md
    └── model-inspection/           # grouped skill
        └── SKILL.md                # name: model-inspection
```

Discovery walks each collection breadth-first with sorted directory names. It skips hidden folders and `node_modules`, and stops descending once a directory contains `SKILL.md`, so a bundle's examples, scripts and references cannot become accidental skills. Grouping folders do not change a skill's name: select the leaf skill as `--skill model-inspection`.

The nearest valid project skill wins when names overlap, followed by project-root and user skills. Within one collection, the first valid copy in breadth-first, name-sorted traversal wins: direct children are considered before grouped copies. `skills list --all` shows shadowed paths. Higher-priority roots are scanned first so user collections cannot exhaust the discovery budget before project overrides. Files in unrelated repositories are not searched. No CLAI-specific installation or copy is needed for skills already in these locations; `~/.codex` and `~/.claude` are not fallback roots.

Discovery reads only YAML metadata. Names must match their directory, use 1–64 lowercase ASCII letters/digits with single intervening hyphens, and descriptions must contain 1–1024 characters. Standard optional fields are accepted; unknown optional fields are ignored. `allowed-tools` is metadata and grants no permissions. Only selected skills' Markdown bodies enter the request. The metadata index is bounded separately and shortens descriptions before omitting entries.

Selection uses local name/description matching, without embeddings or an extra model call. Casual topic mentions do not suffice; tied matches produce a diagnostic directing you to explicit selection. Up to two skills activate automatically by default. Explicit selection takes precedence and can name up to eight skills:

```bash
clai skills list
clai skills list --all
clai skills show openai-cli
clai skills show openai-cli --body
clai skills validate examples/skills/openai-cli
clai skills reload
clai --skill openai-cli list my available OpenAI models
clai --question --skill openai-cli explain how to list models
```

All inspection commands work without a provider key. `list --all` includes invalid and shadowed sources. `validate` checks UTF-8 text and frontmatter within a 1 MiB file limit. `reload` rebuilds discovery; normal requests also rescan, so file changes do not require restarting CLAI. Interactive equivalents include `/skills list`, `/skills use openai-cli`, `/skills off` and `/context show`. Selections and deactivation are session-local. Activation prints `skill: <name>` with its selection reason; set `skills_quiet=true` to suppress routine notices.

### On-demand references and boundaries

With a tool-capable provider, an active skill with eligible documentation exposes `clai_read_skill_reference({"skill":"name","path":"references/guide.md"})`. This read-only internal tool works with `use_tools=false`; it does not enable external MCP tools. Providers without tool calling still receive the selected skill's main instructions and display a notice when references are unavailable.

Eligible references are `.md`, `.txt`, `.rst` and `.adoc` files at the skill root (except `SKILL.md`) or within `references/`. Scripts, assets, hidden files and arbitrary filesystem paths are excluded. Only paths listed at activation can be read. Absolute/drive/UNC paths, traversal, escaping symlinks and non-regular files are rejected. Active skill directories are pinned for the request; repeated reference reads use request-local cached text. Oversized files are rejected with a diagnostic, never executed.

Discovery is limited to six directory levels below each collection root, 2,000 scheduled directories and 512 candidate skills, with at most 512 entries in each scanned directory. Cycles and duplicate directory aliases are skipped; limit and traversal diagnostics are visible. Reference listings allow 32 paths, four nested levels and 256 examined entries; a reference subdirectory with over 128 entries is omitted. Each request permits eight reference calls. YAML frontmatter is capped at 8 KiB. Relative symlinks are accepted only within the applicable allowed root: global configuration directory for AGENTS, project root for project AGENTS, the selected skills collection for skill discovery, or the active skill directory for references. Collection roots must themselves resolve within the home/project scope. Absolute symlinks are unsupported even when their apparent target is inside a permitted root. These are deliberate compatibility limits.

Applicable guidance, the bounded skill metadata index, activated instructions and requested references are sent to the configured response provider. They are untrusted content and cannot change CLAI configuration, execute scripts, grant tool permissions, or bypass command review. They are not supplied separately to System One routing or auditing. Expanded instruction documents are not stored in history, and reference results are redacted before history is saved; model answers and user messages retain normal history behavior. Only use local instructions you are willing to share with that provider.

### OpenAI CLI example

[examples/skills/openai-cli](examples/skills/openai-cli) is a portable, optional example. Copy that entire directory into a discovered `.agents/skills` directory to try it; it is not installed automatically. Its basic model-list command is verified against the [official CLI reference](https://developers.openai.com/api/reference/cli/resources/models/methods/list), and its bundled reference links to version-aware documentation without fixed model IDs or credentials.

With that skill installed, `clai list the OpenAI API models I have access to` activates `openai-cli` and can propose `openai models list` through the normal approval flow. `clai --skill openai-cli ...` forces selection. `clai list the files in this directory` does not activate it.

## Configuration reference

CLAI creates `~/.config/clai.cfg` on first use. It uses the established CLAI `key=value` format. The config path must be a regular file rather than a directory or symbolic link; CLAI enforces mode `0600` before reading it.

| Key | Default | Purpose |
| --- | --- | --- |
| `key` | empty | Provider credential. It must be non-empty before a request is made. |
| `hi_contrast` | `false` | Render informational text without the muted italic style. |
| `expose_current_dir` | `true` | Include the current working directory in model context. |
| `max_history_turns` | `10` | Maximum number of user turns retained in persisted history. |
| `api` | `https://api.openai.com/v1/chat/completions` | Provider endpoint used for requests and adapter detection. |
| `model` | `gpt-4.1` | Model sent to the provider. |
| `json_mode` | `false` | Ask the provider to enforce CLAI's structured response schema. |
| `temp` | `0.1` | Sampling temperature. Invalid values fall back to `0.1`. |
| `tokens` | `500` | Maximum requested output tokens. Invalid or non-positive values fall back to `500`. |
| `reasoning` | empty | Optional reasoning-effort value; provider behavior is described above. |
| `system_one_key` | empty | Optional System One-compatible API credential for typed intent routing and risk auditing. |
| `system_one_api` | `https://api.typesafe.ai/v1/systemone` | System One-compatible HTTPS evaluation endpoint. Used only when key, API, and model are all present. Redirects must remain on the configured HTTPS origin (same host and port). |
| `system_one_model` | `jev-latest` | System One model used for typed judgments. |
| `use_tools` | `false` | Enable external MCP discovery and invocation; the restricted internal skill reference reader is independent. |
| `share_command_results` | `false` | Send bounded command results for immediate model interpretation and retain them for later context. |
| `result_lines` | `20` | Maximum recent stdout and stderr lines stored for each shared result. |
| `confirm_dangerous_commands` | `true` | Require a second confirmation for danger-zone commands. |
| `risk_appetite` | `0` | Automatic execution policy from `0` through `2`; invalid values fall back to `0`. |
| `exec_query` | empty | Replace the built-in command-generation guidance when set. |
| `question_query` | empty | Replace answer-only guidance for `clai --question <question>` or requests routed as questions by System One. Ordinary requests without System One use `exec_query`, regardless of punctuation. |
| `error_query` | empty | Replace the built-in error-recovery guidance when set. |
| `agents_enabled` | `true` | Discover applicable AGENTS guidance. |
| `agents_max_bytes` | `32768` | Aggregate guidance bytes; capped at 4 MiB. |
| `agents_file_max_bytes` | `16384` | Per-file guidance bytes; capped at 1 MiB. |
| `skills_enabled` | `true` | Discover and activate local Agent Skills. |
| `skills_auto_activate` | `true` | Allow deterministic local relevance matching. |
| `skills_max_index_chars` | `8000` | Serialized metadata index characters; capped at 65,536. |
| `skills_max_active` | `2` | Automatically selected skills, from 0 through 8. Explicit selections can still activate up to eight. |
| `skills_file_max_bytes` | `24576` | Per-selected-SKILL.md bytes, including frontmatter; capped at 1 MiB. |
| `skills_reference_max_bytes` | `16384` | Per-reference text bytes, capped at 16 KiB. Tool results also retain the registry's 32 KiB serialized-result limit. |
| `skills_quiet` | `false` | Suppress routine skill activation/reference notices. Warnings remain visible. |

Re-run `clai setup` to change the credential, endpoint, model, or risk appetite. Edit the file directly for the remaining settings.

### Path overrides

The default compatible paths are:

| Data | Default path | Override |
| --- | --- | --- |
| Configuration | `~/.config/clai.cfg` | `CLAI_CONFIG` |
| History | `${XDG_STATE_HOME:-$HOME/.local/state}/clai/history_com.json` | `CLAI_HISTORY` |

Path overrides are useful for tests or separate profiles:

```bash
export CLAI_CONFIG=/tmp/clai-profile/config.cfg
export CLAI_HISTORY=/tmp/clai-profile/history.json
```

## History and command-result sharing

CLAI persists user and assistant conversation messages as JSON. System prompts are rebuilt for each request and are not retained. History is trimmed by user turn according to `max_history_turns`.

Inspect or clear it with:

```bash
clai --show-history
clai --show-history --verbose
clai --clear-history
```

Command stdout, stderr, exit status, and whether the command was edited are not stored by default. Toggle that behavior with:

```bash
clai --show-results-sharing
clai --toggle-results-sharing
```

When enabled, every executed command is followed by a second model request. CLAI sends the original request together with the command, exit status, edit status, and at most `result_lines` recent lines and 64 KiB from each stdout and stderr stream. The byte limit also bounds a single unusually long line. CLAI then displays and stores the model's conclusions—for example, whether observed load and memory figures indicate that a machine is overwhelmed.

The interpretation request cannot call tools, and CLAI discards any command, risk, or variables returned in the interpretation response. Both successful and failed command results are interpreted. This adds one provider request, with corresponding latency and cost, for each executed command.

Shared results also become part of later conversation context. Do not enable sharing for commands likely to expose secrets or sensitive data, and treat command output as potentially untrusted content; the interpretation prompt explicitly tells the model not to follow instructions found in stdout or stderr.

History files and newly created state directories use restrictive permissions. Clearing history removes the persisted history file but does not delete the configuration.

## Hot-loaded tools

CLAI loads tools from independent [Model Context Protocol](https://modelcontextprotocol.io/) servers at runtime. A tool server is a separate executable, can live in its own repository, and does not require an import, registry edit, CLAI branch, or CLAI rebuild. CLAI uses the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) over stdin/stdout.

External tools are disabled by default. Opt in by editing `~/.config/clai.cfg`:

```ini
use_tools=true
```

When `use_tools=false`, ordinary and interactive requests do not scan MCP manifests, load cached MCP definitions, start tool servers, expose external tool schemas, or execute external tools. The explicit `clai tools list|reload` and interactive `/tools list|reload` management commands remain available for inspecting and developing installed tools, but those tools are not exposed to normal requests until enabled. The separate, read-only [skill reference reader](#agent-skills) can operate with external tools disabled.

CLAI scans `${XDG_CONFIG_HOME:-$HOME/.config}/clai/tools.d/*.json`. Set `CLAI_TOOLS_DIR` to override that location. Tool definitions are cached in a private `.tool-cache` file in that directory, so an unchanged server does not start merely to advertise its tools. The server process starts lazily only if the model actually calls one of its tools, then remains available for the rest of that CLAI process.

The first request after installing or changing a manifest or executable must discover that server's definitions. Executable contents—not only timestamps and file sizes—participate in cache invalidation. CLAI discovers all new or stale servers concurrently and refreshes the cache. Interactive sessions check for those changes before each request. Normal requests through providers without tool-call support skip external-tool loading entirely.

These commands are also available:

```bash
clai tools list
clai tools reload
```

Inside an interactive session, use `/tools list` or `/tools reload`. `tools list` uses valid cached definitions and discovers only new or changed servers. `tools reload` deliberately starts every enabled server and rediscovers all definitions, which is useful while developing a plugin.

Each server's ID namespaces its tools. A server with ID `wikipedia` and MCP tool `lookup` is exposed to the LLM as `wikipedia__lookup`, preventing collisions between independently maintained plugins. Broken manifests and servers produce warnings and are omitted without disabling healthy plugins.

Tool calls are currently available through OpenAI, generic OpenAI-compatible endpoints, and Ollama. Anthropic and Gemini requests do not yet include CLAI tool definitions.

### Install the Wikipedia tutorial

[`examples/wikipedia/main.go`](examples/wikipedia/main.go) is a complete standalone Go MCP server. It searches Wikipedia, retrieves the best matching introductory plain-text extract, and returns structured title, summary, language, and source URL fields. Its [`wikipedia.json`](examples/wikipedia/wikipedia.json) manifest is deliberately copyable.

Install it independently of the CLAI binary:

```bash
make install-wikipedia
clai tools list
```

The target builds the server as `~/.config/clai/tools.d/wikipedia` and installs its private manifest beside it. CLAI will then report successful calls without exposing raw arguments:

```text
Used the Wikipedia tool with query "Margaret Thatcher".
```

### Create a separately versioned Go tool

Start a new repository rather than adding the tool to CLAI:

```bash
mkdir my-clai-tool
cd my-clai-tool
git init
go mod init example.com/my-clai-tool
go get github.com/modelcontextprotocol/go-sdk/mcp
```

The essential server shape is:

```go
package main

import (
	"context"
	"errors"
	"io"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Arguments struct {
	Query string `json:"query" jsonschema:"topic to find"`
}

type Result struct {
	Answer string `json:"answer"`
	Source string `json:"source"`
}

func lookup(_ context.Context, _ *mcp.CallToolRequest, input Arguments) (*mcp.CallToolResult, Result, error) {
	return nil, Result{Answer: "...", Source: "https://example.com/..."}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{Name: "my-service", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "lookup",
		Title:       "My Service",
		Description: "Look up information using My Service.",
	}, lookup)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil &&
		!errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "server is closing: EOF") {
		log.Print(err)
	}
}
```

MCP derives the JSON input and output schemas from the typed structs. Keep stdout exclusively for MCP messages; write diagnostics to stderr. Unit-test API behaviour with an injected HTTP client, as the Wikipedia example does.

Build the executable, place it anywhere you control, and add a manifest under `tools.d`:

```json
{
  "id": "my_service",
  "command": "/home/alice/.local/bin/my-clai-tool",
  "capabilities": ["network-read"],
  "safe_arguments": {
    "lookup": ["query"]
  },
  "timeout_seconds": 20
}
```

`command` may be absolute or relative to the manifest. Manifests must be regular files that are not group- or world-writable. Commands must be executable regular files and must not be group- or world-writable.

| Manifest field | Purpose |
| --- | --- |
| `id` | Required server namespace of up to 32 letters, digits, `_`, or `-`. |
| `command` | Required absolute executable path or path relative to the manifest. |
| `args` | Optional fixed command arguments. |
| `environment` | Environment-variable names explicitly inherited from CLAI. |
| `capabilities` | Descriptive `network-read`, `local-read`, or `local-write` metadata; it is not an OS sandbox. |
| `safe_arguments` | Per-tool string arguments that may appear in invocation notices. |
| `timeout_seconds` | Startup, discovery, and per-call timeout from 1 to 300 seconds; defaults to 15. |
| `enabled` | Optional boolean; defaults to `true`. |

`safe_arguments` controls which string arguments CLAI may show in an invocation notice. Omit it for tools whose arguments may be sensitive. CLAI never displays raw arguments automatically.

Commit the tool's source, tests, `go.mod`, `go.sum`, README, and an example manifest to its own repository. Publish binaries or let users install with `go install`; only the local manifest and executable belong on the CLAI machine.

### Use third-party API keys safely

External servers inherit no environment variables by default. A manifest allowlists names whose current values CLAI may pass to that process:

```json
{
  "id": "web_crawler",
  "command": "/home/alice/.local/bin/my-crawler",
  "environment": ["SPECIAL_CRAWLER_API_KEY"],
  "capabilities": ["network-read"],
  "timeout_seconds": 30
}
```

The Go server reads it normally:

```go
apiKey := os.Getenv("SPECIAL_CRAWLER_API_KEY")
```

Store only the variable name in Git. Supply its value through the shell, systemd, a container secret, or another secret manager. Do not include keys in the MCP schema, invocation summaries, results, logs, URLs, or returned errors.

External servers run with the invoking user's authority and must be treated as trusted executable code. CLAI restricts their inherited environment, enforces manifest and executable permissions, bounds each discovery and call by the manifest timeout, limits encoded results to 32 KiB, and closes started subprocesses on reload or exit. The private definition cache contains schemas and descriptions only; it does not contain environment values, tool results, or API keys.

> [!CAUTION]
> Tool results are added to the conversation and may be sent to the configured model provider. Bound server-side network response sizes, return only necessary fields, include source URLs, and treat retrieved page content as untrusted data that may contain prompt injection.

## Architecture

The Go implementation keeps orchestration separate from operating-system and provider boundaries:

| Package | Responsibility |
| --- | --- |
| `cmd/clai` | Cobra entry point, signals, help, version, and shell initialization. |
| `internal/app` | Session orchestration, prompts, structured replies, variables, risk decisions, and error recovery. |
| `internal/config` | Compatible `key=value` configuration, defaults, validation, and atomic private writes. |
| `internal/history` | Compatible JSON history, retention, rendering, clearing, and atomic persistence. |
| `internal/model` | Shared typed messages, tool calls, replies, risks, and command results. |
| `internal/provider` | Native HTTP payloads, authentication, response parsing, and provider-specific structured output. |
| `internal/runner` | Context-aware Bash execution, live output, capture, and result truncation. |
| `internal/ui` | Inline terminal styling, prompts, secret input, setup, confirmations, and spinner. |
| `pkg/tool` | Internal adapter contract, validation, provider definitions, dispatch, and result limits. |
| `internal/mcptools` | Manifest discovery, restricted subprocess environments, MCP sessions, namespacing, reloads, and lifecycle. |
| `examples/wikipedia` | Copyable standalone Wikipedia MCP server, manifest, and isolated HTTP tests. |

The application depends on interfaces for the provider client and command runner. That keeps orchestration tests deterministic and avoids live network calls or real command execution.

### Framework choices

- [Cobra](https://github.com/spf13/cobra) supplies the command lifecycle, help, and version behavior while CLAI retains free-form arguments.
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) supplies declarative terminal styling.
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) provides terminal detection and hidden API-key input.
- The official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) supplies the external tool client/server protocol and stdio transport.
- Go's `net/http` and `encoding/json` remain explicit because provider protocols differ enough that a generic REST abstraction would hide important behavior.
- A full-screen Bubble Tea application is intentionally out of scope; CLAI keeps its compact inline workflow.

## Development

Install Go 1.27.3 and Bats:

```bash
sudo apt update
sudo apt install bats
```

Available targets:

| Command | Checks performed |
| --- | --- |
| `make build` | Build `./clai` from `./cmd/clai`. |
| `make test` | Run all Go unit tests. |
| `make vet` | Run `go vet ./...`. |
| `make integration-test` | Run the installer and migration-cleanup Bats suites. |
| `make check` | Run vet, Go tests, and installer tests. |
| `make install-wikipedia` | Build and install the optional tutorial MCP server and manifest. |
| `make cleanup-legacy` | Dry-run the conservative Bash-install cleanup script. |
| `make clean` | Run `go clean` and remove the local `clai` binary. |

Run the race detector separately when changing concurrent or I/O behavior:

```bash
go test -race ./...
```

Tests use fake providers and isolated temporary state; they do not make successful live API calls. CI runs the full `make check` workflow.

GoReleaser builds static Linux, macOS, and Windows archives and a checksum file from tags. Test the release configuration locally without publishing:

```bash
goreleaser release --snapshot --clean --skip=publish
```

Pushing a semantic-version tag that matches [`internal/app/VERSION`](internal/app/VERSION) runs [the release workflow](.github/workflows/release.yml), repeats the complete cross-platform check suite, and publishes only a GitHub Release with the configured archives and `checksums.txt`. No package manager, container registry, or announcement publisher is configured. The workflow checks out and validates the selected tag before building, so the binary version and archive names continue to come from that tag.

For a new release, update `internal/app/VERSION` in the release PR, merge it, then create and push the matching `vX.Y.Z` tag from that merge commit. Do not maintain the release number in any other source or workflow file.

## Migration from the Bash version

The Go implementation replaces the former Bash application while preserving the `clai request words here` interface and established configuration, history, prompt, risk, and shell-initialization behavior. The removed Bash sources, documentation, and tests remain available through Git history; they are not duplicated in the current working tree or release artifacts.

### Why migrate

The Bash application had grown into a large program responsible for provider-specific JSON, state persistence, process execution, terminal presentation, and an in-process plugin contract. Go provides clearer package boundaries and compile-time types for those responsibilities, deterministic HTTP and JSON handling, bounded and cancellation-aware command execution, race-tested concurrency, and reproducible native release binaries. Users no longer need `curl` or `jq` at runtime.

The migration also replaces sourced Bash plugins with independently versioned MCP server processes. A plugin can now live in its own repository, use its own dependencies and API credentials, be discovered without changing CLAI, and fail without sharing CLAI's shell process. This is an intentional plugin-contract change; legacy `~/.clai_tools/*.sh` files are not loaded by the Go version.

### Reused and obsolete paths

| Path or integration | Migration treatment |
| --- | --- |
| `~/.config/clai.cfg` or `CLAI_CONFIG` | Reused. Contains provider settings and must not be deleted. |
| `${XDG_STATE_HOME:-~/.local/state}/clai/history_com.json` or `CLAI_HISTORY` | Reused. Conversation history must not be deleted. |
| Existing shell alias from `clai shell-init` | Reused because it invokes the same `clai` command name. |
| `/usr/local/bin/clai` or the selected install target | Reused as the command location and replaced with the Go binary. |
| `${XDG_CONFIG_HOME:-~/.config}/clai/tools.d` or `CLAI_TOOLS_DIR` | Current Go MCP plugins; never treated as legacy. |
| `/usr/local/lib/clai/clai.sh` | Obsolete after the active command has been replaced by the Go binary. |
| The three unmodified stock scripts under `~/.clai_tools` | Obsolete. Modified and third-party files are preserved for manual migration or archival. |

The cleanup does not uninstall `curl`, `jq`, Bash, or any system package because they may be used by unrelated software.

### Recommended migration

Optionally create a private config backup, then install the Go version with either the release bootstrap or a source checkout.

The release bootstrap is the quickest migration path. It downloads the binary for the current platform, verifies the release checksum and reported version, and replaces the active `/usr/local/bin/clai` command:

```bash
install -m 0600 ~/.config/clai.cfg ~/.config/clai.cfg.pre-go
curl -fsSL https://raw.githubusercontent.com/merefield/clai/main/install-release.sh | sh
```

Set `CLAI_BIN_DIR="$HOME/.local/bin"` on the `sh` command as shown in [Quickstart](#quickstart) when the existing command is installed there. Alternatively, build and install from source:

```bash
git clone https://github.com/merefield/clai.git
cd clai
./install.sh
```

Whichever installation path you choose, test the Go binary before cleaning anything:

```bash
clai --version
clai how much is 3 times pi
```

The legacy cleanup script remains in the repository. If you used the release bootstrap without an existing checkout, clone the repository before continuing. Review the cleanup without changing the filesystem:

```bash
./scripts/cleanup-legacy.sh
# or: make cleanup-legacy
```

If every listed path is expected, apply it:

```bash
./scripts/cleanup-legacy.sh --apply
```

The script refuses to proceed unless the active `clai` is a native Linux or macOS binary that identifies itself through `clai --version`, and refuses to remove a legacy payload still targeted by the active command. It removes only files whose SHA-256 hashes match the final stock Bash release, removes directories only when they become empty, and prints every preserved path. Unknown, modified, and third-party legacy plugins remain under `~/.clai_tools` for manual porting to MCP or archival.

For non-default old locations, identify both paths explicitly:

```bash
CLAI_LEGACY_INSTALL_DIR="$HOME/.local/lib/clai" \
CLAI_LEGACY_TOOLS_DIR="$HOME/.clai_tools" \
./scripts/cleanup-legacy.sh --clai-path "$HOME/.local/bin/clai"
```

Do not apply cleanup until the Go binary has passed your normal workflows. Before cleanup, rollback is simply restoring the old command target; afterwards, retrieve the Bash implementation from Git history if it is needed.

## Credits and license

CLAI is a hard fork of [`bash-ai`](https://github.com/Hezkore/bash-ai), which was inspired by [Your AI](https://github.com/ekkinox/yai).

CLAI is distributed under the [GNU General Public License version 3 or any later version](LICENSE.txt).
