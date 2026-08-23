package app

import (
	"strings"
	"testing"
)

func TestCurrentVersionUsesMaintainedSourceVersion(t *testing.T) {
	previous := buildVersion
	buildVersion = ""
	t.Cleanup(func() { buildVersion = previous })

	if actual, expected := CurrentVersion(), strings.TrimSpace(sourceVersion); actual != expected {
		t.Fatalf("CurrentVersion() = %q, want %q", actual, expected)
	}
}

func TestCurrentVersionUsesBuildOverride(t *testing.T) {
	previous := buildVersion
	buildVersion = "v9.8.7+fixture"
	t.Cleanup(func() { buildVersion = previous })

	if actual := CurrentVersion(); actual != "9.8.7+fixture" {
		t.Fatalf("CurrentVersion() = %q, want 9.8.7+fixture", actual)
	}
}
