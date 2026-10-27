package config

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultText = `key=

hi_contrast=false
expose_current_dir=true
max_history_turns=10
api=https://api.openai.com/v1/chat/completions
model=gpt-4.1
json_mode=false
temp=0.1
tokens=500
reasoning=
system_one_key=
system_one_api=https://api.typesafe.ai/v1/systemone
system_one_model=jev-latest
use_tools=false
share_command_results=false
result_lines=20
confirm_dangerous_commands=true
risk_appetite=0
exec_query=
question_query=
error_query=
agents_enabled=true
agents_max_bytes=32768
agents_file_max_bytes=16384
skills_enabled=true
skills_auto_activate=true
skills_max_index_chars=8000
skills_max_active=2
skills_file_max_bytes=24576
skills_reference_max_bytes=16384
skills_quiet=false
`

type Config struct {
	Path                     string
	Key                      string
	HighContrast             bool
	ExposeCurrentDir         bool
	MaxHistoryTurns          int
	API                      string
	Model                    string
	JSONMode                 bool
	Temperature              float64
	Tokens                   int
	Reasoning                string
	SystemOneKey             string
	SystemOneAPI             string
	SystemOneModel           string
	UseTools                 bool
	ShareCommandResults      bool
	ResultLines              int
	ConfirmDangerousCommands bool
	RiskAppetite             int
	ExecQuery                string
	QuestionQuery            string
	ErrorQuery               string
	AgentsEnabled            bool
	AgentsMaxBytes           int
	AgentsFileMaxBytes       int
	SkillsEnabled            bool
	SkillsAutoActivate       bool
	SkillsMaxIndexChars      int
	SkillsMaxActive          int
	SkillsFileMaxBytes       int
	SkillsReferenceMaxBytes  int
	SkillsQuiet              bool
	values                   map[string]string
	order                    []string
}

func DefaultPath() (string, error) {
	if path := os.Getenv("CLAI_CONFIG"); path != "" {
		return path, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clai.cfg"), nil
}

func Ensure(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("config path %q must be a regular file", path)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure config permissions: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, []byte(defaultText), 0o600)
}

func Load(path string) (*Config, error) {
	if err := Ensure(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := map[string]string{}
	var order []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	if err := s.Err(); err != nil {
		return nil, err
	}

	c := &Config{Path: path, values: values, order: order}
	c.refresh()
	return c, nil
}

func (c *Config) refresh() {
	c.Key = c.values["key"]
	c.HighContrast = boolValue(c.values["hi_contrast"], false)
	c.ExposeCurrentDir = boolValue(c.values["expose_current_dir"], true)
	c.MaxHistoryTurns = intValue(c.values["max_history_turns"], 10, 1)
	c.API = stringValue(c.values["api"], "https://api.openai.com/v1/chat/completions")
	c.Model = stringValue(c.values["model"], "gpt-4.1")
	c.JSONMode = boolValue(c.values["json_mode"], false)
	c.Temperature = floatValue(c.values["temp"], 0.1)
	c.Tokens = intValue(c.values["tokens"], 500, 1)
	c.Reasoning = c.values["reasoning"]
	c.SystemOneKey = c.values["system_one_key"]
	c.SystemOneAPI = stringValue(c.values["system_one_api"], "https://api.typesafe.ai/v1/systemone")
	c.SystemOneModel = stringValue(c.values["system_one_model"], "jev-latest")
	c.UseTools = boolValue(c.values["use_tools"], false)
	c.ShareCommandResults = boolValue(c.values["share_command_results"], false)
	c.ResultLines = intValue(c.values["result_lines"], 20, 1)
	c.ConfirmDangerousCommands = boolValue(c.values["confirm_dangerous_commands"], true)
	c.RiskAppetite = intValue(c.values["risk_appetite"], 0, 0)
	if c.RiskAppetite < 0 || c.RiskAppetite > 2 {
		c.RiskAppetite = 0
	}
	c.ExecQuery = c.values["exec_query"]
	c.QuestionQuery = c.values["question_query"]
	c.ErrorQuery = c.values["error_query"]
	c.AgentsEnabled = boolValue(c.values["agents_enabled"], true)
	c.AgentsMaxBytes = min(4<<20, intValue(c.values["agents_max_bytes"], 32768, 1))
	c.AgentsFileMaxBytes = min(1<<20, intValue(c.values["agents_file_max_bytes"], 16384, 1))
	c.SkillsEnabled = boolValue(c.values["skills_enabled"], true)
	c.SkillsAutoActivate = boolValue(c.values["skills_auto_activate"], true)
	c.SkillsMaxIndexChars = min(65536, intValue(c.values["skills_max_index_chars"], 8000, 1))
	c.SkillsMaxActive = min(8, intValue(c.values["skills_max_active"], 2, 0))
	c.SkillsFileMaxBytes = min(1<<20, intValue(c.values["skills_file_max_bytes"], 24576, 1))
	c.SkillsReferenceMaxBytes = min(16384, intValue(c.values["skills_reference_max_bytes"], 16384, 1))
	c.SkillsQuiet = boolValue(c.values["skills_quiet"], false)
}

func (c *Config) Set(key, value string) {
	if _, exists := c.values[key]; !exists {
		c.order = append(c.order, key)
	}
	c.values[key] = value
	c.refresh()
}

func (c *Config) Save() error {
	seen := map[string]bool{}
	var b strings.Builder
	for _, key := range c.order {
		if seen[key] {
			continue
		}
		seen[key] = true
		fmt.Fprintf(&b, "%s=%s\n", key, c.values[key])
	}
	for _, key := range defaultKeys() {
		if !seen[key] {
			fmt.Fprintf(&b, "%s=%s\n", key, c.values[key])
		}
	}
	return atomicWrite(c.Path, []byte(b.String()), 0o600)
}

func defaultKeys() []string {
	keys := []string{}
	for _, line := range strings.Split(defaultText, "\n") {
		if key, _, ok := strings.Cut(line, "="); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func boolValue(value string, fallback bool) bool {
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	return fallback
}

func stringValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func intValue(value string, fallback, minimum int) int {
	n, err := strconv.Atoi(value)
	if err != nil || n < minimum {
		return fallback
	}
	return n
}

func floatValue(value string, fallback float64) float64 {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return fallback
	}
	return n
}
