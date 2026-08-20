// Package mcptools discovers external MCP tool servers and adapts their tools
// to CLAI's provider-independent tool registry.
package mcptools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/merefield/clai/pkg/tool"
)

const (
	cacheFilename    = ".tool-cache"
	cacheVersion     = 1
	defaultTimeout   = 15 * time.Second
	maximumCacheSize = 1 << 20
	maximumTimeout   = 5 * time.Minute
)

var (
	idPattern          = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Manifest describes one trusted local MCP server. Environment contains names
// only: values are copied from CLAI's environment when the server is started.
type Manifest struct {
	ID               string              `json:"id"`
	Command          string              `json:"command"`
	Args             []string            `json:"args,omitempty"`
	Environment      []string            `json:"environment,omitempty"`
	Capabilities     []tool.Capability   `json:"capabilities,omitempty"`
	SafeArguments    map[string][]string `json:"safe_arguments,omitempty"`
	TimeoutSeconds   int                 `json:"timeout_seconds,omitempty"`
	Enabled          *bool               `json:"enabled,omitempty"`
	manifestPath     string
	resolvedCommand  string
	executionTimeout time.Duration
}

// Server describes a successfully loaded manifest and its exposed CLAI names.
type Server struct {
	ID       string
	Manifest string
	Tools    []string
}

type loadedServer struct {
	info    Server
	runtime *runtimeServer
}

type cachedTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type cachedServer struct {
	Signature string       `json:"signature"`
	Tools     []cachedTool `json:"tools"`
}

type definitionCache struct {
	Version int                     `json:"version"`
	Servers map[string]cachedServer `json:"servers"`
}

type runtimeServer struct {
	manifest Manifest
	stderr   io.Writer
	onChange func()

	mu      sync.Mutex
	session *mcp.ClientSession
}

// Manager owns runtime MCP sessions and an immutable registry snapshot.
type Manager struct {
	directory string
	stderr    io.Writer

	mu        sync.RWMutex
	registry  *tool.Registry
	servers   []loadedServer
	signature string
	dirty     atomic.Bool
}

// DefaultDirectory returns CLAI's per-user external tool manifest directory.
func DefaultDirectory() (string, error) {
	if directory := strings.TrimSpace(os.Getenv("CLAI_TOOLS_DIR")); directory != "" {
		return filepath.Abs(directory)
	}
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDirectory, "clai", "tools.d"), nil
}

func New(directory string, stderr io.Writer) (*Manager, error) {
	if strings.TrimSpace(directory) == "" {
		var err error
		directory, err = DefaultDirectory()
		if err != nil {
			return nil, err
		}
	}
	empty, err := tool.NewRegistry()
	if err != nil {
		return nil, err
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Manager{directory: directory, stderr: stderr, registry: empty}, nil
}

func (m *Manager) Directory() string { return m.directory }

func (m *Manager) Registry() *tool.Registry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.registry
}

func (m *Manager) Servers() []Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	servers := make([]Server, len(m.servers))
	for index, server := range m.servers {
		servers[index] = server.info
		servers[index].Tools = append([]string(nil), server.info.Tools...)
	}
	return servers
}

func (m *Manager) Dirty() bool {
	if m.dirty.Load() {
		return true
	}
	m.mu.RLock()
	prior := m.signature
	m.mu.RUnlock()
	return directorySignature(m.directory) != prior
}

// Load builds a registry from valid cached definitions and discovers only new
// or changed servers. A server process is otherwise started only when called.
func (m *Manager) Load(ctx context.Context) []error {
	return m.load(ctx, m.dirty.Load())
}

// Reload forces concurrent rediscovery of every enabled server.
func (m *Manager) Reload(ctx context.Context) []error {
	return m.load(ctx, true)
}

// load constructs a complete new snapshot before replacing the active one. A
// broken manifest is reported and omitted without affecting other servers.
func (m *Manager) load(ctx context.Context, forceDiscovery bool) []error {
	manifests, warnings := readManifests(m.directory)
	cache, cacheWarning := readDefinitionCache(m.directory)
	if cacheWarning != nil {
		warnings = append(warnings, cacheWarning)
	}

	type loadResult struct {
		server loadedServer
		tools  []tool.Tool
		cache  cachedServer
		err    error
	}
	results := make([]loadResult, len(manifests))
	var wait sync.WaitGroup
	for index, manifest := range manifests {
		wait.Add(1)
		go func() {
			defer wait.Done()
			runtime := &runtimeServer{
				manifest: manifest,
				stderr:   m.stderr,
				onChange: func() { m.dirty.Store(true) },
			}
			signature := manifestSignature(manifest)
			definitions, cached := cache.Servers[manifest.ID]
			if forceDiscovery || !cached || definitions.Signature != signature {
				discovered, err := runtime.discover(ctx)
				if err != nil {
					results[index].err = err
					_ = runtime.close()
					return
				}
				definitions = cachedServer{Signature: signature, Tools: discovered}
			}
			adapters, names, err := adaptTools(manifest, runtime, definitions.Tools)
			if err != nil {
				results[index].err = err
				_ = runtime.close()
				return
			}
			if len(adapters) == 0 {
				results[index].err = fmt.Errorf("server exposes no compatible tools")
				_ = runtime.close()
				return
			}
			results[index] = loadResult{
				server: loadedServer{
					info:    Server{ID: manifest.ID, Manifest: manifest.manifestPath, Tools: names},
					runtime: runtime,
				},
				tools: adapters,
				cache: definitions,
			}
		}()
	}
	wait.Wait()

	implementations := make([]tool.Tool, 0)
	servers := make([]loadedServer, 0, len(manifests))
	nextCache := definitionCache{Version: cacheVersion, Servers: map[string]cachedServer{}}
	for index, result := range results {
		if result.err != nil {
			warnings = append(warnings, fmt.Errorf("load MCP server %q: %w", manifests[index].ID, result.err))
			continue
		}
		servers = append(servers, result.server)
		implementations = append(implementations, result.tools...)
		nextCache.Servers[manifests[index].ID] = result.cache
	}

	registry, err := tool.NewRegistry(implementations...)
	if err != nil {
		warnings = append(warnings, fmt.Errorf("build runtime tool registry: %w", err))
		for _, server := range servers {
			_ = server.runtime.close()
		}
		return warnings
	}
	if len(manifests) > 0 && !reflect.DeepEqual(cache, nextCache) {
		if err := writeDefinitionCache(m.directory, nextCache); err != nil {
			warnings = append(warnings, fmt.Errorf("write MCP definition cache: %w", err))
		}
	}

	m.mu.Lock()
	oldServers := m.servers
	m.servers = servers
	m.registry = registry
	m.signature = directorySignature(m.directory)
	m.dirty.Store(false)
	m.mu.Unlock()

	for _, server := range oldServers {
		if err := server.runtime.close(); err != nil {
			warnings = append(warnings, fmt.Errorf("close MCP server %q: %w", server.info.ID, err))
		}
	}
	return warnings
}

func (r *runtimeServer) connect(ctx context.Context) (*mcp.ClientSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil {
		return r.session, nil
	}
	command := exec.Command(r.manifest.resolvedCommand, r.manifest.Args...)
	command.Env = restrictedEnvironment(r.manifest.Environment)
	command.Stderr = r.stderr
	client := mcp.NewClient(
		&mcp.Implementation{Name: "clai", Version: "1.1"},
		&mcp.ClientOptions{
			Capabilities: &mcp.ClientCapabilities{},
			ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
				if r.onChange != nil {
					r.onChange()
				}
			},
		},
	)
	connectContext, cancel := context.WithTimeout(ctx, r.manifest.executionTimeout)
	session, err := client.Connect(connectContext, &mcp.CommandTransport{
		Command:           command,
		TerminateDuration: 2 * time.Second,
	}, nil)
	cancel()
	if err != nil {
		return nil, err
	}
	r.session = session
	return session, nil
}

func (r *runtimeServer) discover(ctx context.Context) ([]cachedTool, error) {
	session, err := r.connect(ctx)
	if err != nil {
		return nil, err
	}
	listContext, cancel := context.WithTimeout(ctx, r.manifest.executionTimeout)
	defer cancel()
	var definitions []cachedTool
	for remote, listErr := range session.Tools(listContext, nil) {
		if listErr != nil {
			return nil, fmt.Errorf("list tools: %w", listErr)
		}
		definition, err := cacheTool(remote)
		if err != nil {
			return nil, fmt.Errorf("read tool %q: %w", remote.Name, err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (r *runtimeServer) call(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	session, err := r.connect(ctx)
	if err != nil {
		return nil, err
	}
	callContext, cancel := context.WithTimeout(ctx, r.manifest.executionTimeout)
	defer cancel()
	return session.CallTool(callContext, &mcp.CallToolParams{Name: name, Arguments: arguments})
}

func (r *runtimeServer) close() error {
	r.mu.Lock()
	session := r.session
	r.session = nil
	r.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Close()
}

func (m *Manager) Close() error {
	m.mu.Lock()
	servers := m.servers
	m.servers = nil
	m.mu.Unlock()
	var closeErrors []error
	for _, server := range servers {
		if err := server.runtime.close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("close MCP server %q: %w", server.info.ID, err))
		}
	}
	return errors.Join(closeErrors...)
}

type adapter struct {
	definition    tool.Definition
	remoteName    string
	safeArguments []string
	runtime       *runtimeServer
}

func cacheTool(remote *mcp.Tool) (cachedTool, error) {
	if remote == nil || !idPattern.MatchString(remote.Name) {
		return cachedTool{}, fmt.Errorf("name must match %s", idPattern)
	}
	parameters, err := schemaObject(remote.InputSchema)
	if err != nil {
		return cachedTool{}, err
	}
	title := strings.TrimSpace(remote.Title)
	if title == "" && remote.Annotations != nil {
		title = strings.TrimSpace(remote.Annotations.Title)
	}
	return cachedTool{Name: remote.Name, Title: title, Description: remote.Description, InputSchema: parameters}, nil
}

func adaptTools(manifest Manifest, runtime *runtimeServer, definitions []cachedTool) ([]tool.Tool, []string, error) {
	adapters := make([]tool.Tool, 0, len(definitions))
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		adapter, err := newAdapter(manifest, runtime, definition)
		if err != nil {
			return nil, nil, fmt.Errorf("adapt tool %q: %w", definition.Name, err)
		}
		adapters = append(adapters, adapter)
		names = append(names, adapter.definition.Name)
	}
	sort.Strings(names)
	return adapters, names, nil
}

func newAdapter(manifest Manifest, runtime *runtimeServer, remote cachedTool) (*adapter, error) {
	if !idPattern.MatchString(remote.Name) {
		return nil, fmt.Errorf("name must match %s", idPattern)
	}
	name := manifest.ID + "__" + remote.Name
	if len(name) > 64 {
		return nil, fmt.Errorf("namespaced name %q exceeds 64 characters", name)
	}
	if schemaType, _ := remote.InputSchema["type"].(string); schemaType != "object" {
		return nil, fmt.Errorf("input schema type must be object")
	}
	displayName := strings.TrimSpace(remote.Title)
	description := strings.TrimSpace(remote.Description)
	if description == "" {
		description = fmt.Sprintf("Tool %s provided by MCP server %s.", remote.Name, manifest.ID)
	}
	return &adapter{
		definition: tool.Definition{
			Name:         name,
			DisplayName:  displayName,
			Description:  description,
			Parameters:   remote.InputSchema,
			Capabilities: append([]tool.Capability(nil), manifest.Capabilities...),
		},
		remoteName:    remote.Name,
		safeArguments: append([]string(nil), manifest.SafeArguments[remote.Name]...),
		runtime:       runtime,
	}, nil
}

func (a *adapter) Definition() tool.Definition { return a.definition }

func (a *adapter) Execute(ctx context.Context, raw json.RawMessage) (any, error) {
	var arguments map[string]any
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return nil, fmt.Errorf("decode arguments: %w", err)
	}
	result, err := a.runtime.call(ctx, a.remoteName, arguments)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("MCP server returned an empty result")
	}
	if result.StructuredContent != nil && !result.IsError {
		return result.StructuredContent, nil
	}
	return map[string]any{"content": result.Content, "is_error": result.IsError}, nil
}

func (a *adapter) InvocationSummary(raw json.RawMessage) string {
	if len(a.safeArguments) == 0 {
		return ""
	}
	var arguments map[string]any
	if json.Unmarshal(raw, &arguments) != nil {
		return ""
	}
	details := make([]string, 0, len(a.safeArguments))
	for _, name := range a.safeArguments {
		value, exists := arguments[name]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		details = append(details, strings.ReplaceAll(name, "_", " ")+" "+fmt.Sprintf("%q", strings.Join(strings.Fields(text), " ")))
	}
	if len(details) == 0 {
		return ""
	}
	return "with " + strings.Join(details, " and ")
}

func readManifests(directory string) ([]Manifest, []error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{fmt.Errorf("read MCP manifest directory %s: %w", directory, err)}
	}
	var manifests []Manifest
	var warnings []error
	seen := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		manifest, enabled, err := readManifest(path)
		if err != nil {
			warnings = append(warnings, err)
			continue
		}
		if !enabled {
			continue
		}
		if prior, exists := seen[manifest.ID]; exists {
			warnings = append(warnings, fmt.Errorf("read MCP manifest %s: duplicate id %q already declared by %s", path, manifest.ID, prior))
			continue
		}
		seen[manifest.ID] = path
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].ID < manifests[j].ID })
	return manifests, warnings
}

func readManifest(path string) (Manifest, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Manifest{}, false, err
	}
	if !info.Mode().IsRegular() {
		return Manifest{}, false, fmt.Errorf("read MCP manifest %s: not a regular file", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return Manifest{}, false, fmt.Errorf("read MCP manifest %s: file must not be group- or world-writable", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, false, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, false, fmt.Errorf("read MCP manifest %s: %w", path, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Manifest{}, false, fmt.Errorf("read MCP manifest %s: trailing JSON data", path)
	}
	if manifest.Enabled != nil && !*manifest.Enabled {
		return manifest, false, nil
	}
	manifest.manifestPath = path
	if err := validateManifest(&manifest); err != nil {
		return Manifest{}, false, fmt.Errorf("read MCP manifest %s: %w", path, err)
	}
	return manifest, true, nil
}

func validateManifest(manifest *Manifest) error {
	if !idPattern.MatchString(manifest.ID) {
		return fmt.Errorf("id must match %s", idPattern)
	}
	if strings.TrimSpace(manifest.Command) == "" {
		return fmt.Errorf("command is required")
	}
	command := manifest.Command
	if !filepath.IsAbs(command) {
		command = filepath.Join(filepath.Dir(manifest.manifestPath), command)
	}
	command, err := filepath.Abs(command)
	if err != nil {
		return fmt.Errorf("resolve command: %w", err)
	}
	info, err := os.Stat(command)
	if err != nil {
		return fmt.Errorf("inspect command %s: %w", command, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("command %s is not an executable regular file", command)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("command %s must not be group- or world-writable", command)
	}
	manifest.resolvedCommand = command

	seenEnvironment := map[string]bool{}
	for _, name := range manifest.Environment {
		if !environmentPattern.MatchString(name) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
		if seenEnvironment[name] {
			return fmt.Errorf("duplicate environment variable name %q", name)
		}
		seenEnvironment[name] = true
	}
	for _, capability := range manifest.Capabilities {
		switch capability {
		case tool.CapabilityNetworkRead, tool.CapabilityLocalRead, tool.CapabilityLocalWrite:
		default:
			return fmt.Errorf("unknown capability %q", capability)
		}
	}
	for toolName, names := range manifest.SafeArguments {
		if !idPattern.MatchString(toolName) {
			return fmt.Errorf("invalid safe_arguments tool name %q", toolName)
		}
		for _, name := range names {
			if !environmentPattern.MatchString(name) {
				return fmt.Errorf("invalid safe argument name %q", name)
			}
		}
	}
	if manifest.TimeoutSeconds == 0 {
		manifest.executionTimeout = defaultTimeout
	} else {
		manifest.executionTimeout = time.Duration(manifest.TimeoutSeconds) * time.Second
		if manifest.executionTimeout < time.Second || manifest.executionTimeout > maximumTimeout {
			return fmt.Errorf("timeout_seconds must be between 1 and %d", int(maximumTimeout/time.Second))
		}
	}
	return nil
}

func restrictedEnvironment(names []string) []string {
	environment := make([]string, 0, len(names))
	for _, name := range names {
		if value, exists := os.LookupEnv(name); exists {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func schemaObject(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal input schema: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return nil, fmt.Errorf("decode input schema: %w", err)
	}
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	if schemaType, _ := schema["type"].(string); schemaType != "object" {
		return nil, fmt.Errorf("input schema type must be object")
	}
	return schema, nil
}

func readDefinitionCache(directory string) (definitionCache, error) {
	empty := definitionCache{Version: cacheVersion, Servers: map[string]cachedServer{}}
	path := filepath.Join(directory, cacheFilename)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("read MCP definition cache: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return empty, fmt.Errorf("read MCP definition cache: %s must be a private regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, fmt.Errorf("read MCP definition cache: %w", err)
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumCacheSize+1))
	if err != nil {
		return empty, fmt.Errorf("read MCP definition cache: %w", err)
	}
	if len(body) > maximumCacheSize {
		return empty, fmt.Errorf("read MCP definition cache: exceeds %d bytes", maximumCacheSize)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var cache definitionCache
	if err := decoder.Decode(&cache); err != nil {
		return empty, fmt.Errorf("read MCP definition cache: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return empty, fmt.Errorf("read MCP definition cache: trailing JSON data")
	}
	if cache.Version != cacheVersion {
		return empty, fmt.Errorf("read MCP definition cache: unsupported version %d", cache.Version)
	}
	if cache.Servers == nil {
		cache.Servers = map[string]cachedServer{}
	}
	return cache, nil
}

func writeDefinitionCache(directory string, cache definitionCache) error {
	encoded, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	if len(encoded) > maximumCacheSize {
		return fmt.Errorf("cache exceeds %d bytes", maximumCacheSize)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, cacheFilename+".*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(directory, cacheFilename))
}

func manifestSignature(manifest Manifest) string {
	hash := sha256.New()
	body, readErr := os.ReadFile(manifest.manifestPath)
	fmt.Fprintf(hash, "%x\x00%v", body, readErr)
	writeFileSignature(hash, manifest.resolvedCommand)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func directorySignature(directory string) string {
	hash := sha256.New()
	entries, err := os.ReadDir(directory)
	if err != nil {
		fmt.Fprint(hash, err)
		return fmt.Sprintf("%x", hash.Sum(nil))
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		data, readErr := os.ReadFile(path)
		fmt.Fprintf(hash, "%s\x00%x\x00%v", entry.Name(), data, readErr)
		var commandOnly struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(data, &commandOnly) != nil || strings.TrimSpace(commandOnly.Command) == "" {
			continue
		}
		command := commandOnly.Command
		if !filepath.IsAbs(command) {
			command = filepath.Join(filepath.Dir(path), command)
		}
		writeFileSignature(hash, command)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeFileSignature(hash io.Writer, path string) {
	fmt.Fprintf(hash, "\x00%s\x00", path)
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprint(hash, err)
		return
	}
	defer file.Close()
	info, statErr := file.Stat()
	if statErr != nil {
		fmt.Fprint(hash, statErr)
		return
	}
	fmt.Fprintf(hash, "%d\x00%d\x00%d\x00", info.Size(), info.ModTime().UnixNano(), info.Mode())
	if _, err := io.Copy(hash, file); err != nil {
		fmt.Fprint(hash, "\x00", err)
	}
}
