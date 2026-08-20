// Package tool defines the small, provider-independent contract implemented by
// tools that CLAI exposes to an LLM.
package tool

import (
	"context"
	"encoding/json"
)

// Capability describes the local authority a tool implementation needs. It is
// CLAI-side metadata and is not included in the provider function schema.
type Capability string

const (
	CapabilityNetworkRead Capability = "network-read"
	CapabilityLocalRead   Capability = "local-read"
	CapabilityLocalWrite  Capability = "local-write"
)

// Definition is the provider-independent description of an LLM-facing tool.
type Definition struct {
	Name         string
	DisplayName  string
	Description  string
	Parameters   map[string]any
	Capabilities []Capability
}

// Tool is CLAI's internal adapter contract. Runtime MCP tools and test tools
// implement it so provider-specific formatting stays out of tool servers.
type Tool interface {
	Definition() Definition
	Execute(context.Context, json.RawMessage) (any, error)
}

// InvocationSummarizer is optional. A tool can implement it to expose a safe,
// human-readable subset of its arguments in CLAI's interface. It must never
// return credentials or other secret values.
type InvocationSummarizer interface {
	InvocationSummary(json.RawMessage) string
}
