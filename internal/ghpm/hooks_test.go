package ghpm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPostInstallChmodUsesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root with spaces")
	target := filepath.Join(root, "usr/local/bin/wstunnel")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GHPM_ROOT", "/incorrect-inherited-root")
	m := &Manager{Root: root}
	m.runHooks([]string{`chmod 0755 "$GHPM_ROOT/usr/local/bin/wstunnel"`})
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("permissions = %04o, want 0755", got)
	}
}
