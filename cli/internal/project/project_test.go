package project

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func setup(t *testing.T) (home, projects, cwd string) {
	t.Helper()
	home = t.TempDir()
	projects = filepath.Join(home, "Projects")
	mkdirs(t, projects, "tdm-app", "claude-base-project-agent-kit", "treeboard", ".hidden")
	os.WriteFile(filepath.Join(projects, "notes.txt"), []byte("x"), 0o644)
	cwd = filepath.Join(home, "work")
	mkdirs(t, cwd, "local-only")
	return
}

func TestResolveEmptyIsCwd(t *testing.T) {
	home, projects, cwd := setup(t)
	got, err := Resolve("", projects, cwd, home)
	if err != nil || got != cwd {
		t.Fatalf("got %q, %v; want %q", got, err, cwd)
	}
}

func TestResolveNameInProjectsDir(t *testing.T) {
	home, projects, cwd := setup(t)
	got, err := Resolve("tdm-app", projects, cwd, home)
	if err != nil || got != filepath.Join(projects, "tdm-app") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolveBareNameFallsBackToCwd(t *testing.T) {
	home, projects, cwd := setup(t)
	got, err := Resolve("local-only", projects, cwd, home)
	if err != nil || got != filepath.Join(cwd, "local-only") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolvePaths(t *testing.T) {
	home, projects, cwd := setup(t)
	abs := filepath.Join(projects, "treeboard")
	cases := map[string]string{
		abs:                         abs,
		"./local-only":              filepath.Join(cwd, "local-only"),
		".":                         cwd,
		"../Projects/tdm-app":       filepath.Join(projects, "tdm-app"),
		"~/Projects/treeboard":      abs,
		filepath.Join("..", "work"): cwd,
	}
	for in, want := range cases {
		got, err := Resolve(in, projects, cwd, home)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestResolvePathErrors(t *testing.T) {
	home, projects, cwd := setup(t)
	if _, err := Resolve("./nope", projects, cwd, home); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing path: %v", err)
	}
	if _, err := Resolve(filepath.Join(projects, "notes.txt"), projects, cwd, home); err == nil || !strings.Contains(err.Error(), "is a file") {
		t.Errorf("file path: %v", err)
	}
}

func TestResolveTypoSuggests(t *testing.T) {
	home, projects, cwd := setup(t)
	_, err := Resolve("tdm", projects, cwd, home)
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
	if !reflect.DeepEqual(nf.Suggestions, []string{"tdm-app"}) {
		t.Errorf("suggestions = %v", nf.Suggestions)
	}
	wantAll := []string{"claude-base-project-agent-kit", "tdm-app", "treeboard"}
	if !reflect.DeepEqual(nf.All, wantAll) {
		t.Errorf("all = %v, want %v (hidden dirs and files excluded)", nf.All, wantAll)
	}
	msg := err.Error()
	for _, s := range []string{"did you mean: tdm-app", "projects: ", "or pass a path"} {
		if !strings.Contains(msg, s) {
			t.Errorf("message %q missing %q", msg, s)
		}
	}
}

func TestResolveNoMatchStillListsCandidates(t *testing.T) {
	home, projects, cwd := setup(t)
	_, err := Resolve("zzzz", projects, cwd, home)
	var nf *NotFoundError
	if !errors.As(err, &nf) || len(nf.Suggestions) != 0 || len(nf.All) != 3 {
		t.Fatalf("got %#v", err)
	}
	if !strings.Contains(err.Error(), "tdm-app") {
		t.Errorf("error must list candidates: %v", err)
	}
}

func TestResolveMissingProjectsDir(t *testing.T) {
	home := t.TempDir()
	_, err := Resolve("x", filepath.Join(home, "nope"), home, home)
	var nf *NotFoundError
	if !errors.As(err, &nf) || !nf.DirMissing {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "ckit config set projectsDir") {
		t.Errorf("must hint at the fix: %v", err)
	}
}

func TestMatchTiers(t *testing.T) {
	names := []string{"tdm-app", "treeboard", "claude-base-project-agent-kit", "TDM"}
	cases := []struct {
		q    string
		tier Tier
		want []string
	}{
		{"tdm", TierExact, []string{"TDM"}}, // case-insensitive exact beats prefix
		{"tre", TierPrefix, []string{"treeboard"}},
		{"agent", TierSubstring, []string{"claude-base-project-agent-kit"}},
		{"trbrd", TierFuzzy, []string{"treeboard"}},
		{"qqq", TierNone, nil},
		{"", TierNone, nil},
	}
	for _, tc := range cases {
		tier, ms := Match(names, tc.q)
		var got []string
		for _, m := range ms {
			got = append(got, m.Name)
		}
		if tier != tc.tier || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Match(%q) = %v %v, want %v %v", tc.q, tier, got, tc.tier, tc.want)
		}
	}
}

func TestSuggestCapsAtFive(t *testing.T) {
	var names []string
	for _, s := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"} {
		names = append(names, "proj-"+s)
	}
	if got := Suggest(names, "proj"); len(got) != 5 {
		t.Fatalf("got %d suggestions", len(got))
	}
}
