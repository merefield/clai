package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsFrontmatterDiscoveryValidationAndActivation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "windows-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	crlf := string(data)
	if !strings.Contains(crlf, "\r\n") || strings.Contains(strings.ReplaceAll(crlf, "\r\n", ""), "\n") {
		t.Fatal("Windows fixture must retain CRLF line endings")
	}
	lf := strings.ReplaceAll(crlf, "\r\n", "\n")
	lfParts := strings.SplitN(lf, "\n---\n", 2)
	crlfParts := strings.SplitN(crlf, "\r\n---\r\n", 2)
	for _, test := range []struct{ name, text, body string }{
		{"LF", lf, lfParts[1]},
		{"CRLF", crlf, crlfParts[1]},
		{"CRLF with BOM", "\ufeff" + crlf, crlfParts[1]},
		{"CRLF opening only", strings.Replace(lf, "---\n", "---\r\n", 1), lfParts[1]},
		{"CRLF closing only", strings.Replace(lf, "\n---\n", "\n---\r\n", 1), lfParts[1]},
		{"CRLF closing at EOF", crlfParts[0] + "\r\n---", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			locations := fixture(t)
			path := filepath.Join(locations.Project, ".agents", "skills", "windows-skill", "SKILL.md")
			write(t, path, test.text)
			catalog := Discover(locations, true)
			if len(catalog.Skills) != 1 || len(catalog.Warnings) != 0 {
				t.Fatalf("discovery rejected line endings: %+v", catalog)
			}
			metadata, err := Validate(path)
			if err != nil || metadata != catalog.Skills[0].Metadata || metadata.Description != "Inspect Windows skill documents with portable workflows." {
				t.Fatalf("validation: %+v, %v", metadata, err)
			}
			active, _, err := catalog.Select("", []string{"windows-skill"}, Options{Enabled: true, FileMax: 24576})
			if err != nil || len(active) != 1 {
				t.Fatalf("activation: %+v, %v", active, err)
			}
			defer active[0].Close()
			if active[0].Body != test.body {
				t.Fatalf("body changed or retained frontmatter: %q", active[0].Body)
			}
		})
	}
}

func TestSingleWordSkillActivationRequiresWorkflowRelevance(t *testing.T) {
	locations := fixture(t)
	write(t, filepath.Join(locations.Project, ".agents", "skills", "terraform", "SKILL.md"), skillText("terraform", "Plan and validate Terraform infrastructure changes", "Terraform workflow"))
	catalog := Discover(locations, true)
	options := Options{Enabled: true, Auto: true, MaxActive: 2, FileMax: 24576}
	for _, test := range []struct {
		query    string
		explicit []string
		want     int
	}{
		{"terraform validate infrastructure changes", nil, 1},
		{"what is terraform", nil, 0},
		{"terraform", nil, 0},
		{"unrelated", []string{"terraform"}, 1},
	} {
		active, _, err := catalog.Select(test.query, test.explicit, options)
		for _, skill := range active {
			skill.Close()
		}
		if err != nil || len(active) != test.want {
			t.Fatalf("single-word name for %q, explicit %v: %d active, %v", test.query, test.explicit, len(active), err)
		}
	}
}
