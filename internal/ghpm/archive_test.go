package ghpm

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"

	"ghpm/internal/config"
	"ghpm/internal/state"
)

func TestInstallRejectsEmptyExtraction(t *testing.T) {
	for _, format := range []string{"tar.gz", "tar.xz", "zip"} {
		for _, tc := range []struct {
			name, options string
			entries       []string
			wantError     bool
		}{
			{"empty archive", "", nil, true},
			{"directories only", "", []string{"dir/"}, true},
			{"stripped away", "    stripComponents: 1\n", []string{"tool"}, true},
			{"pick misses", "    pick: [missing]\n", []string{"tool"}, true},
			{"omit all", "    omit: ['*']\n", []string{"tool"}, true},
			{"selected file", "    stripComponents: 1\n    pick: [tool]\n", []string{"dir/tool"}, false},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				m := NewManager(config.DefaultConfig(), root)
				dir := filepath.Join(m.PackagesDir(), "test")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				archive := "archive." + format
				writeTestArchive(t, filepath.Join(dir, archive), format, tc.entries)
				// An earlier action must not run if a later extraction is invalid.
				yaml := fmt.Sprintf("name: test\ninstall:\n  - type: mkdir\n    path: /earlier\n  - type: extract\n    from:\n      type: file\n      path: %s\n    targetDir: /out\n%s", archive, tc.options)
				if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(yaml), 0644); err != nil {
					t.Fatal(err)
				}
				_, err := m.Install("test", InstallOptions{})
				if !tc.wantError {
					if err != nil {
						t.Fatal(err)
					}
					if data, err := os.ReadFile(filepath.Join(root, "out/tool")); err != nil || string(data) != "payload" {
						t.Fatalf("extracted file: %q, %v", data, err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "no files selected") || !strings.Contains(err.Error(), archive) {
					t.Fatalf("unexpected error: %v", err)
				}
				for _, path := range []string{filepath.Join(root, "earlier"), filepath.Join(root, "out"), state.InstalledPath(m.StateDir()), state.ReceiptPath(m.StateDir(), "test")} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("unexpected side effect at %s: %v", path, err)
					}
				}
			})
		}
	}
}

func writeTestArchive(t *testing.T, path, format string, entries []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if format == "zip" {
		w := zip.NewWriter(f)
		for _, name := range entries {
			out, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(name, "/") {
				if _, err := io.WriteString(out, "payload"); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	var compressed io.WriteCloser
	if format == "tar.gz" {
		compressed = gzip.NewWriter(f)
	} else {
		compressed, err = xz.NewWriter(f)
		if err != nil {
			t.Fatal(err)
		}
	}
	w := tar.NewWriter(compressed)
	for _, name := range entries {
		h := &tar.Header{Name: name, Mode: 0755, Size: 7, Typeflag: tar.TypeReg}
		if strings.HasSuffix(name, "/") {
			h.Typeflag = tar.TypeDir
			h.Size = 0
		}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := io.WriteString(w, "payload"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
}
