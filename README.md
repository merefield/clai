# CLAI

CLAI is an AI-powered terminal assistant that turns natural-language requests into shell commands you can inspect, edit, approve, or reject before they run.

```text
clai list files by size
clai how much is 3 times pi
```

The primary implementation is now Go. It keeps the existing `clai <request...>` interface, configuration and history locations, and risk controls while moving API clients, orchestration, persistence, terminal UI, and LLM tools into typed Go packages.

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
- Native, compiled Go tools exposed through provider function calling
- A Wikipedia lookup plugin that doubles as a copyable implementation tutorial
- Shell integration for unquoted `*` and `?` in Bash and zsh

## Requirements

To run a compiled CLAI binary:

- Linux or macOS on AMD64 or ARM64
- Bash, used to execute approved commands
- An API key, provider credential, or non-empty placeholder required by a local service configuration

The Go binary implements HTTP, JSON, and tool handling itself. It does not require `curl` or `jq`.

Building from source requires the Go toolchain declared by [`go.mod`](go.mod): Go 1.26.6 for this branch.

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

Ensure `$HOME/.local/bin` is on `PATH` if you use that location. `CLAI_BIN_NAME` can temporarily select another executable name, for example while comparing implementations:

```bash
CLAI_BIN_DIR="$HOME/.local/bin" CLAI_BIN_NAME=goclai ./install.sh
```

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

Release archives are configured through [GoReleaser](.goreleaser.yaml) for Linux and macOS on AMD64 and ARM64.

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
| `clai --show-history` | Render persisted conversation history. |
| `clai --show-history --verbose` | Include full stored command stdout and stderr. |
| `clai --clear-history` | Remove persisted conversation history. |
| `clai --show-results-sharing` | Report whether command results are stored for later context. |
| `clai --toggle-results-sharing` | Enable or disable command-result sharing. |
| `clai shell-init bash` | Print Bash integration for literal glob characters. |
| `clai shell-init zsh` | Print zsh integration for literal glob characters. |
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

Approved commands run through `bash -o errexit -o pipefail -c`. Stdout and stderr stream to the terminal. After a failed command in an interactive terminal, CLAI can send the command and its stderr back to the provider to request an explanation and possible repair.

Risk is model-generated guidance, not a security boundary. Read every proposed or edited command before approving it.

## Providers

CLAI selects its native adapter from the configured `api` URL:

| Provider | Typical endpoint | Authentication and behavior |
| --- | --- | --- |
| OpenAI | `https://api.openai.com/v1/chat/completions` | Bearer token; supports native JSON Schema and local tool calls. |
| OpenAI-compatible | Provider-specific Chat Completions URL | Bearer token; uses the OpenAI-compatible message and tool format. |
| Ollama | `http://localhost:11434/api/chat` | Uses the configured model, disables streaming, and supports Ollama tool-call responses. Keep `key` non-empty even if the service ignores it. |
| Anthropic | `https://api.anthropic.com/v1/messages` | Uses `x-api-key` and the Messages payload. Local CLAI tools are not sent. |
| Gemini | A `generativelanguage.googleapis.com` `generateContent` URL | Uses `x-goog-api-key`; the configured model replaces the model segment in the request URL. Local CLAI tools are not sent. |

Provider detection is URL-based. Other URLs use the generic OpenAI-compatible adapter, so use a Chat Completions-compatible endpoint and model.

When `json_mode=true`, CLAI requests provider-enforced structured JSON:

- OpenAI uses JSON Schema.
- Generic OpenAI-compatible endpoints request a JSON object.
- Ollama sends `format: "json"`.
- Anthropic uses structured output configuration.
- Gemini uses a JSON response schema.

For Ollama, any non-empty `reasoning` value enables `think: true` and also requests JSON output. For OpenAI completion endpoints, `reasoning` is sent as `reasoning_effort` only for known reasoning-model families; generic compatible completion endpoints receive the configured value directly.

Example Ollama configuration:

```ini
key=ollama
api=http://localhost:11434/api/chat
model=gemma4:latest
json_mode=true
reasoning=true
```

## Configuration reference

CLAI creates `~/.config/clai.cfg` on first use. It uses the established CLAI `key=value` format.

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
| `share_command_results` | `false` | Store executed-command results so they can be included in later model context. |
| `result_lines` | `20` | Maximum recent stdout and stderr lines stored for each shared result. |
| `confirm_dangerous_commands` | `true` | Require a second confirmation for danger-zone commands. |
| `risk_appetite` | `0` | Automatic execution policy from `0` through `2`; invalid values fall back to `0`. |
| `exec_query` | empty | Replace the built-in command-generation guidance when set. |
| `question_query` | empty | Replace the built-in question-mode guidance when set. |
| `error_query` | empty | Replace the built-in error-recovery guidance when set. |

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

When enabled, CLAI retains at most `result_lines` recent lines from each stdout and stderr stream. These results become part of later conversation context and may therefore be sent to the configured provider. Do not enable sharing for commands likely to expose secrets or sensitive data.

History files and newly created state directories use restrictive permissions. Clearing history removes the persisted history file but does not delete the configuration.

## Native Go tools

Tools are compiled Go capabilities exposed to compatible models through provider function calling. They let a model obtain information during its reasoning loop and then use the result in its final answer.

The legacy `~/.clai_tools/*.sh` loader has been removed from the Go application. Tools no longer source arbitrary Bash, depend on `jq`, or inherit CLAI's shell process. The registry validates names and schemas, dispatches typed implementations, JSON-encodes results, and rejects results larger than 32 KiB.

The initial [`wikipedia_lookup`](plugins/wikipedia/wikipedia.go) plugin searches Wikipedia, retrieves the best matching article's introductory plain-text extract, and returns its title, summary, language, and source URL. It uses the official MediaWiki [Search API](https://www.mediawiki.org/wiki/API:Search) and [TextExtracts API](https://www.mediawiki.org/wiki/Extension:TextExtracts).

After a successful call, CLAI reports the tool and a plugin-controlled safe summary in the interface:

```text
Used the Wikipedia tool with query "Margaret Thatcher".
```

CLAI never prints raw tool arguments automatically. A plugin opts into additional detail through `InvocationSummarizer`, which prevents future plugins from accidentally displaying API keys or sensitive request fields.

Tool calls are currently available through OpenAI, generic OpenAI-compatible endpoints, and Ollama. Anthropic and Gemini requests do not yet include CLAI tool definitions.

### Add a tool by copying the Wikipedia plugin

The framework contract is public in [`pkg/tool`](pkg/tool/). Actual tools are kept outside the core under [`plugins/`](plugins/), and [`plugins/registry.go`](plugins/registry.go) is the only registration point.

Start by copying the tutorial plugin:

```bash
cp -R plugins/wikipedia plugins/my_service
```

Then edit the copied package:

1. Rename its Go package and `Plugin` implementation as desired.
2. Define typed `Arguments` and `Result` structs.
3. Change `Definition()` to provide the unique function name, human-facing display name, model-facing description, JSON parameter schema, and capability metadata.
4. Change `Execute()` to validate arguments, perform the operation with its `context.Context`, and return a JSON-marshalable result.
5. Add the constructor to [`plugins/registry.go`](plugins/registry.go).
6. Copy and adapt the tests, injecting an HTTP client or other dependency so tests do not contact the live service.

The essential implementation shape is:

```go
type Plugin struct {
	client *http.Client
}

type Arguments struct {
	Query string `json:"query"`
}

type Result struct {
	Answer string `json:"answer"`
	Source string `json:"source"`
}

func (p *Plugin) Definition() tool.Definition {
	return tool.Definition{
		Name:         "my_service_lookup",
		DisplayName:  "My Service",
		Description:  "Look up information using My Service.",
		Capabilities: []tool.Capability{tool.CapabilityNetworkRead},
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		},
	}
}

func (p *Plugin) Execute(ctx context.Context, raw json.RawMessage) (any, error) {
	var arguments Arguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return nil, err
	}
	// Perform the bounded, context-aware request here.
	return Result{Answer: "...", Source: "https://example.com/..."}, nil
}

func (p *Plugin) InvocationSummary(raw json.RawMessage) string {
	var arguments Arguments
	if json.Unmarshal(raw, &arguments) != nil {
		return ""
	}
	return fmt.Sprintf("with query %q", arguments.Query)
}
```

Returning a typed value keeps provider-specific message formatting out of the plugin. The registry serializes it as JSON before returning it to the LLM.

### Use environment variables for third-party API keys

A compiled plugin can read environment variables normally. Prefer reading a key once during registration and injecting it into the plugin rather than reading it on every call:

```go
// plugins/registry.go
registered := []tool.Tool{
	wikipedia.New(client),
}

if apiKey := os.Getenv("SPECIAL_CRAWLER_API_KEY"); apiKey != "" {
	registered = append(registered, crawler.New(client, apiKey))
}

return tool.NewRegistry(registered...)
```

Store the value privately on the implementation and use it only to authenticate the outbound request:

```go
type Plugin struct {
	client *http.Client
	apiKey string
}

func New(client *http.Client, apiKey string) *Plugin {
	return &Plugin{client: client, apiKey: apiKey}
}

func (p *Plugin) Execute(ctx context.Context, raw json.RawMessage) (any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://crawler.example/api", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	// Send the request, enforce status/body limits, and return a typed result.
	return Result{}, nil
}
```

Do not include API keys in the LLM-facing parameter schema, invocation summary, tool results, log messages, URLs, or returned errors. Environment variables remain part of the CLAI process and are not automatically sent to the model. A plugin can still leak them through its own code, so every added implementation must be reviewed.

> [!CAUTION]
> Tool results are added to the conversation and may be sent to the configured model provider. Bound network timeouts and response sizes, return only necessary fields, include source URLs, and treat retrieved page content as untrusted data that may contain prompt injection.

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
| `pkg/tool` | Public native-tool contract, validation, provider definitions, dispatch, and result limits. |
| `plugins` | Explicit compiled-in tool registration. |
| `plugins/wikipedia` | Copyable Wikipedia lookup example and its isolated HTTP implementation. |

The application depends on interfaces for the provider client and command runner. That keeps orchestration tests deterministic and avoids live network calls or real command execution.

### Framework choices

- [Cobra](https://github.com/spf13/cobra) supplies the command lifecycle, help, and version behavior while CLAI retains free-form arguments.
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) supplies declarative terminal styling.
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) provides terminal detection and hidden API-key input.
- Go's `net/http` and `encoding/json` remain explicit because provider protocols differ enough that a generic REST abstraction would hide important behavior.
- A full-screen Bubble Tea application is intentionally out of scope; CLAI keeps its compact inline workflow.

## Development

Install Go 1.26.6 and Bats:

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
| `make integration-test` | Run the Go installer Bats suite. |
| `make check` | Run vet, Go tests, and installer tests. |
| `make clean` | Run `go clean` and remove the local `clai` binary. |

Run the race detector separately when changing concurrent or I/O behavior:

```bash
go test -race ./...
```

Tests use fake providers and isolated temporary state; they do not make successful live API calls. CI runs the full `make check` workflow.

GoReleaser builds static Linux and macOS archives and a checksum file from tags:

```bash
goreleaser release --snapshot --clean
```

## Migration

The Go implementation replaces the former Bash application while preserving the `clai request words here` interface and established configuration, history, prompt, and risk behavior. The removed Bash sources, installer, documentation, and tests remain available through Git history; they are not duplicated in the current working tree or release artifacts.

## Credits and license

CLAI is a hard fork of [`bash-ai`](https://github.com/Hezkore/bash-ai), which was inspired by [Your AI](https://github.com/ekkinox/yai).

CLAI is distributed under the [GNU General Public License v3.0](LICENSE.txt).
