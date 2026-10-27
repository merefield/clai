package shellintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// Reuse the native test executable to inspect arguments on every platform,
	// including Windows where shell scripts are not native executables.
	if os.Getenv("_CLAI_TEST_NATIVE_ARGS") == "1" {
		json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func runPowerShell(t *testing.T, shell, script string, env ...string) string {
	t.Helper()
	command := exec.Command(shell, powerShellArgs(script)...)
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell failed: %v: %s", err, output)
	}
	return strings.ReplaceAll(string(output), "\r\n", "\n")
}

func TestPowerShellAliasForwardsNativeArgumentsAndExitStatus(t *testing.T) {
	for _, shell := range []string{"pwsh", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			requireShell(t, shell)
			source, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			executable := filepath.Join(t.TempDir(), "clai's préview $binary.exe")
			if err := os.WriteFile(executable, data, 0o700); err != nil {
				t.Fatal(err)
			}
			initialization, err := Init(shell, "computer", executable)
			if err != nil {
				t.Fatal(err)
			}
			wildcards := "* ?"
			if runtime.GOOS != "windows" {
				// PowerShell expands unquoted native wildcards on Unix, just as
				// it does when invoking clai by its original name.
				wildcards = "'*' '?'"
			}
			script := initialization + "\ncomputer show " + wildcards + " 'two words' --question a,b\nWrite-Output ('STATUS=' + $LASTEXITCODE)\n"
			output := runPowerShell(t, shell, script, "_CLAI_TEST_NATIVE_ARGS=1")
			lines := strings.Split(strings.TrimSpace(output), "\n")
			want, _ := json.Marshal([]string{"show", "*", "?", "two words", "--question", "a,b"})
			if len(lines) != 2 || lines[0] != string(want) || lines[1] != "STATUS=7" {
				t.Fatalf("alias changed native arguments or exit status: %q", output)
			}
		})
	}
}

func TestPowerShellChecksConflictsAndPreservesExistingCommands(t *testing.T) {
	for _, shell := range []string{"pwsh", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			requireShell(t, shell)
			profile := Profile{Shell: shell, Path: filepath.Join(t.TempDir(), "profile.ps1")}
			for _, definition := range []string{"Set-Alias -Name computer -Value Write-Output\n", "function Computer { 'other' }\n", ""} {
				if err := os.WriteFile(profile.Path, []byte(definition), 0o600); err != nil {
					t.Fatal(err)
				}
				err := profile.CheckConflict(context.Background(), "computer")
				if (err != nil) != (definition != "") {
					t.Fatalf("definition %q: error = %v", definition, err)
				}
			}
			for _, name := range []string{"cd", "get-item", "if", "clai"} {
				if err := profile.CheckConflict(context.Background(), name); err == nil {
					t.Errorf("accepted existing command or keyword %q", name)
				}
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			if err := profile.Save("computer", executable); err != nil {
				t.Fatal(err)
			}
			if err := profile.CheckConflict(context.Background(), "computer"); err != nil {
				t.Fatalf("managed alias blocked repeated setup: %v", err)
			}
			// A conflict introduced after setup must keep precedence on startup.
			script := "Set-Alias -Name computer -Value Write-Output\n" + profile.powerShellStartup() + "computer existing-command\n"
			if output := runPowerShell(t, shell, script); strings.TrimSpace(output) != "existing-command" {
				t.Fatalf("managed alias replaced an existing command: %q", output)
			}
		})
	}
}

func TestPowerShellProfilePreservesEncodingAndLineEndings(t *testing.T) {
	for _, encoding := range []textEncoding{utf8Text, utf8BOM, utf16LE, utf16BE} {
		t.Run(fmt.Sprint(encoding), func(t *testing.T) {
			profile := Profile{Shell: "powershell", Path: filepath.Join(t.TempDir(), "profile.ps1")}
			prefix := "# my préférences\r\n$env:EDITOR = 'vim'\r\n"
			if err := os.WriteFile(profile.Path, encoding.encode(prefix), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"computer", "helper", ""} {
				if err := profile.Save(name, `C:\Users\André\clai's folder\clai.exe`); err != nil {
					t.Fatal(err)
				}
				if current, err := profile.CurrentName(); err != nil || current != name {
					t.Fatalf("current name = %q, %v; want %q", current, err, name)
				}
				data, err := os.ReadFile(profile.Path)
				if err != nil {
					t.Fatal(err)
				}
				content, actualEncoding, err := decodeText(data)
				wantEncoding := encoding
				if wantEncoding == utf8Text {
					wantEncoding = utf8BOM
				}
				if err != nil || actualEncoding != wantEncoding || !strings.HasPrefix(content, prefix) || strings.Contains(strings.ReplaceAll(content, "\r\n", ""), "\n") {
					t.Fatalf("profile encoding or line endings changed: encoding %d, content %q, error %v", actualEncoding, content, err)
				}
				if name == "" && content != prefix {
					t.Fatalf("removing alias changed other settings: %q", content)
				}
			}
		})
	}
}

func TestDetectPowerShellProfile(t *testing.T) {
	for _, shell := range []string{"pwsh", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			requireShell(t, shell)
			t.Setenv("CLAI_SHELL", shell)
			profile, err := DetectProfile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !profile.isPowerShell() || !filepath.IsAbs(profile.Path) || len(profile.startup) != 4 || !strings.EqualFold(filepath.Base(profile.Path), "profile.ps1") {
				t.Fatalf("unexpected PowerShell profile: %+v", profile)
			}
		})
	}
}
