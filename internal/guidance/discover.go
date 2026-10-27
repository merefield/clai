package guidance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	Enabled           bool
	MaxBytes, FileMax int
}

type Document struct {
	Path, Scope, Content string
	Bytes                int
}

type Result struct {
	Documents []Document
	Warnings  []string
}

func Discover(locations Locations, options Options) Result {
	result := Result{}
	if !options.Enabled {
		return result
	}
	type source struct{ directory, root, scope string }
	sources := []source{{locations.Config, locations.Config, "global"}}
	for index, directory := range locations.Chain {
		scope := "project"
		if index > 0 {
			scope = "nested project"
		}
		sources = append(sources, source{directory, locations.Project, scope})
	}
	remaining := max(0, options.MaxBytes)
	seen := map[string]bool{}
	for _, source := range sources {
		for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
			path := filepath.Join(source.directory, name)
			text, canonical, size, err := Read(source.root, path, max(0, options.FileMax))
			if err != nil {
				if !os.IsNotExist(err) {
					result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", Display(path), err))
				}
				continue
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			identity := Display(canonical)
			if seen[identity] {
				break
			}
			seen[identity] = true
			if size > remaining {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s: exceeds remaining agents_max_bytes budget; omitted", Display(path)))
				break
			}
			remaining -= size
			result.Documents = append(result.Documents, Document{Path: Display(path), Scope: source.scope, Content: text, Bytes: size})
			break
		}
	}
	return result
}
