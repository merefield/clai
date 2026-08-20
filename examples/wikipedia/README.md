# Wikipedia MCP tool

This directory is a complete external CLAI tool server and a template for new Go tools. It has no compile-time connection to the CLAI binary.

From the CLAI repository, install it with:

```bash
make install-wikipedia
clai tools list
```

Tool use is opt-in. Set `use_tools=true` in `~/.config/clai.cfg` before asking CLAI to use Wikipedia. The management command above can inspect the installation even while tools are disabled.

To create an independently maintained tool, copy `main.go` and `wikipedia.json` into a new repository, initialize a Go module, replace the lookup implementation, and commit that repository normally:

```bash
go mod init example.com/my-clai-tool
go get github.com/modelcontextprotocol/go-sdk/mcp
go test ./...
```

Build the resulting executable and point its manifest's `command` at it. CLAI discovers a new or changed server on its next tool-capable invocation and caches its definitions. On later requests, the server starts only when the model calls one of its tools. `/tools reload` forces immediate rediscovery while developing it.

The manifest controls the server namespace, declared capabilities, execution timeout, environment-variable allowlist, and arguments that may safely appear in CLAI's invocation notice. Never place secret values in the manifest.
