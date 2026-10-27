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

const MaxCandidates = 512

type Options struct {
	Enabled, Auto      bool
	MaxActive, FileMax int
}

type Skill struct {
	Metadata
	Path, Directory, Scope string
	boundary               string
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
	type source struct{ path, boundary, scope string }
	sources := []source{{filepath.Join(locations.Home, ".agents", "skills"), locations.Home, "user"}}
	for index, directory := range locations.Chain {
		scope := "project"
		if index > 0 {
			scope = "nested project"
		}
		sources = append(sources, source{filepath.Join(directory, ".agents", "skills"), locations.Project, scope})
	}
	winners := map[string]Skill{}
	winningEntry := map[string]int{}
	count := 0
	for _, source := range sources {
		entries, err := guidance.EntriesWithin(source.boundary, source.path, MaxCandidates)
		if err != nil {
			if !os.IsNotExist(err) {
				catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", guidance.Display(source.path), err))
			}
			continue
		}
		for _, directory := range entries {
			if !directory.IsDir() && directory.Type()&os.ModeSymlink == 0 {
				continue
			}
			count++
			if count > MaxCandidates {
				catalog.Warnings = append(catalog.Warnings, "skill discovery reached its 512-candidate limit; remaining entries omitted")
				break
			}
			path := filepath.Join(source.path, directory.Name(), "SKILL.md")
			entry := Entry{Path: guidance.Display(path), Scope: source.scope, Status: "invalid"}
			file, canonical, err := guidance.Open(source.boundary, path)
			metadata := Metadata{}
			if err == nil {
				metadata, err = frontmatter(file, filepath.Dir(path))
				file.Close()
			}
			if err != nil {
				entry.Detail = err.Error()
				catalog.Entries = append(catalog.Entries, entry)
				catalog.Warnings = append(catalog.Warnings, fmt.Sprintf("%s: %v", entry.Path, err))
				continue
			}
			entry.Name, entry.Status = metadata.Name, "available"
			if previous, exists := winningEntry[metadata.Name]; exists {
				catalog.Entries[previous].Status = "shadowed"
				catalog.Entries[previous].Detail = "overridden by " + entry.Path
			}
			winningEntry[metadata.Name] = len(catalog.Entries)
			catalog.Entries = append(catalog.Entries, entry)
			winners[metadata.Name] = Skill{Metadata: metadata, Path: path, Directory: filepath.Dir(canonical), Scope: source.scope, boundary: source.boundary}
		}
	}
	for _, skill := range winners {
		catalog.Skills = append(catalog.Skills, skill)
	}
	sort.Slice(catalog.Skills, func(i, j int) bool { return catalog.Skills[i].Name < catalog.Skills[j].Name })
	return catalog
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
