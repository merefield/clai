package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("CLAI_MCP_HELPER") != "1" {
		return
	}
	if milliseconds, _ := strconv.Atoi(os.Getenv("CLAI_MCP_DELAY_MS")); milliseconds > 0 {
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	}
	if path := os.Getenv("CLAI_MCP_START_FILE"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
		_, _ = file.WriteString("started\n")
		_ = file.Close()
	}
	type input struct {
		Query string `json:"query" jsonschema:"the topic to find"`
	}
	type output struct {
		Answer          string `json:"answer"`
		ReceivedAPIKey  bool   `json:"received_api_key"`
		SawUnlistedData bool   `json:"saw_unlisted_data"`
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "clai-test-server", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "lookup",
		Title:       "Test Search",
		Description: "Search test data.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, arguments input) (*mcp.CallToolResult, output, error) {
		return nil, output{
			Answer:          "found " + arguments.Query,
			ReceivedAPIKey:  os.Getenv("TEST_PLUGIN_KEY") == "test-key",
			SawUnlistedData: os.Getenv("UNLISTED_SECRET") != "",
		}, nil
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "server is closing: EOF") {
		fmt.Fprintln(os.Stderr, err)
	}
}

func TestManagerDiscoversExecutesAndReloadsExternalServer(t *testing.T) {
	directory := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAI_MCP_HELPER", "1")
	startFile := filepath.Join(directory, "starts")
	t.Setenv("CLAI_MCP_START_FILE", startFile)
	t.Setenv("TEST_PLUGIN_KEY", "test-key")
	t.Setenv("UNLISTED_SECRET", "must-not-be-inherited")
	manifestPath := filepath.Join(directory, "search.json")
	writeManifest(t, manifestPath, map[string]any{
		"id":           "search",
		"command":      executable,
		"args":         []string{"-test.run=TestMCPHelperProcess"},
		"environment":  []string{"CLAI_MCP_HELPER", "CLAI_MCP_START_FILE", "TEST_PLUGIN_KEY"},
		"capabilities": []string{"network-read"},
		"safe_arguments": map[string][]string{
			"lookup": {"query"},
		},
	})

	manager, err := New(directory, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if warnings := manager.Reload(context.Background()); len(warnings) != 0 {
		t.Fatalf("reload warnings = %v", warnings)
	}
	if manager.Dirty() {
		t.Fatal("freshly loaded manager is dirty")
	}
	if count := startCount(t, startFile); count != 1 {
		t.Fatalf("discovery starts = %d, want 1", count)
	}

	cachedManager, err := New(directory, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	if warnings := cachedManager.Load(context.Background()); len(warnings) != 0 {
		t.Fatalf("cached load warnings = %v", warnings)
	}
	if count := startCount(t, startFile); count != 1 {
		t.Fatalf("cached load started server; starts = %d", count)
	}
	if _, err := cachedManager.Registry().Execute(context.Background(), "search__lookup", `{"query":"lazy"}`); err != nil {
		t.Fatal(err)
	}
	if count := startCount(t, startFile); count != 2 {
		t.Fatalf("lazy call starts = %d, want 2", count)
	}
	if err := cachedManager.Close(); err != nil {
		t.Fatal(err)
	}
	if names := manager.Registry().Names(); len(names) != 1 || names[0] != "search__lookup" {
		t.Fatalf("tool names = %v", names)
	}
	function := manager.Registry().Definitions()[0]["function"].(map[string]any)
	parameters := function["parameters"].(map[string]any)
	required := parameters["required"].([]any)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required parameters = %#v", required)
	}
	result, err := manager.Registry().Execute(context.Background(), "search__lookup", `{"query":"Botswana"}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Answer          string `json:"answer"`
		ReceivedAPIKey  bool   `json:"received_api_key"`
		SawUnlistedData bool   `json:"saw_unlisted_data"`
	}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Answer != "found Botswana" || !decoded.ReceivedAPIKey || decoded.SawUnlistedData {
		t.Fatalf("result = %#v", decoded)
	}
	if summary := manager.Registry().InvocationSummary("search__lookup", `{"query":"Botswana"}`); summary != `Used the Test Search tool with query "Botswana".` {
		t.Fatalf("invocation summary = %q", summary)
	}
	servers := manager.Servers()
	if len(servers) != 1 || servers[0].ID != "search" || len(servers[0].Tools) != 1 {
		t.Fatalf("servers = %#v", servers)
	}
	badPath := filepath.Join(directory, "broken.json")
	if err := os.WriteFile(badPath, []byte(`{"id":"broken","command":"missing","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	warnings := manager.Reload(context.Background())
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "unknown field") {
		t.Fatalf("isolated reload warnings = %v", warnings)
	}
	if manager.Registry().Len() != 1 {
		t.Fatal("broken manifest disabled the healthy server")
	}
	if err := os.Remove(badPath); err != nil {
		t.Fatal(err)
	}

	writeManifest(t, manifestPath, map[string]any{
		"id":      "search",
		"command": executable,
		"enabled": false,
	})
	if !manager.Dirty() {
		t.Fatal("manifest change was not detected")
	}
	if warnings := manager.Reload(context.Background()); len(warnings) != 0 {
		t.Fatalf("disabled reload warnings = %v", warnings)
	}
	if manager.Registry().Len() != 0 || len(manager.Servers()) != 0 {
		t.Fatal("disabled manifest remained active")
	}
}

func TestManagerIsolatesInvalidManifests(t *testing.T) {
	directory := t.TempDir()
	badPath := filepath.Join(directory, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"id":"bad","command":"missing","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	warnings := manager.Reload(context.Background())
	if len(warnings) != 1 || !strings.Contains(warnings[0].Error(), "unknown field") {
		t.Fatalf("warnings = %v", warnings)
	}
	if manager.Registry().Len() != 0 {
		t.Fatal("invalid manifest registered a tool")
	}
}

func TestManagerDiscoversStaleServersConcurrently(t *testing.T) {
	directory := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAI_MCP_HELPER", "1")
	t.Setenv("CLAI_MCP_DELAY_MS", "700")
	for _, id := range []string{"first", "second"} {
		writeManifest(t, filepath.Join(directory, id+".json"), map[string]any{
			"id":          id,
			"command":     executable,
			"args":        []string{"-test.run=TestMCPHelperProcess"},
			"environment": []string{"CLAI_MCP_HELPER", "CLAI_MCP_DELAY_MS"},
		})
	}
	manager, err := New(directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	started := time.Now()
	if warnings := manager.Reload(context.Background()); len(warnings) != 0 {
		t.Fatalf("reload warnings = %v", warnings)
	}
	if elapsed := time.Since(started); elapsed > 1200*time.Millisecond {
		t.Fatalf("discovery was sequential: %s", elapsed)
	}
	if manager.Registry().Len() != 2 {
		t.Fatalf("tools = %v", manager.Registry().Names())
	}
}

func TestDefaultDirectoryHonoursOverride(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "tools")
	t.Setenv("CLAI_TOOLS_DIR", directory)
	got, err := DefaultDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got != directory {
		t.Fatalf("directory = %q, want %q", got, directory)
	}
}

func TestManifestRejectsWritableFiles(t *testing.T) {
	directory := t.TempDir()
	commandPath := filepath.Join(directory, "tool")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, "tool.json")
	writeManifest(t, manifestPath, map[string]any{"id": "tool", "command": commandPath})

	if err := os.Chmod(commandPath, 0o722); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("writable command error = %v", err)
	}
	if err := os.Chmod(commandPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(manifestPath, 0o622); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("writable manifest error = %v", err)
	}
}

func writeManifest(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startCount(t *testing.T, path string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(body), "started\n")
}
