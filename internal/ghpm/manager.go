package ghpm

import (
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"ghpm/internal/config"
	"ghpm/internal/manifest"
	"ghpm/internal/ui"
)

type Manager struct {
	Config   config.Config
	Root     string
	HTTP     *http.Client
	lockFile *os.File
	Logger   ui.Logger
}

type InstallOptions struct {
	Version string
	Force   bool
	DryRun  bool
}

type RemoveOptions struct {
	Purge bool
}

func NewManager(cfg config.Config, root string) *Manager {
	timeout := cfg.HTTPTimeout()
	client := &http.Client{Timeout: timeout}
	return &Manager{
		Config: cfg,
		Root:   root,
		HTTP:   client,
		Logger: ui.NewLogger(ui.LevelNormal, os.Stderr),
	}
}

func (m *Manager) PackagesDir() string {
	return filepath.Join(m.Root, m.Config.PackagesDir)
}

func (m *Manager) StateDir() string {
	return filepath.Join(m.Root, m.Config.StateDir)
}

func (m *Manager) CacheDir() string {
	return filepath.Join(m.Root, m.Config.CacheDir)
}

func (m *Manager) lock() error {
	lockPath := filepath.Join(m.Root, "var/lock/ghpm.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return err
	}
	m.lockFile = f
	return nil
}

func (m *Manager) unlock() {
	if m.lockFile != nil {
		_ = syscall.Flock(int(m.lockFile.Fd()), syscall.LOCK_UN)
		_ = m.lockFile.Close()
		m.lockFile = nil
	}
}

func (m *Manager) ListManifests() ([]manifest.Manifest, error) {
	dir := m.PackagesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var manifests []manifest.Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "package.yaml")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		mf, err := manifest.Load(path)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, mf)
	}
	return manifests, nil
}

func (m *Manager) LoadManifest(name string) (manifest.Manifest, error) {
	path := filepath.Join(m.PackagesDir(), name, "package.yaml")
	return manifest.Load(path)
}
