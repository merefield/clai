package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/merefield/clai/internal/guidance"
)

type Active struct {
	Skill
	Body, Reason string
	References   []string
	root         *os.Root
}

func (a Active) Close() {
	if a.root != nil {
		a.root.Close()
	}
}

var wordsPattern = regexp.MustCompile(`[a-z0-9]+`)

func tokens(text string) map[string]bool {
	words := map[string]bool{}
	for _, word := range wordsPattern.FindAllString(strings.ToLower(text), -1) {
		words[word] = true
	}
	return words
}

func generic(word string) bool {
	return len(word) < 3 || strings.Contains(" the and for with from this that your use using tool tools command commands cli api official when about requests ", " "+word+" ")
}

func relevance(skill Skill, query string) int {
	words := tokens(query)
	name := tokens(skill.Name)
	full, anchor := true, false
	for word := range name {
		full = full && words[word]
		anchor = anchor || (!generic(word) && words[word])
	}
	if full && len(name) > 1 {
		return 100
	}
	matches := 0
	for word := range tokens(skill.Description) {
		if !generic(word) && !name[word] && words[word] {
			matches++
		}
	}
	if anchor && matches > 0 {
		return 40 + matches
	}
	if matches >= 3 {
		return 20 + matches
	}
	return 0
}

func (c Catalog) Select(query string, explicit []string, options Options) ([]Active, []string, error) {
	if !options.Enabled {
		if len(explicit) > 0 {
			return nil, nil, fmt.Errorf("skills are disabled by skills_enabled=false")
		}
		return nil, nil, nil
	}
	names := []string{}
	warnings := []string{}
	reason := "explicit selection"
	if len(explicit) > 0 {
		if len(explicit) > 8 {
			return nil, nil, fmt.Errorf("at most eight explicit skills may be selected")
		}
		for _, name := range explicit {
			if _, err := c.Find(name); err != nil {
				return nil, nil, err
			}
			if !contains(names, name) {
				names = append(names, name)
			}
		}
	} else if options.Auto {
		type match struct {
			name  string
			score int
		}
		matches := []match{}
		for _, skill := range c.Skills {
			if score := relevance(skill, query); score > 0 {
				matches = append(matches, match{skill.Name, score})
			}
		}
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].score == matches[j].score {
				return matches[i].name < matches[j].name
			}
			return matches[i].score > matches[j].score
		})
		for index := 0; index < len(matches); {
			end := index + 1
			for end < len(matches) && matches[end].score == matches[index].score {
				end++
			}
			if end-index > 1 {
				warnings = append(warnings, fmt.Sprintf("ambiguous skill match (%s, %s); use --skill to select explicitly", matches[index].name, matches[index+1].name))
			} else if len(names) < min(8, max(0, options.MaxActive)) {
				names = append(names, matches[index].name)
			}
			index = end
		}
		reason = "local relevance match"
	}
	active := []Active{}
	for _, name := range names {
		skill, _ := c.Find(name)
		loaded, err := skill.Load(options.FileMax)
		if err != nil {
			if len(explicit) > 0 {
				for _, previous := range active {
					previous.Close()
				}
				return nil, warnings, fmt.Errorf("activate skill %s: %w", name, err)
			}
			warnings = append(warnings, fmt.Sprintf("%s: %v", guidance.Display(skill.Path), err))
			continue
		}
		loaded.Reason = reason
		resources, diagnostics := loaded.resourcePaths()
		loaded.References = resources
		warnings = append(warnings, diagnostics...)
		active = append(active, loaded)
	}
	return active, warnings, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s Skill) Load(maximum int) (Active, error) {
	collection, err := guidance.OpenDirectory(s.boundary, s.collection)
	if err != nil {
		return Active{}, err
	}
	relative, err := filepath.Rel(s.collection, filepath.Dir(s.Path))
	if err != nil || !filepath.IsLocal(relative) {
		collection.Close()
		return Active{}, fmt.Errorf("skill is outside its collection")
	}
	root, err := collection.OpenRoot(relative)
	collection.Close()
	if err != nil {
		return Active{}, err
	}
	keep := false
	defer func() {
		if !keep {
			root.Close()
		}
	}()
	text, _, err := guidance.ReadAt(root, "SKILL.md", max(0, maximum))
	if err != nil {
		return Active{}, err
	}
	metadata, err := frontmatter(strings.NewReader(text), filepath.Dir(s.Path))
	if err != nil {
		return Active{}, err
	}
	if metadata != s.Metadata {
		return Active{}, fmt.Errorf("skill metadata changed during discovery; retry the request")
	}
	lines := strings.Split(text, "\n")
	for index := 1; index < len(lines); index++ {
		if strings.TrimSuffix(lines[index], "\r") == "---" {
			keep = true
			return Active{Skill: s, Body: strings.Join(lines[index+1:], "\n"), root: root}, nil
		}
	}
	return Active{}, fmt.Errorf("skill body delimiter is missing")
}
