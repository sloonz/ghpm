package ghpm

import (
	"fmt"
	"os"
	"path/filepath"

	"ghpm/internal/manifest"
	"ghpm/internal/source"
	"ghpm/internal/state"
)

type plan struct {
	steps        []func() error
	targets      []string
	receiptFiles *[]state.ReceiptFile
}

func (m *Manager) buildPlan(mf manifest.Manifest, release source.Release, ctx manifest.TemplateContext, workDir string) (plan, []state.Artifact, error) {
	receiptFiles := []state.ReceiptFile{}
	pl := plan{receiptFiles: &receiptFiles}
	var artifacts []state.Artifact
	for _, act := range mf.Install {
		switch act.Type {
		case "mkdir":
			action := *act.Mkdir
			target := filepath.Join(m.Root, manifest.ExpandTemplate(action.Path, ctx))
			pl.targets = append(pl.targets, target)
			pl.steps = append(pl.steps, func() error {
				m.Logger.Verbosef("mkdir %s", target)
				return os.MkdirAll(target, 0o755)
			})
			*pl.receiptFiles = append(*pl.receiptFiles, state.ReceiptFile{
				Path: manifest.ExpandTemplate(action.Path, ctx),
				Type: "dir",
				Mode: parseMode(action.Mode),
			})
		case "symlink":
			action := *act.Symlink
			target := filepath.Join(m.Root, manifest.ExpandTemplate(action.Target, ctx))
			to := manifest.ExpandTemplate(action.To, ctx)
			pl.targets = append(pl.targets, target)
			pl.steps = append(pl.steps, func() error {
				m.Logger.Verbosef("symlink %s -> %s", target, to)
				return createSymlinkAtomic(target, to)
			})
			*pl.receiptFiles = append(*pl.receiptFiles, state.ReceiptFile{
				Path: manifest.ExpandTemplate(action.Target, ctx),
				Type: "symlink",
				To:   to,
			})
		case "file":
			action := *act.File
			src := filepath.Join(mf.PackageDir(), action.Path)
			target := filepath.Join(m.Root, manifest.ExpandTemplate(action.Target, ctx))
			pl.targets = append(pl.targets, target)
			pl.steps = append(pl.steps, func() error {
				m.Logger.Verbosef("install file %s -> %s", src, target)
				return installFileAtomic(target, src, parseMode(action.Mode))
			})
			sum, size, err := hashFileWithSize(src)
			if err != nil {
				return plan{}, nil, err
			}
			*pl.receiptFiles = append(*pl.receiptFiles, state.ReceiptFile{
				Path:     manifest.ExpandTemplate(action.Target, ctx),
				Type:     "file",
				Mode:     parseMode(action.Mode),
				SHA256:   sum,
				Preserve: action.Preserve,
			})
			artifacts = append(artifacts, state.Artifact{
				Type:   "file",
				Name:   action.Path,
				SHA256: sum,
				Size:   size,
			})
		case "url":
			action := *act.URL
			urlStr := manifest.ExpandTemplate(action.URL, ctx)
			target := filepath.Join(m.Root, manifest.ExpandTemplate(action.Target, ctx))
			m.Logger.Infof("download %s", urlStr)
			localPath, sum, size, _, err := m.fetchURL(urlStr)
			if err != nil {
				return plan{}, nil, err
			}
			pl.targets = append(pl.targets, target)
			pl.steps = append(pl.steps, func() error {
				m.Logger.Verbosef("install url -> %s", target)
				return installFileAtomic(target, localPath, parseMode(action.Mode))
			})
			*pl.receiptFiles = append(*pl.receiptFiles, state.ReceiptFile{
				Path:     manifest.ExpandTemplate(action.Target, ctx),
				Type:     "file",
				Mode:     parseMode(action.Mode),
				SHA256:   sum,
				Preserve: action.Preserve,
			})
			artifacts = append(artifacts, state.Artifact{
				Type:   "url",
				URL:    urlStr,
				SHA256: sum,
				Size:   size,
			})
		case "asset":
			action := *act.Asset
			asset, err := source.SelectAsset(release, action)
			if err != nil {
				return plan{}, nil, err
			}
			target := filepath.Join(m.Root, manifest.ExpandTemplate(action.Target, ctx))
			m.Logger.Infof("download %s %s", asset.Name, asset.URL)
			localPath, sum, size, _, err := m.fetchURL(asset.URL)
			if err != nil {
				return plan{}, nil, err
			}
			pl.targets = append(pl.targets, target)
			pl.steps = append(pl.steps, func() error {
				m.Logger.Verbosef("install asset %s -> %s", asset.Name, target)
				return installFileAtomic(target, localPath, parseMode(action.Mode))
			})
			*pl.receiptFiles = append(*pl.receiptFiles, state.ReceiptFile{
				Path:     manifest.ExpandTemplate(action.Target, ctx),
				Type:     "file",
				Mode:     parseMode(action.Mode),
				SHA256:   sum,
				Preserve: action.Preserve,
			})
			artifacts = append(artifacts, state.Artifact{
				Type:   "asset",
				Name:   asset.Name,
				URL:    asset.URL,
				SHA256: sum,
				Size:   size,
			})
		case "extract":
			action := *act.Extract
			installAction, archiveName, skipped, err := m.buildExtractPlan(mf, action, ctx, workDir, pl.receiptFiles)
			if err != nil {
				return plan{}, nil, err
			}
			targetDir := manifest.ExpandTemplate(action.TargetDir, ctx)
			m.Logger.Infof("extract %s -> %s", archiveName, targetDir)
			for _, target := range installAction.targets {
				m.Logger.Verbosef("extract %s", target)
			}
			for _, name := range skipped {
				m.Logger.Verbosef("skip %s", name)
			}
			pl.targets = append(pl.targets, installAction.targets...)
			pl.steps = append(pl.steps, installAction.steps...)
		default:
			return plan{}, nil, fmt.Errorf("unsupported action %s", act.Type)
		}
	}
	return pl, artifacts, nil
}

func (m *Manager) buildExtractPlan(mf manifest.Manifest, action manifest.ExtractAction, ctx manifest.TemplateContext, workDir string, receiptFiles *[]state.ReceiptFile) (plan, string, []string, error) {
	pl := plan{receiptFiles: receiptFiles}
	sourcePath := ""
	hintName := ""
	switch action.From.Type {
	case "asset":
		assetAction := manifest.AssetAction{
			Name:    manifest.ExpandTemplate(action.From.Name, ctx),
			Pattern: manifest.ExpandTemplate(action.From.Pattern, ctx),
		}
		resolver, err := source.NewResolver(mf.Source.Kind, m.HTTP)
		if err != nil {
			return plan{}, "", nil, err
		}
		release, err := resolver.ResolveRelease(mf.Source.Repo, ctx.Tag)
		if err != nil {
			return plan{}, "", nil, err
		}
		asset, err := source.SelectAsset(release, assetAction)
		if err != nil {
			return plan{}, "", nil, err
		}
		m.Logger.Infof("download %s %s", asset.Name, asset.URL)
		local, _, _, hint, err := m.fetchURL(asset.URL)
		if err != nil {
			return plan{}, "", nil, err
		}
		sourcePath = local
		hintName = hint
	case "url":
		urlStr := manifest.ExpandTemplate(action.From.URL, ctx)
		m.Logger.Infof("download %s", urlStr)
		local, _, _, hint, err := m.fetchURL(urlStr)
		if err != nil {
			return plan{}, "", nil, err
		}
		sourcePath = local
		hintName = hint
	case "file":
		sourcePath = filepath.Join(mf.PackageDir(), manifest.ExpandTemplate(action.From.Path, ctx))
		hintName = filepath.Base(sourcePath)
	default:
		return plan{}, "", nil, fmt.Errorf("extract.from.type %q is not supported", action.From.Type)
	}
	targetDir := filepath.Join(m.Root, manifest.ExpandTemplate(action.TargetDir, ctx))
	files, skipped, err := listArchiveFiles(sourcePath, hintName, action)
	if err != nil {
		return plan{}, "", nil, err
	}
	for _, name := range files {
		target := filepath.Join(targetDir, name)
		pl.targets = append(pl.targets, target)
	}
	archiveName := hintName
	if archiveName == "" {
		archiveName = filepath.Base(sourcePath)
	}
	pl.steps = append(pl.steps, func() error {
		return extractArchive(sourcePath, hintName, workDir, targetDir, action)
	})
	pl.steps = append(pl.steps, func() error {
		return recordExtractedList(m.Root, targetDir, files, receiptFiles)
	})
	return pl, archiveName, skipped, nil
}
