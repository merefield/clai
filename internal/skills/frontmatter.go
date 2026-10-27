// Package skills implements metadata-first Agent Skills discovery and loading.
package skills

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const MaxFrontmatterBytes = 8192

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func resolveAlias(node *yaml.Node) *yaml.Node {
	for depth := 0; node != nil && node.Kind == yaml.AliasNode && depth < 32; depth++ {
		node = node.Alias
	}
	return node
}

type Metadata struct {
	Name, Description, License, Compatibility string
}

// Only read through the closing frontmatter delimiter during discovery. No
// skill bodies or reference contents are collected for the metadata index.
func frontmatter(reader io.Reader, directory string) (Metadata, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxFrontmatterBytes+1))
	scanner.Buffer(make([]byte, 512), MaxFrontmatterBytes+1)
	if !scanner.Scan() || strings.TrimPrefix(scanner.Text(), "\ufeff") != "---" {
		return Metadata{}, fmt.Errorf("SKILL.md must begin with YAML frontmatter")
	}
	var header strings.Builder
	closed := false
	for scanner.Scan() {
		if scanner.Text() == "---" {
			closed = true
			break
		}
		header.WriteString(scanner.Text())
		header.WriteByte('\n')
	}
	if !closed || header.Len() > MaxFrontmatterBytes || scanner.Err() != nil {
		return Metadata{}, fmt.Errorf("missing frontmatter delimiter or frontmatter exceeds %d bytes", MaxFrontmatterBytes)
	}
	if !utf8.ValidString(header.String()) {
		return Metadata{}, fmt.Errorf("frontmatter must be UTF-8")
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(header.String()), &document); err != nil {
		return Metadata{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return Metadata{}, fmt.Errorf("frontmatter must be a YAML mapping")
	}
	values := map[any]yaml.Node{}
	if err := document.Decode(&values); err != nil {
		return Metadata{}, fmt.Errorf("invalid YAML metadata mapping: %w", err)
	}
	fields := map[string]*yaml.Node{}
	for key, node := range values {
		name, ok := key.(string)
		if !ok {
			return Metadata{}, fmt.Errorf("frontmatter keys must be strings")
		}
		fields[name] = resolveAlias(&node)
	}
	get := func(key string) (string, error) {
		node := fields[key]
		if node == nil {
			return "", nil
		}
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return "", fmt.Errorf("%s must be a string", key)
		}
		return node.Value, nil
	}
	metadata := Metadata{}
	for _, field := range []struct {
		key    string
		target *string
	}{{"name", &metadata.Name}, {"description", &metadata.Description}, {"license", &metadata.License}, {"compatibility", &metadata.Compatibility}} {
		value, err := get(field.key)
		if err != nil {
			return Metadata{}, err
		}
		*field.target = value
	}
	if len(metadata.Name) > 64 || !namePattern.MatchString(metadata.Name) || metadata.Name != filepath.Base(directory) {
		return Metadata{}, fmt.Errorf("name must match its directory and contain 1–64 lowercase letters, digits and single hyphens")
	}
	if strings.TrimSpace(metadata.Description) == "" || utf8.RuneCountInString(metadata.Description) > 1024 {
		return Metadata{}, fmt.Errorf("description must contain 1–1024 characters")
	}
	if utf8.RuneCountInString(metadata.Compatibility) > 500 {
		return Metadata{}, fmt.Errorf("compatibility exceeds 500 characters")
	}
	if node := fields["metadata"]; node != nil {
		if node.Kind != yaml.MappingNode {
			return Metadata{}, fmt.Errorf("metadata must be a string-to-string mapping")
		}
		properties := map[any]yaml.Node{}
		if err := node.Decode(&properties); err != nil {
			return Metadata{}, fmt.Errorf("invalid YAML metadata: %w", err)
		}
		for key, property := range properties {
			if _, ok := key.(string); !ok {
				return Metadata{}, fmt.Errorf("metadata keys and values must be strings")
			}
			value := resolveAlias(&property)
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return Metadata{}, fmt.Errorf("metadata keys and values must be strings")
			}
		}
	}
	if node := fields["allowed-tools"]; node != nil && (node.Kind != yaml.ScalarNode || node.Tag != "!!str") {
		return Metadata{}, fmt.Errorf("allowed-tools must be a space-separated string; it never grants CLAI permissions")
	}
	return metadata, nil
}
