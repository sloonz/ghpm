package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadUsesPackageDirectoryName(t *testing.T) {
	for _, content := range []string{
		"description: test\n",
		"name: wrong-package\n",
		"name: ''\n",
	} {
		t.Run(content, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "actual-package")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "package.yaml")
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			m, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if m.Name != "actual-package" {
				t.Fatalf("Name = %q", m.Name)
			}
			if got := ExpandTemplate("{name}", TemplateContext{Name: m.Name}); got != "actual-package" {
				t.Fatalf("template name = %q", got)
			}
			data, err := yaml.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := yaml.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["name"]; ok {
				t.Fatal("serialized manifest contains name")
			}
		})
	}
}
