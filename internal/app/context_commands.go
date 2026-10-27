package app

import (
	"fmt"
	"strings"

	"github.com/merefield/clai/internal/guidance"
	"github.com/merefield/clai/internal/skills"
)

const contextHelp = `Local context commands (no API key required):
  clai context show [--verbose]
  clai skills list [--all]
  clai skills show NAME [--body]
  clai skills validate PATH
  clai skills reload
  clai --skill NAME [--skill NAME] [--question] <request>
Interactive: /skills list, /skills use NAME, /skills off, /context show
Local documents are untrusted guidance, sent to the configured response provider.
`

func (a *Application) handleContextCommand(args []string) error {
	if len(args) == 1 || args[len(args)-1] == "--help" || args[len(args)-1] == "-h" {
		a.UI.Plain(contextHelp)
		return nil
	}
	if args[0] == "skills" && args[1] == "validate" {
		if len(args) != 3 {
			return fmt.Errorf("usage: clai skills validate PATH")
		}
		metadata, err := skills.Validate(args[2])
		if err != nil {
			return err
		}
		a.UI.Plain("Valid skill: " + metadata.Name + "\n")
		return nil
	}
	if args[0] == "skills" && args[1] == "off" && len(args) == 2 {
		a.selectedSkills, a.skillsOff = nil, true
		a.UI.Info("Skills off for this session.")
		return nil
	}
	guides, catalog, err := a.discoverContext()
	if err != nil {
		return err
	}
	if args[0] == "context" && args[1] == "show" && (len(args) == 2 || (len(args) == 3 && args[2] == "--verbose")) {
		a.UI.Plain("AGENTS guidance (earlier to later precedence):\n")
		if len(guides.Documents) == 0 {
			a.UI.Plain("  none\n")
		}
		if len(args) == 3 {
			a.UI.Info("Verbose output displays local instructions, which may contain private information.")
		}
		for _, document := range guides.Documents {
			a.UI.Plain(fmt.Sprintf("  %s | %d bytes | %s\n", document.Scope, document.Bytes, document.Path))
			if len(args) == 3 {
				a.UI.Plain(document.Content + "\n")
			}
		}
		a.UI.Plain(fmt.Sprintf("Skills: %d available; session selection: %s\n", len(catalog.Skills), strings.Join(a.selectedSkills, ", ")))
		if len(a.selectedSkills) > 0 {
			a.UI.Plain("Session skills were explicitly selected and take precedence over automatic matching.\n")
		} else if a.Config.SkillsAutoActivate && !a.skillsOff {
			a.UI.Plain("Automatic skills are resolved per request by local relevance matching; inspect skills list for available sources.\n")
		}
		if a.skillsOff {
			a.UI.Plain("Automatic skill activation is off for this session.\n")
		}
		a.UI.Plain("Guidance and activated skill instructions are sent to the configured response provider; inspection does not load reference bodies.\n")
	} else if args[0] == "skills" {
		switch args[1] {
		case "list":
			if len(args) > 3 || (len(args) == 3 && args[2] != "--all") {
				return fmt.Errorf("usage: clai skills list [--all]")
			}
			for _, skill := range catalog.Skills {
				a.UI.Plain(fmt.Sprintf("%s | %s | %s\n  %s\n", skill.Name, skill.Scope, guidance.Display(skill.Path), skill.Description))
			}
			if len(catalog.Skills) == 0 {
				a.UI.Plain("No skills found.\n")
			}
			if len(args) == 3 {
				for _, entry := range catalog.Entries {
					if entry.Status != "available" {
						a.UI.Plain(fmt.Sprintf("%s | %s | %s\n", entry.Status, entry.Path, entry.Detail))
					}
				}
			}
		case "show":
			if len(args) < 3 || len(args) > 4 || (len(args) == 4 && args[3] != "--body") {
				return fmt.Errorf("usage: clai skills show NAME [--body]")
			}
			skill, err := catalog.Find(args[2])
			if err != nil {
				return err
			}
			a.UI.Plain(fmt.Sprintf("%s\n%s\nScope: %s\nSource: %s\nLicense: %s\nCompatibility: %s\n", skill.Name, skill.Description, skill.Scope, guidance.Display(skill.Path), skill.License, skill.Compatibility))
			if len(args) == 4 {
				active, err := skill.Load(a.Config.SkillsFileMaxBytes)
				if err != nil {
					return err
				}
				defer active.Close()
				a.UI.Info("Skill body follows; it may contain private information.")
				a.UI.Plain(active.Body + "\n")
			}
		case "reload":
			if len(args) != 2 {
				return fmt.Errorf("usage: clai skills reload")
			}
			a.UI.Plain(fmt.Sprintf("Reloaded %d skills. Discovery refreshes from disk on every request.\n", len(catalog.Skills)))
		case "use":
			if len(args) < 3 || len(args) > 10 {
				return fmt.Errorf("usage: /skills use NAME [NAME]")
			}
			for _, name := range args[2:] {
				if _, err := catalog.Find(name); err != nil {
					return err
				}
			}
			a.selectedSkills, a.skillsOff = append([]string(nil), args[2:]...), false
			a.UI.Info("Session skills: " + strings.Join(a.selectedSkills, ", "))
		default:
			return fmt.Errorf("unknown skills command; use clai skills --help")
		}
	} else {
		return fmt.Errorf("usage: clai context show [--verbose]")
	}
	for _, warning := range append(guides.Warnings, catalog.Warnings...) {
		a.UI.Error("Local context: " + warning)
	}
	return nil
}
