package scaffold

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type entry struct {
	name string
	body string
	typ  byte
}

func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
			if strings.HasSuffix(e.name, "/") {
				typ = tar.TypeDir
			}
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o644, Size: int64(len(e.body))}
		if typ == tar.TypeDir || typ == tar.TypeSymlink {
			h.Size = 0
		}
		if typ == tar.TypeSymlink {
			h.Linkname = "../../outside"
		}
		if typ == tar.TypeXGlobalHeader {
			// Like GitHub's pax_global_header: only PAX records may be set.
			h = &tar.Header{Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "abc123"}}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// githubLike mirrors codeload's layout: a pax global header, then
// <repo>-<ref>/ with the whole repo inside.
func githubLike(t *testing.T, extra ...entry) []byte {
	es := []entry{
		{name: "pax_global_header", typ: tar.TypeXGlobalHeader},
		{name: "kit-main/"},
		{name: "kit-main/README.md", body: "# kit\n"},
		{name: "kit-main/plugins/agent-kit/rules.md", body: "rules\n"},
		{name: "kit-main/template/"},
		{name: "kit-main/template/CLAUDE.md", body: "# {{project_name}}\n\n{{description}}\n"},
		{name: "kit-main/template/.gitattributes", body: "* text=auto eol=lf\n"},
		{name: "kit-main/template/.editorconfig", body: "root = true\n"},
		{name: "kit-main/template/.claude/"},
		{name: "kit-main/template/.claude/settings.json", body: "{}\n"},
	}
	return tarball(t, append(es, extra...)...)
}

func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func TestExtractOnlyTemplateKeepsDotfilesAndLF(t *testing.T) {
	dst := t.TempDir()
	files, err := ExtractTemplate(bytes.NewReader(githubLike(t)), dst)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".claude/settings.json", ".editorconfig", ".gitattributes", "CLAUDE.md"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	if got := listTree(t, dst); !reflect.DeepEqual(got, want) {
		t.Fatalf("tree = %v (README/plugins must not be extracted)", got)
	}
	b, _ := os.ReadFile(filepath.Join(dst, ".gitattributes"))
	if string(b) != "* text=auto eol=lf\n" {
		t.Fatalf("bytes changed: %q", b)
	}
}

func TestExtractRejectsTraversalAnywhere(t *testing.T) {
	bad := []string{
		"kit-main/template/../../evil.txt",
		"../evil.txt",
		"/abs/template/x",
		"C:/x/template/y",
		`kit-main\template\..\..\evil`,
		"kit-main/README/../../x", // outside template/ is still checked
	}
	for _, name := range bad {
		dst := t.TempDir()
		_, err := ExtractTemplate(bytes.NewReader(githubLike(t, entry{name: name, body: "x"})), dst)
		if err == nil || !strings.Contains(err.Error(), "unsafe path") {
			t.Errorf("%q: want unsafe path error, got %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "evil.txt")); err == nil {
			t.Errorf("%q escaped the target", name)
		}
	}
}

func TestExtractRejectsSymlinkInTemplate(t *testing.T) {
	_, err := ExtractTemplate(bytes.NewReader(githubLike(t, entry{name: "kit-main/template/link", typ: tar.TypeSymlink})), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsupported entry type") {
		t.Fatalf("got %v", err)
	}
}

func TestExtractNoTemplateDir(t *testing.T) {
	_, err := ExtractTemplate(bytes.NewReader(tarball(t, entry{name: "x-main/README.md", body: "x"})), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no <top>/template/") {
		t.Fatalf("got %v", err)
	}
	if _, err := ExtractTemplate(strings.NewReader("not gzip"), t.TempDir()); err == nil {
		t.Fatal("want gzip error")
	}
}

func TestExtractNeverOverwrites(t *testing.T) {
	dst := t.TempDir()
	os.WriteFile(filepath.Join(dst, "CLAUDE.md"), []byte("mine"), 0o644)
	if _, err := ExtractTemplate(bytes.NewReader(githubLike(t)), dst); err == nil {
		t.Fatal("want error instead of overwriting")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "CLAUDE.md")); string(b) != "mine" {
		t.Fatal("overwrote an existing file")
	}
}

func TestFill(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# {{project_name}}\n\n{{description}}\n{{unknown}}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte("{{project_name}}\x00"), 0o644)
	os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("nothing here\n"), 0o644)
	changed, err := Fill(dir, []string{"CLAUDE.md", "bin.dat", "plain.txt"}, Values{
		ProjectName: "demo",
		Description: "  a $HOME & 100% {{project_name}}\r\nline two ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(changed, []string{"CLAUDE.md"}) {
		t.Fatalf("changed = %v", changed)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	// User text is literal (no recursive expansion), one line, unknown placeholders untouched.
	want := "# demo\n\na $HOME & 100% {{project_name}} line two\n{{unknown}}\n"
	if string(b) != want {
		t.Fatalf("got %q want %q", b, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "bin.dat")); !bytes.Contains(b, []byte("{{project_name}}")) {
		t.Fatal("binary file must be skipped")
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"demo", "my-app", "My_App.v2", "a", "x1"}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("%q: %v", n, err)
		}
	}
	bad := []string{"", ".", "..", ".hidden", "-x", "a/b", `a\b`, "a b", "trail.", "CON", "nul.txt", "com1", "é", strings.Repeat("a", 101), "a:b", "a&b"}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("%q: want error", n)
		}
	}
}

func TestCheckTarget(t *testing.T) {
	root := t.TempDir()
	if err := CheckTarget(filepath.Join(root, "new")); err != nil {
		t.Errorf("missing dir: %v", err)
	}
	os.Mkdir(filepath.Join(root, "empty"), 0o755)
	if err := CheckTarget(filepath.Join(root, "empty")); err != nil {
		t.Errorf("empty dir: %v", err)
	}
	os.MkdirAll(filepath.Join(root, "full", "x"), 0o755)
	if err := CheckTarget(filepath.Join(root, "full")); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("full dir: %v", err)
	}
	os.WriteFile(filepath.Join(root, "file"), nil, 0o644)
	if err := CheckTarget(filepath.Join(root, "file")); err == nil {
		t.Error("file: want error")
	}
}

func TestResolveSource(t *testing.T) {
	s, err := ResolveSource("", "")
	if err != nil || s.Kind != SourceTarball || s.URL != "https://codeload.github.com/drecdroid/claude-base-project-agent-kit/tar.gz/main" {
		t.Errorf("default: %+v %v", s, err)
	}
	if s, _ := ResolveSource("", "v1.2.0"); !strings.HasSuffix(s.URL, "/tar.gz/v1.2.0") {
		t.Errorf("ref: %+v", s)
	}
	for _, r := range []string{"a b", "../x", "x;rm"} {
		if _, err := ResolveSource("", r); err == nil {
			t.Errorf("bad ref %q accepted", r)
		}
	}
	if s, err := ResolveSource("https://example.com/fork.tar.gz", ""); err != nil || s.URL != "https://example.com/fork.tar.gz" {
		t.Errorf("url: %+v %v", s, err)
	}
	if _, err := ResolveSource("https://example.com/x.tar.gz", "main"); err == nil {
		t.Error("--ref with explicit source must error")
	}
	kitRoot := t.TempDir()
	os.MkdirAll(filepath.Join(kitRoot, "template"), 0o755)
	if s, err := ResolveSource(kitRoot, ""); err != nil || s.Kind != SourceLocal || s.Dir != filepath.Join(kitRoot, "template") {
		t.Errorf("kit root: %+v %v", s, err)
	}
	tplDir := t.TempDir()
	os.WriteFile(filepath.Join(tplDir, "CLAUDE.md"), nil, 0o644)
	if s, err := ResolveSource(tplDir, ""); err != nil || s.Dir != tplDir {
		t.Errorf("template dir: %+v %v", s, err)
	}
	if _, err := ResolveSource(t.TempDir(), ""); err == nil {
		t.Error("random dir accepted")
	}
}

func TestFetchTarballAndLocal(t *testing.T) {
	archive := githubLike(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok.tar.gz" {
			w.Write(archive)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	f, err := Fetch(context.Background(), srv.Client(), Source{Kind: SourceTarball, URL: srv.URL + "/ok.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "proj")
	files, err := f.WriteTo(dst)
	if err != nil || len(files) != 4 {
		t.Fatalf("files %v err %v", files, err)
	}

	_, err = Fetch(context.Background(), srv.Client(), Source{Kind: SourceTarball, URL: srv.URL + "/missing"})
	if err == nil || !strings.Contains(err.Error(), "private or missing") || !strings.Contains(err.Error(), "--template-source") {
		t.Fatalf("404: %v", err)
	}

	// Local: dotfiles and nested dirs copied.
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, ".claude"), 0o755)
	os.WriteFile(filepath.Join(src, ".claude", "settings.json"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("# {{project_name}}\n"), 0o644)
	lf, err := Fetch(context.Background(), nil, Source{Kind: SourceLocal, Dir: src})
	if err != nil {
		t.Fatal(err)
	}
	dst2 := filepath.Join(t.TempDir(), "p2")
	files, err = lf.WriteTo(dst2)
	if err != nil || !reflect.DeepEqual(files, []string{".claude/settings.json", "CLAUDE.md"}) {
		t.Fatalf("local: %v %v", files, err)
	}
}
