package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/merefield/clai/internal/model"
)

var (
	placeholderPattern  = regexp.MustCompile(`\{\{[A-Za-z_][A-Za-z0-9_]*\}\}`)
	variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func ParseReply(text string) (model.Reply, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			text = text[newline+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		start, end := strings.IndexByte(text, '{'), strings.LastIndexByte(text, '}')
		if start < 0 || end <= start || json.Unmarshal([]byte(text[start:end+1]), &raw) != nil {
			return model.Reply{}, fmt.Errorf("parse structured reply: %w", err)
		}
	}
	normalized := map[string]json.RawMessage{}
	for key, value := range raw {
		normalized[strings.Join(strings.Fields(key), "")] = value
	}
	b, _ := json.Marshal(normalized)
	var reply model.Reply
	if err := json.Unmarshal(b, &reply); err != nil {
		return model.Reply{}, err
	}
	reply.Command = strings.TrimSpace(reply.Command)
	reply.Info = cleanText(reply.Info)
	reply.Risk = NormalizeRisk(strings.TrimSpace(reply.Risk), reply.Command)
	reply.Variables = normalizeVariables(reply.Variables, reply.Command, reply.Info)
	return reply, nil
}

func NormalizeRisk(risk, command string) string {
	switch risk {
	case model.RiskNone, model.RiskReversible, model.RiskDanger:
		return risk
	default:
		if command == "" {
			return model.RiskNone
		}
		return model.RiskDanger
	}
}

func RequiresConfirmation(risk string, appetite int) bool {
	switch risk {
	case model.RiskDanger:
		return true
	case model.RiskReversible:
		return appetite < 2
	default:
		return appetite < 1
	}
}

func HasPlaceholders(value string) bool {
	return placeholderPattern.MatchString(value)
}

func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`).MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func ResolveVariable(command, info, name, value string) (string, string, error) {
	placeholder := "{{" + name + "}}"
	quoted := ShellQuote(value)
	command, err := resolveCommandVariable(command, placeholder, quoted)
	if err != nil {
		return command, info, err
	}
	info = strings.ReplaceAll(info, placeholder, value)
	return command, info, nil
}

func resolveCommandVariable(command, placeholder, quoted string) (string, error) {
	if !strings.Contains(command, placeholder) {
		return command, nil
	}
	if strings.ContainsAny(command, "\r\n") {
		return command, fmt.Errorf("resolve %s: placeholders in multiline commands are unsupported", placeholder)
	}
	singleToken := "'" + placeholder + "'"
	doubleToken := `"` + placeholder + `"`
	var resolved strings.Builder
	resolved.Grow(len(command) + len(quoted))
	var quote byte
	for i := 0; i < len(command); {
		if quote == 0 {
			if strings.HasPrefix(command[i:], singleToken) || strings.HasPrefix(command[i:], doubleToken) {
				if i > 0 && command[i-1] == '$' {
					return command, fmt.Errorf("resolve %s: unsupported shell quoting context", placeholder)
				}
				resolved.WriteString(quoted)
				if command[i] == '\'' {
					i += len(singleToken)
				} else {
					i += len(doubleToken)
				}
				continue
			}
		}
		if strings.HasPrefix(command[i:], placeholder) {
			if quote != 0 || (i > 0 && command[i-1] == '$') {
				return command, fmt.Errorf("resolve %s: unsupported shell quoting context", placeholder)
			}
			resolved.WriteString(quoted)
			i += len(placeholder)
			continue
		}
		character := command[i]
		resolved.WriteByte(character)
		switch quote {
		case 0:
			switch character {
			case '\'', '"', '`':
				quote = character
			case '\\':
				if i+1 < len(command) {
					if strings.HasPrefix(command[i+1:], placeholder) {
						return command, fmt.Errorf("resolve %s: escaped placeholders are unsupported", placeholder)
					}
					i++
					resolved.WriteByte(command[i])
				}
			}
		case '\'':
			if character == '\'' {
				quote = 0
			}
		case '"', '`':
			if character == '\\' && i+1 < len(command) {
				if strings.HasPrefix(command[i+1:], placeholder) {
					return command, fmt.Errorf("resolve %s: unsupported shell quoting context", placeholder)
				}
				i++
				resolved.WriteByte(command[i])
			} else if character == quote {
				quote = 0
			}
		}
		i++
	}
	return resolved.String(), nil
}

func normalizeVariables(variables []model.Variable, command, info string) []model.Variable {
	byName := map[string]model.Variable{}
	for _, variable := range variables {
		if !variableNamePattern.MatchString(variable.Name) {
			continue
		}
		placeholder := "{{" + variable.Name + "}}"
		if !strings.Contains(command, placeholder) && !strings.Contains(info, placeholder) {
			continue
		}
		if variable.Prompt == "" {
			variable.Prompt = variable.Name
		}
		byName[variable.Name] = variable
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]model.Variable, 0, len(names))
	for _, name := range names {
		result = append(result, byName[name])
	}
	return result
}

func cleanText(value string) string {
	value = strings.ReplaceAll(value, `\n`, " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.Trim(value, `" `)
}
