package scaffold

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// MaxFileBytes caps a single template file.
const MaxFileBytes = 10 << 20

// checkEntryName rejects archive entry names that could escape the target:
// absolute paths, drive letters, backslashes (a Windows separator smuggled
// through a '/'-only format) and any ".." segment. Checked for EVERY entry,
// including ones outside template/ that would otherwise be ignored.
func checkEntryName(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) ||
		(len(name) >= 2 && name[1] == ':') || strings.ContainsRune(name, 0) {
		return fmt.Errorf("unsafe path in template archive: %q", name)
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return fmt.Errorf("unsafe path in template archive: %q", name)
		}
	}
	return nil
}

// within reports whether target is inside root (belt and braces after
// checkEntryName).
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// ExtractTemplate reads a GitHub-style .tar.gz (<top>/...) and writes only
// <top>/template/** into dst, keeping dotfiles and file bytes as-is (LF stays
// LF). Symlinks, hardlinks and devices inside template/ are rejected.
func ExtractTemplate(r io.Reader, dst string) ([]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("template archive is not gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var files []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return files, fmt.Errorf("reading template archive: %w", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader || h.Typeflag == tar.TypeXHeader {
			continue // GitHub's pax_global_header (commit id), not a file
		}
		if err := checkEntryName(h.Name); err != nil {
			return files, err
		}
		parts := strings.Split(strings.TrimPrefix(path.Clean(h.Name), "./"), "/")
		if len(parts) < 3 || parts[1] != "template" {
			continue // outside <top>/template/, or the template dir itself
		}
		rel := strings.Join(parts[2:], "/")
		target := filepath.Join(dst, filepath.FromSlash(rel))
		if !within(dst, target) {
			return files, fmt.Errorf("unsafe path in template archive: %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return files, err
			}
		case tar.TypeReg:
			if h.Size > MaxFileBytes {
				return files, fmt.Errorf("template file %s is larger than %d MB", rel, MaxFileBytes>>20)
			}
			if err := writeFile(target, tr, h.FileInfo().Mode()); err != nil {
				return files, err
			}
			files = append(files, rel)
		default:
			return files, fmt.Errorf("unsupported entry type %q in template archive: %s (only files and directories)", string(h.Typeflag), h.Name)
		}
	}
	if len(files) == 0 {
		return nil, errors.New("template archive has no <top>/template/ files")
	}
	sort.Strings(files)
	return files, nil
}

func writeFile(target string, r io.Reader, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	// O_EXCL: never overwrite something already in the target.
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(r, MaxFileBytes)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// CopyDir copies a local template directory into dst (dotfiles included;
// symlinks and other non-regular files are rejected).
func CopyDir(src, dst string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() > MaxFileBytes {
				return fmt.Errorf("template file %s is larger than %d MB", rel, MaxFileBytes>>20)
			}
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			if err := writeFile(target, in, info.Mode()); err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		default:
			return fmt.Errorf("unsupported file in template: %s (only files and directories)", p)
		}
	})
	if err != nil {
		return files, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("template directory %s is empty", src)
	}
	sort.Strings(files)
	return files, nil
}
