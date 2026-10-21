package ghpm

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"ghpm/internal/config"
)

func TestSelfManifestUsesDirectoryName(t *testing.T) {
	m := NewManager(config.DefaultConfig(), t.TempDir())
	if err := m.ensureSelfManifest("/usr/local/bin/ghpm", 0755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(m.PackagesDir(), "ghpm", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := yaml.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["name"]; ok {
		t.Fatal("self manifest contains top-level name")
	}
	mf, err := m.LoadManifest("ghpm")
	if err != nil {
		t.Fatal(err)
	}
	if mf.Name != "ghpm" || mf.Install[0].Asset.Name != "ghpm_{version}_{os}_{arch}_bin" {
		t.Fatalf("unexpected self manifest: %+v", mf)
	}
}
