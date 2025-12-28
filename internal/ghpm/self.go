package ghpm

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"ghpm/internal/state"
)

type SelfOptions struct {
	Version string
}

func (m *Manager) Self(opts SelfOptions) (state.Receipt, error) {
	if err := m.lock(); err != nil {
		return state.Receipt{}, err
	}
	defer m.unlock()

	if err := m.Config.EnsureDirs(m.Root); err != nil {
		return state.Receipt{}, err
	}

	execPath, err := os.Executable()
	if err != nil {
		return state.Receipt{}, err
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return state.Receipt{}, err
	}
	if !filepath.IsAbs(execPath) {
		execPath, err = filepath.Abs(execPath)
		if err != nil {
			return state.Receipt{}, err
		}
	}

	info, err := os.Stat(execPath)
	if err != nil {
		return state.Receipt{}, err
	}
	perm := info.Mode().Perm()

	version := opts.Version
	if version == "" {
		version = BuildInfo().Version
	}

	if err := m.ensureSelfManifest(execPath, perm); err != nil {
		return state.Receipt{}, err
	}

	sum, err := hashFile(execPath)
	if err != nil {
		return state.Receipt{}, err
	}

	receipt := state.Receipt{
		Schema:    1,
		Name:      "ghpm",
		Source:    state.ReceiptSource{Kind: "github", Repo: "sloonz/ghpm", Tag: version},
		Platform:  state.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Artifacts: nil,
		Files: []state.ReceiptFile{
			{
				Path:   normalizePathForReceipt(m.Root, execPath),
				Type:   "file",
				Mode:   int(perm),
				SHA256: sum,
			},
		},
	}

	receiptPath := state.ReceiptPath(m.StateDir(), "ghpm")
	if err := state.SaveReceipt(receiptPath, receipt); err != nil {
		return state.Receipt{}, err
	}
	if _, err := state.RecordInstall(m.StateDir(), "ghpm", version); err != nil {
		return state.Receipt{}, err
	}
	return receipt, nil
}

func (m *Manager) ensureSelfManifest(execPath string, perm os.FileMode) error {
	pkgDir := filepath.Join(m.PackagesDir(), "ghpm")
	manifestPath := filepath.Join(pkgDir, "package.yaml")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return err
	}

	manifest := fmt.Sprintf(`name: ghpm
description: ghpm package manager
source:
  kind: github
  repo: sloonz/ghpm
install:
  - type: asset
    name: "ghpm_{version}_{os}_{arch}_bin"
    target: "%s"
    mode: "%#o"
`, execPath, perm.Perm())

	return os.WriteFile(manifestPath, []byte(manifest), 0o644)
}
