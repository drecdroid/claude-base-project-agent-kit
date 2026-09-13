// Package scaffold is the filesystem half of `ckit new`: project-name
// validation, where the template comes from (GitHub tarball, tarball URL, or a
// local kit checkout), safe extraction of ONLY the kit's template/ directory,
// and placeholder filling. No git, no network beyond one HTTP GET.
package scaffold

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/kit"
)

// DefaultRef is the branch fetched when --ref is not given ("always latest").
const DefaultRef = "main"

// MaxArchiveBytes caps a downloaded template archive.
const MaxArchiveBytes = 50 << 20

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var windowsReserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for i := 1; i <= 9; i++ {
		windowsReserved[fmt.Sprintf("COM%d", i)] = true
		windowsReserved[fmt.Sprintf("LPT%d", i)] = true
	}
}

// ValidateName checks a project name is safe as a folder name on every OS
// and as a GitHub repository name: letters, digits, ".", "_", "-", starting
// with a letter or digit, not ending with ".", at most 100 characters, and
// not a Windows reserved device name.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("project name is empty")
	case len(name) > 100:
		return fmt.Errorf("project name %q is longer than 100 characters", name)
	case !nameRe.MatchString(name):
		return fmt.Errorf("project name %q: use letters, digits, '.', '_' or '-', starting with a letter or digit", name)
	case strings.HasSuffix(name, "."):
		return fmt.Errorf("project name %q must not end with '.'", name)
	}
	base := strings.ToUpper(name)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if windowsReserved[base] {
		return fmt.Errorf("project name %q is a reserved device name on Windows", name)
	}
	return nil
}

// CheckTarget refuses a target directory that exists and is not empty.
func CheckTarget(dir string) error {
	st, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s already exists and is a file", dir)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(ents) > 0 {
		return fmt.Errorf("%s already exists and is not empty; pick another name or --dir", dir)
	}
	return nil
}

var refRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// SourceKind says how a template is obtained.
type SourceKind int

const (
	SourceTarball SourceKind = iota // a .tar.gz URL (GitHub codeload or other)
	SourceLocal                     // a directory on disk
)

// Source is a resolved --template-source/--ref pair.
type Source struct {
	Kind SourceKind
	URL  string // SourceTarball
	Dir  string // SourceLocal: the template directory itself
}

// TarballURL is GitHub's codeload tarball for the kit at ref; no auth
// needed once the repo is public.
func TarballURL(ref string) string {
	return "https://codeload.github.com/" + kit.Repo + "/tar.gz/" + ref
}

// ResolveSource turns --template-source and --ref into a Source.
//   - empty source: the kit's GitHub tarball at ref (default main)
//   - http(s) URL: a .tar.gz whose entries look like <top>/template/**
//   - otherwise a local path: a kit checkout (uses its template/) or a
//     template directory itself (contains CLAUDE.md)
func ResolveSource(source, ref string) (Source, error) {
	if ref != "" && (!refRe.MatchString(ref) || strings.Contains(ref, "..")) {
		return Source{}, fmt.Errorf("invalid --ref %q", ref)
	}
	if source == "" {
		if ref == "" {
			ref = DefaultRef
		}
		return Source{Kind: SourceTarball, URL: TarballURL(ref)}, nil
	}
	if ref != "" {
		return Source{}, errors.New("--ref only applies to the default GitHub template source; point --template-source at the ref you want instead")
	}
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://") {
		return Source{Kind: SourceTarball, URL: source}, nil
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return Source{}, err
	}
	if isDir(filepath.Join(abs, "template")) {
		return Source{Kind: SourceLocal, Dir: filepath.Join(abs, "template")}, nil
	}
	if isDir(abs) && isFile(filepath.Join(abs, "CLAUDE.md")) {
		return Source{Kind: SourceLocal, Dir: abs}, nil
	}
	return Source{}, fmt.Errorf("--template-source %s: not a kit checkout (no template/) nor a template directory (no CLAUDE.md)", abs)
}

// String describes the source for messages.
func (s Source) String() string {
	if s.Kind == SourceLocal {
		return s.Dir
	}
	return s.URL
}

func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }
func isFile(p string) bool { st, err := os.Stat(p); return err == nil && st.Mode().IsRegular() }

// Fetched is a template ready to write: an archive held in memory, or a local
// directory. Fetching happens BEFORE anything is created on disk, so a
// private/missing repo fails without leaving a half-made project behind.
type Fetched struct {
	src     Source
	archive []byte
}

// Fetch downloads a tarball source (or checks a local one).
func Fetch(ctx context.Context, client *http.Client, s Source) (*Fetched, error) {
	if s.Kind == SourceLocal {
		if !isDir(s.Dir) {
			return nil, fmt.Errorf("template directory %s does not exist", s.Dir)
		}
		return &Fetched{src: s}, nil
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	hint := "; pass --template-source <local kit checkout or .tar.gz URL>"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("template URL %s: %w%s", s.URL, err, hint)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading template %s: %w%s", s.URL, err, hint)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("downloading template %s: 404 not found; the kit repo may be private or missing, or the ref does not exist%s", s.URL, hint)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading template %s: HTTP %d%s", s.URL, resp.StatusCode, hint)
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(resp.Body, MaxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("downloading template %s: %w%s", s.URL, err, hint)
	}
	if n > MaxArchiveBytes {
		return nil, fmt.Errorf("template archive %s is larger than %d MB", s.URL, MaxArchiveBytes>>20)
	}
	return &Fetched{src: s, archive: buf.Bytes()}, nil
}

// WriteTo materializes the template into dst (created if needed) and
// returns the written files as slash-separated relative paths.
func (f *Fetched) WriteTo(dst string) ([]string, error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	if f.src.Kind == SourceLocal {
		return CopyDir(f.src.Dir, dst)
	}
	return ExtractTemplate(bytes.NewReader(f.archive), dst)
}
