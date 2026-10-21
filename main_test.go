package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ghpm/internal/state"
)

func TestPackageCommands(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "ghpm")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	tests := []struct {
		name      string
		args      []string
		seed      bool
		want      []string
		wantError bool
		dry       bool
	}{
		{name: "install defaults to all", args: []string{"install"}, want: []string{"a", "b"}},
		{name: "install all", args: []string{"install", "--all"}, want: []string{"a", "b"}},
		{name: "install multiple", args: []string{"install", "a", "b"}, want: []string{"a", "b"}},
		{name: "install upgrades", args: []string{"install", "a", "--version", "v2"}, seed: true, want: []string{"a"}},
		{name: "upgrade defaults to installed", args: []string{"upgrade"}, seed: true, want: []string{"a"}},
		{name: "upgrade all", args: []string{"upgrade", "--all"}, seed: true, want: []string{"a"}},
		{name: "upgrade multiple", args: []string{"upgrade", "a", "b"}, wantError: true},
		{name: "upgrade missing", args: []string{"upgrade", "b"}, wantError: true},
		{name: "dry run missing", args: []string{"upgrade", "b", "--dry-run"}, wantError: true, dry: true},
		{name: "dry run all", args: []string{"upgrade", "--all", "--dry-run"}, seed: true, want: []string{"a"}, dry: true},
		{name: "upgrade empty", args: []string{"upgrade"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"a", "b"} {
				dir := filepath.Join(root, "var/lib/ghpm/packages", name)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				manifest := "install:\n  - type: mkdir\n    path: /opt/" + name + "\n"
				if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(manifest), 0644); err != nil {
					t.Fatal(err)
				}
			}
			run := func(args ...string) ([]byte, error) {
				return exec.Command(binary, append([]string{"--root", root, "--config", filepath.Join(root, "config.yaml")}, args...)...).CombinedOutput()
			}
			statePath := filepath.Join(root, "var/lib/ghpm/state/installed.json")
			if tt.seed {
				if out, err := run("install", "a", "--version", "v1"); err != nil {
					t.Fatalf("seed: %v: %s", err, out)
				}
			}
			before, _ := os.ReadFile(statePath)
			out, err := run(tt.args...)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v, output: %s", err, out)
			}
			if tt.wantError && !strings.Contains(string(out), "is not installed") {
				t.Fatalf("unexpected error: %s", out)
			}
			installed, err := state.LoadInstalled(statePath)
			if err != nil {
				t.Fatal(err)
			}
			if len(installed.Installed) != len(tt.want) {
				t.Fatalf("installed = %v, want %v", installed.Installed, tt.want)
			}
			for _, name := range tt.want {
				if _, ok := installed.Installed[name]; !ok {
					t.Fatalf("missing %s", name)
				}
			}
			if tt.name == "install upgrades" && installed.Installed["a"].Version != "v2" {
				t.Fatal("install did not upgrade")
			}
			if tt.dry {
				after, _ := os.ReadFile(statePath)
				if string(before) != string(after) {
					t.Fatal("dry run changed state")
				}
				if _, err := os.Stat(filepath.Join(root, "opt/b")); !os.IsNotExist(err) {
					t.Fatal("dry run installed b")
				}
			}
		})
	}
}
