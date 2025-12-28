//go:build integration
// +build integration

package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInstallRemoveRawBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix-like filesystem semantics")
	}

	root := t.TempDir()
	cfg := newTestLayout(t, root)
	writeManifest(t, cfg.packagesDir, "ghpm-raw", rawManifest())

	ghpm := buildBinary(t)

	runGHPM(t, ghpm, cfg, "install", "ghpm-raw")

	binPath := filepath.Join(root, "bin", "ghpm")
	assertExecutable(t, binPath)

	runGHPM(t, ghpm, cfg, "remove", "ghpm-raw")
	assertMissing(t, binPath)
}

func TestInstallRemoveArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration tests require unix-like filesystem semantics")
	}

	root := t.TempDir()
	cfg := newTestLayout(t, root)
	writeManifest(t, cfg.packagesDir, "ghpm-archive", archiveManifest())

	ghpm := buildBinary(t)

	runGHPM(t, ghpm, cfg, "install", "ghpm-archive")

	base := findArchiveBase(t, filepath.Join(root, "opt", "ghpm"))
	assertExecutable(t, filepath.Join(base, "ghpm"))
	assertFile(t, filepath.Join(base, "README.md"))
	assertFile(t, filepath.Join(base, "doc", "manifest-reference.md"))
	assertFile(t, filepath.Join(base, "examples", "k3s", "package.yaml"))
	assertOnlyFiles(t, base, []string{
		"ghpm",
		"README.md",
		"doc/manifest-reference.md",
		"examples/k3s/package.yaml",
	})

	runGHPM(t, ghpm, cfg, "remove", "ghpm-archive")
	assertMissing(t, filepath.Join(base, "ghpm"))
	assertMissing(t, filepath.Join(base, "README.md"))
	assertMissing(t, filepath.Join(base, "doc", "manifest-reference.md"))
	assertMissing(t, filepath.Join(base, "examples", "k3s", "package.yaml"))
}

type testLayout struct {
	root        string
	packagesDir string
	stateDir    string
	cacheDir    string
	configPath  string
}

func newTestLayout(t *testing.T, root string) testLayout {
	t.Helper()
	layout := testLayout{
		root:        root,
		packagesDir: filepath.Join(root, "var", "lib", "ghpm", "packages"),
		stateDir:    filepath.Join(root, "var", "lib", "ghpm", "state"),
		cacheDir:    filepath.Join(root, "var", "cache", "ghpm"),
		configPath:  filepath.Join(root, "config.yaml"),
	}
	for _, dir := range []string{layout.packagesDir, layout.stateDir, layout.cacheDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create test dir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(layout.configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return layout
}

func writeManifest(t *testing.T, packagesDir, name, content string) {
	t.Helper()
	pkgDir := filepath.Join(packagesDir, name)
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("create package dir: %v", err)
	}
	manifestPath := filepath.Join(pkgDir, "package.yaml")
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ghpm")
	root := repoRoot(t)

	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ghpm: %v\n%s", err, string(output))
	}
	return binPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	return filepath.Dir(filepath.Dir(file))
}

func runGHPM(t *testing.T, ghpm string, cfg testLayout, args ...string) {
	t.Helper()
	baseArgs := []string{
		"--root", cfg.root,
		"--packages-dir", "var/lib/ghpm/packages",
		"--state-dir", "var/lib/ghpm/state",
		"--cache-dir", "var/cache/ghpm",
		"--config", cfg.configPath,
	}
	cmd := exec.Command(ghpm, append(baseArgs, args...)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("ghpm %v failed: %v\n%s", args, err, buf.String())
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected executable at %s", path)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected file at %s", path)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to be removed", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func findArchiveBase(t *testing.T, root string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, "README.md")); err == nil {
		return root
	}
	matches, err := filepath.Glob(filepath.Join(root, "*", "README.md"))
	if err != nil {
		t.Fatalf("glob archive base: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one README.md under %s, got %d", root, len(matches))
	}
	return filepath.Dir(matches[0])
}

func assertOnlyFiles(t *testing.T, root string, expected []string) {
	t.Helper()
	expectedSet := map[string]struct{}{}
	for _, name := range expected {
		expectedSet[filepath.Clean(name)] = struct{}{}
	}
	found := map[string]struct{}{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		found[filepath.Clean(rel)] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("walk archive output: %v", err)
	}
	for name := range found {
		if _, ok := expectedSet[name]; !ok {
			t.Fatalf("unexpected file installed: %s", name)
		}
	}
	for name := range expectedSet {
		if _, ok := found[name]; !ok {
			t.Fatalf("expected file missing: %s", name)
		}
	}
}

func rawManifest() string {
	return `name: ghpm-raw
description: ghpm raw binary install test
source:
  kind: github
  repo: sloonz/ghpm
install:
  - type: asset
    pattern: "^ghpm_.*_{os}_{arch}_bin$"
    target: "bin/ghpm"
    mode: "0755"
`
}

func archiveManifest() string {
	return `name: ghpm-archive
description: ghpm archive install test
source:
  kind: github
  repo: sloonz/ghpm
install:
  - type: extract
    from:
      type: asset
      name: "ghpm_{version}_{os}_{arch}.tar.gz"
    targetDir: "opt/ghpm"
    pick:
      - "ghpm"
      - "README.md"
      - "doc/manifest-reference.md"
      - "examples/k3s/package.yaml"
`
}
