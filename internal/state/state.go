package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type InstalledState struct {
	Schema    int                       `json:"schema"`
	Installed map[string]InstalledEntry `json:"installed"`
}

type InstalledEntry struct {
	Version     string `json:"version"`
	Receipt     string `json:"receipt"`
	InstalledAt string `json:"installedAt"`
}

type Receipt struct {
	Schema    int           `json:"schema"`
	Name      string        `json:"name"`
	Source    ReceiptSource `json:"source"`
	Platform  Platform      `json:"platform"`
	Artifacts []Artifact    `json:"artifacts"`
	Files     []ReceiptFile `json:"files"`
}

type ReceiptSource struct {
	Kind      string `json:"kind"`
	Repo      string `json:"repo,omitempty"`
	Tag       string `json:"tag,omitempty"`
	ReleaseID int64  `json:"releaseId,omitempty"`
}

type Platform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type Artifact struct {
	Type   string `json:"type"`
	Name   string `json:"name,omitempty"`
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

type ReceiptFile struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Mode     int    `json:"mode,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	To       string `json:"to,omitempty"`
	Preserve bool   `json:"preserve,omitempty"`
}

func LoadInstalled(path string) (InstalledState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstalledState{Schema: 1, Installed: map[string]InstalledEntry{}}, nil
		}
		return InstalledState{}, err
	}
	var s InstalledState
	if err := json.Unmarshal(data, &s); err != nil {
		return InstalledState{}, err
	}
	if s.Installed == nil {
		s.Installed = map[string]InstalledEntry{}
	}
	if s.Schema == 0 {
		s.Schema = 1
	}
	return s, nil
}

func SaveInstalled(path string, s InstalledState) error {
	if s.Schema == 0 {
		s.Schema = 1
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadReceipt(path string) (Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return Receipt{}, err
	}
	if r.Schema == 0 {
		r.Schema = 1
	}
	return r, nil
}

func SaveReceipt(path string, r Receipt) error {
	if r.Schema == 0 {
		r.Schema = 1
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ReceiptPath(stateDir, name string) string {
	return filepath.Join(stateDir, "receipts", name+".json")
}

func InstalledPath(stateDir string) string {
	return filepath.Join(stateDir, "installed.json")
}

func RecordInstall(stateDir, name, version string) (InstalledState, error) {
	installedPath := InstalledPath(stateDir)
	installed, err := LoadInstalled(installedPath)
	if err != nil {
		return InstalledState{}, err
	}
	installed.Installed[name] = InstalledEntry{
		Version:     version,
		Receipt:     filepath.ToSlash(filepath.Join("receipts", name+".json")),
		InstalledAt: time.Now().Format(time.RFC3339),
	}
	return installed, SaveInstalled(installedPath, installed)
}

func RecordRemove(stateDir, name string) error {
	installedPath := InstalledPath(stateDir)
	installed, err := LoadInstalled(installedPath)
	if err != nil {
		return err
	}
	delete(installed.Installed, name)
	return SaveInstalled(installedPath, installed)
}
