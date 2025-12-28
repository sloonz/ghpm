package ghpm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"ghpm/internal/manifest"
	"ghpm/internal/source"
	"ghpm/internal/state"
)

func (m *Manager) Install(name string, opts InstallOptions) (state.Receipt, error) {
	if err := m.lock(); err != nil {
		return state.Receipt{}, err
	}
	defer m.unlock()

	if err := m.Config.EnsureDirs(m.Root); err != nil {
		return state.Receipt{}, err
	}

	mf, err := m.LoadManifest(name)
	if err != nil {
		return state.Receipt{}, err
	}
	m.Logger.Infof("install %s", mf.Name)

	installed, err := state.LoadInstalled(state.InstalledPath(m.StateDir()))
	if err != nil {
		return state.Receipt{}, err
	}
	if entry, ok := installed.Installed[mf.Name]; ok && !opts.Force {
		resolved, _, err := m.resolveVersion(mf, opts.Version)
		if err != nil {
			return state.Receipt{}, err
		}
		if resolved != "" && resolved == entry.Version {
			receiptPath := state.ReceiptPath(m.StateDir(), mf.Name)
			if receipt, err := state.LoadReceipt(receiptPath); err == nil {
				m.Logger.Infof("already installed %s %s", mf.Name, resolved)
				return receipt, nil
			}
		}
	}

	var previousReceipt *state.Receipt
	receiptPath := state.ReceiptPath(m.StateDir(), mf.Name)
	if existing, err := state.LoadReceipt(receiptPath); err == nil {
		previousReceipt = &existing
	}

	ownership, err := m.buildOwnership()
	if err != nil {
		return state.Receipt{}, err
	}

	platform := state.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	resolved, release, err := m.resolveVersion(mf, opts.Version)
	if err != nil {
		return state.Receipt{}, err
	}
	if resolved != "" {
		m.Logger.Infof("resolved %s", resolved)
	}

	ctx := manifest.TemplateContext{
		Version: resolved,
		Tag:     resolved,
		OS:      platform.OS,
		Arch:    platform.Arch,
		Repo:    mf.Source.Repo,
		Name:    mf.Name,
	}

	workDir, err := os.MkdirTemp(filepath.Join(m.StateDir(), "work"), mf.Name+"-")
	if err != nil {
		return state.Receipt{}, err
	}
	defer os.RemoveAll(workDir)

	plan, artifacts, err := m.buildPlan(mf, release, ctx, workDir)
	if err != nil {
		return state.Receipt{}, err
	}

	conflicts := m.checkConflicts(plan.targets, ownership, name, opts.Force)
	if len(conflicts) > 0 {
		return state.Receipt{}, fmt.Errorf("install conflicts: %s", strings.Join(conflicts, ", "))
	}

	if opts.DryRun {
		m.Logger.Infof("dry-run complete")
		return state.Receipt{}, nil
	}

	receipt := state.Receipt{
		Schema:    1,
		Name:      mf.Name,
		Source:    state.ReceiptSource{Kind: mf.Source.Kind, Repo: mf.Source.Repo, Tag: resolved, ReleaseID: release.ID},
		Platform:  platform,
		Artifacts: artifacts,
	}

	for _, step := range plan.steps {
		if err := step(); err != nil {
			return state.Receipt{}, err
		}
	}

	if plan.receiptFiles != nil {
		receipt.Files = append(receipt.Files, *plan.receiptFiles...)
	}

	if err := state.SaveReceipt(receiptPath, receipt); err != nil {
		return state.Receipt{}, err
	}
	if _, err := state.RecordInstall(m.StateDir(), mf.Name, resolved); err != nil {
		return state.Receipt{}, err
	}
	if previousReceipt != nil {
		_ = removeObsoleteFiles(m.Root, previousReceipt, &receipt)
	}

	m.runHooks(mf.PostInstall)
	return receipt, nil
}

func (m *Manager) Remove(name string, opts RemoveOptions) error {
	if err := m.lock(); err != nil {
		return err
	}
	defer m.unlock()

	receiptPath := state.ReceiptPath(m.StateDir(), name)
	receipt, err := state.LoadReceipt(receiptPath)
	if err != nil {
		return err
	}
	for _, f := range receipt.Files {
		if f.Preserve && !opts.Purge {
			m.Logger.Verbosef("skip %s", filepath.Join(m.Root, f.Path))
			continue
		}
		target := filepath.Join(m.Root, f.Path)
		switch f.Type {
		case "file", "symlink":
			m.Logger.Verbosef("remove %s", target)
			_ = os.Remove(target)
		case "dir":
			m.Logger.Verbosef("remove %s", target)
			_ = os.Remove(target)
		}
	}
	if err := os.Remove(receiptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := state.RecordRemove(m.StateDir(), name); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Status(name string) (state.Receipt, map[string]bool, error) {
	receiptPath := state.ReceiptPath(m.StateDir(), name)
	receipt, err := state.LoadReceipt(receiptPath)
	if err != nil {
		return state.Receipt{}, nil, err
	}
	status := map[string]bool{}
	for _, f := range receipt.Files {
		target := filepath.Join(m.Root, f.Path)
		ok := false
		switch f.Type {
		case "file":
			sum, err := hashFile(target)
			if err == nil && sum == f.SHA256 {
				ok = true
			}
		case "symlink":
			dest, err := os.Readlink(target)
			if err == nil && dest == f.To {
				ok = true
			}
		case "dir":
			info, err := os.Stat(target)
			if err == nil && info.IsDir() {
				ok = true
			}
		}
		status[f.Path] = ok
	}
	return receipt, status, nil
}

func (m *Manager) Upgrade(name string, opts InstallOptions) (bool, state.Receipt, error) {
	installed, err := state.LoadInstalled(state.InstalledPath(m.StateDir()))
	if err != nil {
		return false, state.Receipt{}, err
	}
	entry, ok := installed.Installed[name]
	if !ok {
		receipt, err := m.Install(name, opts)
		return true, receipt, err
	}
	if opts.DryRun {
		mf, err := m.LoadManifest(name)
		if err != nil {
			return false, state.Receipt{}, err
		}
		resolved, _, err := m.resolveVersion(mf, "")
		if err != nil {
			return false, state.Receipt{}, err
		}
		receipt := state.Receipt{Name: name, Source: state.ReceiptSource{Tag: resolved}}
		return resolved != entry.Version, receipt, nil
	}
	opts.Version = ""
	receipt, err := m.Install(name, opts)
	if err != nil {
		return false, state.Receipt{}, err
	}
	if receipt.Source.Tag == entry.Version {
		return false, receipt, nil
	}
	return true, receipt, nil
}

func (m *Manager) resolveVersion(mf manifest.Manifest, version string) (string, source.Release, error) {
	if mf.Source.Kind == "" {
		return version, source.Release{}, nil
	}
	if mf.Source.Kind == "http" && version == "" {
		return "", source.Release{}, nil
	}
	resolver, err := source.NewResolver(mf.Source.Kind, m.HTTP)
	if err != nil {
		return "", source.Release{}, err
	}
	release, err := resolver.ResolveRelease(mf.Source.Repo, version)
	if err != nil {
		return "", source.Release{}, err
	}
	return release.Tag, release, nil
}

func (m *Manager) fetchURL(urlStr string) (string, string, int64, string, error) {
	cacheDir := filepath.Join(m.CacheDir(), "downloads")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", 0, "", err
	}
	key := sha256.Sum256([]byte(urlStr))
	name := hex.EncodeToString(key[:])
	hintName := cacheHintName(urlStr)
	cacheName := name
	if hintName != "" {
		cacheName = name + "-" + hintName
	}
	path := filepath.Join(cacheDir, cacheName)
	if _, err := os.Stat(path); err == nil {
		sum, size, err := hashFileWithSize(path)
		return path, sum, size, hintName, err
	}
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return "", "", 0, "", err
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return "", "", 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, "", fmt.Errorf("download %s: %s", urlStr, resp.Status)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", "", 0, "", err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, hash), resp.Body)
	if err != nil {
		return "", "", 0, "", err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if err := f.Sync(); err != nil {
		return "", "", 0, "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", "", 0, "", err
	}
	return path, sum, size, hintName, nil
}

func (m *Manager) buildOwnership() (map[string]string, error) {
	installed, err := state.LoadInstalled(state.InstalledPath(m.StateDir()))
	if err != nil {
		return nil, err
	}
	ownership := map[string]string{}
	for name, entry := range installed.Installed {
		receiptPath := filepath.Join(m.StateDir(), entry.Receipt)
		receipt, err := state.LoadReceipt(receiptPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, f := range receipt.Files {
			ownership[f.Path] = name
		}
	}
	return ownership, nil
}

func (m *Manager) checkConflicts(targets []string, ownership map[string]string, pkg string, force bool) []string {
	var conflicts []string
	for _, target := range targets {
		relative := normalizePathForReceipt(m.Root, target)
		if owner, ok := ownership[relative]; ok && owner != pkg {
			conflicts = append(conflicts, relative)
			continue
		}
		if _, err := os.Stat(target); err == nil {
			if !force && !okOwned(ownership, relative, pkg) {
				conflicts = append(conflicts, relative)
			}
		}
	}
	return conflicts
}

func okOwned(ownership map[string]string, path, pkg string) bool {
	owner, ok := ownership[path]
	return ok && owner == pkg
}

func cacheHintName(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}
	base := path.Base(parsed.Path)
	if base == "." || base == "/" {
		return ""
	}
	return sanitizeFilename(base)
}

func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func installFileAtomic(target, source string, mode int) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	temp := target + ".ghpm.new"
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(temp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if mode != 0 {
		if err := os.Chmod(temp, fs.FileMode(mode)); err != nil {
			return err
		}
	}
	backup := target + ".ghpm.bak"
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(temp, target); err != nil {
		if _, err := os.Stat(backup); err == nil {
			_ = os.Rename(backup, target)
		}
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func createSymlinkAtomic(target, to string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".ghpm.new"
	_ = os.Remove(tmp)
	if err := os.Symlink(to, tmp); err != nil {
		return err
	}
	backup := target + ".ghpm.bak"
	if _, err := os.Lstat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		if _, err := os.Stat(backup); err == nil {
			_ = os.Rename(backup, target)
		}
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func parseMode(val string) int {
	if val == "" {
		return 0
	}
	var mode int
	_, _ = fmt.Sscanf(val, "%o", &mode)
	return mode
}

func hashFile(path string) (string, error) {
	sum, _, err := hashFileWithSize(path)
	return sum, err
}

func hashFileWithSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (m *Manager) runHooks(commands []string) {
	for _, cmd := range commands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		c := exec.Command("/bin/sh", "-c", cmd)
		_ = c.Run()
	}
}

func normalizePathForReceipt(root, target string) string {
	if root == "/" {
		return target
	}
	trimmed := strings.TrimPrefix(target, root)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

func removeObsoleteFiles(root string, oldReceipt *state.Receipt, newReceipt *state.Receipt) error {
	current := map[string]state.ReceiptFile{}
	for _, f := range newReceipt.Files {
		current[f.Path] = f
	}
	for _, old := range oldReceipt.Files {
		if _, ok := current[old.Path]; ok {
			continue
		}
		if old.Preserve {
			continue
		}
		target := filepath.Join(root, old.Path)
		switch old.Type {
		case "file", "symlink":
			_ = os.Remove(target)
		case "dir":
			_ = os.Remove(target)
		}
	}
	return nil
}
