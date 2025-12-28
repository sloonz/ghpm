package ghpm

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"

	"ghpm/internal/manifest"
	"ghpm/internal/state"
)

func extractArchive(path string, hintName string, workDir string, targetDir string, action manifest.ExtractAction) error {
	format := action.Format
	if format == "" || format == "auto" {
		format = inferArchiveFormat(hintName)
		if format == "" {
			format = inferArchiveFormat(path)
		}
		if format == "" {
			return fmt.Errorf("cannot infer archive format for %s; set extract.format", formatHint(hintName, path))
		}
	}
	switch format {
	case "tar.gz":
		return extractTar(path, workDir, targetDir, action, "gzip")
	case "tar.xz":
		return extractTar(path, workDir, targetDir, action, "xz")
	case "zip":
		return extractZip(path, targetDir, action)
	default:
		return fmt.Errorf("unsupported archive format %s", format)
	}
}

func listArchiveFiles(path string, hintName string, action manifest.ExtractAction) ([]string, []string, error) {
	format := action.Format
	if format == "" || format == "auto" {
		format = inferArchiveFormat(hintName)
		if format == "" {
			format = inferArchiveFormat(path)
		}
		if format == "" {
			return nil, nil, fmt.Errorf("cannot infer archive format for %s; set extract.format", formatHint(hintName, path))
		}
	}
	switch format {
	case "tar.gz":
		return listTarFiles(path, action, "gzip")
	case "tar.xz":
		return listTarFiles(path, action, "xz")
	case "zip":
		return listZipFiles(path, action)
	default:
		return nil, nil, fmt.Errorf("unsupported archive format %s", format)
	}
}

func listTarFiles(path string, action manifest.ExtractAction, compress string) ([]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	var reader io.Reader = f
	if compress == "gzip" {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, nil, err
		}
		defer gr.Close()
		reader = gr
	}
	if compress == "xz" {
		xr, err := xz.NewReader(f)
		if err != nil {
			return nil, nil, err
		}
		reader = xr
	}
	tr := tar.NewReader(reader)
	var files []string
	var skipped []string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		name := stripComponents(hdr.Name, action.StripComponents)
		if name == "" {
			continue
		}
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			if shouldInclude(name, action.Pick, action.Omit) {
				files = append(files, name)
			} else {
				skipped = append(skipped, name)
			}
		}
	}
	return files, skipped, nil
}

func listZipFiles(path string, action manifest.ExtractAction) ([]string, []string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	var files []string
	var skipped []string
	for _, f := range r.File {
		name := stripComponents(f.Name, action.StripComponents)
		if name == "" {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if shouldInclude(name, action.Pick, action.Omit) {
			files = append(files, name)
		} else {
			skipped = append(skipped, name)
		}
	}
	return files, skipped, nil
}

func extractTar(path string, workDir string, targetDir string, action manifest.ExtractAction, compress string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var reader io.Reader = f
	if compress == "gzip" {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gr.Close()
		reader = gr
	}
	if compress == "xz" {
		xr, err := xz.NewReader(f)
		if err != nil {
			return err
		}
		reader = xr
	}
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := stripComponents(hdr.Name, action.StripComponents)
		if name == "" {
			continue
		}
		if !shouldInclude(name, action.Pick, action.Omit) {
			continue
		}
		target := filepath.Join(targetDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
			if err := os.Chmod(target, hdr.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(path string, targetDir string, action manifest.ExtractAction) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := stripComponents(f.Name, action.StripComponents)
		if name == "" {
			continue
		}
		if !shouldInclude(name, action.Pick, action.Omit) {
			continue
		}
		target := filepath.Join(targetDir, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		if err := out.Close(); err != nil {
			rc.Close()
			return err
		}
		if err := os.Chmod(target, f.Mode().Perm()); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func recordExtractedList(root string, targetDir string, files []string, receiptFiles *[]state.ReceiptFile) error {
	for _, name := range files {
		target := filepath.Join(targetDir, name)
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		if info.IsDir() {
			continue
		}
		sum, _, err := hashFileWithSize(target)
		if err != nil {
			return err
		}
		rel := normalizePathForReceipt(root, target)
		*receiptFiles = append(*receiptFiles, state.ReceiptFile{
			Path:   rel,
			Type:   "file",
			Mode:   int(info.Mode().Perm()),
			SHA256: sum,
		})
	}
	return nil
}

func stripComponents(path string, count int) string {
	if count <= 0 {
		return filepath.Clean(path)
	}
	parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
	if len(parts) <= count {
		return ""
	}
	return filepath.Join(parts[count:]...)
}

func shouldInclude(name string, pick []string, omit []string) bool {
	if len(pick) > 0 {
		for _, p := range pick {
			if ok, _ := filepath.Match(p, name); ok {
				return true
			}
		}
		return false
	}
	if len(omit) > 0 {
		for _, o := range omit {
			if ok, _ := filepath.Match(o, name); ok {
				return false
			}
		}
		return true
	}
	return true
}

func inferArchiveFormat(name string) string {
	if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(name, ".tar.xz") {
		return "tar.xz"
	}
	if strings.HasSuffix(name, ".zip") {
		return "zip"
	}
	return ""
}

func formatHint(hintName, path string) string {
	if hintName != "" {
		return hintName
	}
	return path
}
