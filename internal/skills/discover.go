package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/merefield/clai/internal/guidance"
)

const (
	MaxCandidates           = 512
	MaxDiscoveryDepth       = 6
	MaxDiscoveryDirectories = 2000
)

type Options struct {
	Enabled, Auto      bool
	MaxActive, FileMax int
}

type Skill struct {
	Metadata
	Path, Directory, Scope string
	boundary               string
	collection             string
}

type Entry struct {
	Path, Name, Scope, Status, Detail string
}

type Catalog struct {
	Skills   []Skill
	Entries  []Entry
	Warnings []string
}

func Discover(locations guidance.Locations, enabled bool) Catalog {
	catalog := Catalog{}
	if !enabled {
		return catalog
	}
	sources := []source{{filepath.Join(locations.Home, ".agents", "skills"), locations.Home, "user"}}
	for index, directory := range locations.Chain {
		scope := "project"
		if index > 0 {
			scope = "nested project"
		}
		sources = append(sources, source{filepath.Join(directory, ".agents", "skills"), locations.Project, scope})
	}
	winners := map[string]Skill{}
	budget := discoveryBudget{}
	// Scan higher precedence first, so lower-priority user collections cannot
	// exhaust the global budget before project overrides have been examined.
	for index := len(sources) - 1; index >= 0; index-- {
		if budget.candidates >= MaxCandidates || budget.directories >= MaxDiscoveryDirectories {
			catalog.Warnings = append(catalog.Warnings, "skill discovery budget reached; remaining lower-priority roots were not inspected")
			break
		}
		scanSource(sources[index], &budget, &catalog, winners)
	}
	for _, skill := range winners {
		catalog.Skills = append(catalog.Skills, skill)
	}
	sort.Slice(catalog.Skills, func(i, j int) bool { return catalog.Skills[i].Name < catalog.Skills[j].Name })
	return catalog
}

type source struct{ path, boundary, scope string }
type discoveryBudget struct{ directories, candidates int }

func scanSource(source source, budget *discoveryBudget, catalog *Catalog, winners map[string]Skill) {
	root, err := guidance.OpenDirectory(source.boundary, source.path)
	if err != nil {
		if !os.IsNotExist(err) {
			catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", guidance.Display(source.path), err))
		}
		return
	}
	defer root.Close()
	type directory struct {
		path  string
		depth int
	}
	queue := []directory{{".", 0}}
	budget.directories++
	seen := []os.FileInfo{}
	directoryWarning := false
	for len(queue) > 0 {
		if budget.candidates >= MaxCandidates {
			catalog.Warnings = append(catalog.Warnings, "skill discovery reached its 512-candidate limit; remaining entries omitted")
			return
		}
		current := queue[0]
		queue = queue[1:]
		info, err := root.Stat(current.path)
		if err != nil {
			catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", guidance.Display(filepath.Join(source.path, current.path)), err))
			continue
		}
		if !info.IsDir() {
			continue
		}
		duplicate := false
		for _, previous := range seen {
			if os.SameFile(previous, info) {
				duplicate = true
				break
			}
		}
		if duplicate {
			catalog.Warnings = append(catalog.Warnings, "duplicate or cyclic skill directory ignored: "+guidance.Display(filepath.Join(source.path, current.path)))
			continue
		}
		seen = append(seen, info)
		entries, err := guidance.EntriesAt(root, current.path, MaxCandidates)
		if err != nil {
			catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", guidance.Display(filepath.Join(source.path, current.path)), err))
			continue
		}
		bundle := false
		if current.depth > 0 {
			for _, entry := range entries {
				if entry.Name() == "SKILL.md" {
					bundle = true
					break
				}
			}
		}
		if bundle {
			budget.candidates++
			relative := filepath.Join(current.path, "SKILL.md")
			path := filepath.Join(source.path, relative)
			entry := Entry{Path: guidance.Display(path), Scope: source.scope, Status: "invalid"}
			metadata := Metadata{}
			file, err := guidance.OpenAt(root, relative)
			if err == nil {
				metadata, err = frontmatter(file, filepath.Dir(path))
				file.Close()
			}
			if err != nil {
				entry.Detail = err.Error()
				catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", entry.Path, err))
			} else {
				entry.Name = metadata.Name
				if previous, exists := winners[metadata.Name]; exists {
					entry.Status = "shadowed"
					entry.Detail = "overridden by " + guidance.Display(previous.Path)
				} else {
					entry.Status = "available"
					winners[metadata.Name] = Skill{Metadata: metadata, Path: path, Directory: filepath.Dir(path), Scope: source.scope, boundary: source.boundary, collection: source.path}
				}
			}
			catalog.Entries = append(catalog.Entries, entry)
			// A bundle's scripts, references and assets are resources, not
			// additional skill collections, even if they contain SKILL.md.
			continue
		}
		depthWarning := false
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || (!entry.IsDir() && entry.Type()&os.ModeSymlink == 0) {
				continue
			}
			if current.depth >= MaxDiscoveryDepth {
				if !depthWarning {
					catalog.Warnings = append(catalog.Warnings, "skill discovery depth limit reached at "+guidance.Display(filepath.Join(source.path, current.path)))
					depthWarning = true
				}
				continue
			}
			if budget.directories >= MaxDiscoveryDirectories {
				if !directoryWarning {
					catalog.Warnings = append(catalog.Warnings, "skill discovery reached its 2000-directory limit; remaining folders omitted")
					directoryWarning = true
				}
				continue
			}
			budget.directories++
			queue = append(queue, directory{filepath.Join(current.path, entry.Name()), current.depth + 1})
		}
	}
}

func Validate(path string) (Metadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Metadata{}, err
	}
	if info.IsDir() {
		path = filepath.Join(path, "SKILL.md")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Metadata{}, err
	}
	text, _, _, err := guidance.Read(filepath.Dir(absolute), absolute, 1<<20)
	if err != nil {
		return Metadata{}, err
	}
	return frontmatter(strings.NewReader(text), filepath.Dir(absolute))
}

func (c Catalog) Names() []string {
	names := make([]string, 0, len(c.Skills))
	for _, skill := range c.Skills {
		names = append(names, skill.Name)
	}
	return names
}

func (c Catalog) Find(name string) (Skill, error) {
	for _, skill := range c.Skills {
		if skill.Name == name {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("unknown or invalid skill %q; available skills: %s", name, strings.Join(c.Names(), ", "))
}

// Index reduces descriptions before omitting entries and bounds the actual
// serialized character count, including names, scopes and source paths.
func (c Catalog) Index(maximum int) (string, []string) {
	for _, descriptionLimit := range []int{160, 80, 40} {
		var index strings.Builder
		omitted := 0
		for _, skill := range c.Skills {
			description := []rune(skill.Description)
			if len(description) > descriptionLimit {
				description = append(description[:descriptionLimit-1], '…')
			}
			entry, _ := json.Marshal(map[string]string{"name": skill.Name, "description": string(description), "path": guidance.Display(skill.Path), "scope": skill.Scope})
			line := string(entry) + "\n"
			if utf8.RuneCountInString(index.String())+utf8.RuneCountInString(line) > maximum {
				omitted++
				continue
			}
			index.WriteString(line)
		}
		if omitted == 0 {
			return index.String(), nil
		}
		if descriptionLimit == 40 {
			return index.String(), []string{fmt.Sprintf("%d skill index entries omitted by skills_max_index_chars; inspect skills list or use --skill", omitted)}
		}
	}
	return "", nil
}
