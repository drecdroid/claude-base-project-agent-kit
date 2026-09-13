package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHome points os.UserHomeDir at a temp dir on every OS.
func fakeHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	return h
}

func TestPathIsUnderHomeNotAppData(t *testing.T) {
	h := fakeHome(t)
	t.Setenv("APPDATA", filepath.Join(h, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "xdg"))
	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if home != h {
		t.Fatalf("Home() = %q, want the overridden %q", home, h)
	}
	p := Path(home)
	if want := filepath.Join(h, ".ckit", "config.json"); p != want {
		t.Fatalf("Path = %q, want %q", p, want)
	}
	if uc, err := os.UserConfigDir(); err == nil && strings.HasPrefix(p, uc) {
		t.Fatalf("config path %q is under os.UserConfigDir %q (%%APPDATA%% redirection risk)", p, uc)
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.ToLower(p), strings.ToLower(os.Getenv("APPDATA"))) {
		t.Fatalf("config path %q must not be under %%APPDATA%%", p)
	}
}

func TestLoadMissingIsDefaults(t *testing.T) {
	h := fakeHome(t)
	c, err := Load(Path(h))
	if err != nil {
		t.Fatal(err)
	}
	if c != (Config{}) {
		t.Fatalf("want zero config, got %+v", c)
	}
	if got, want := c.ResolvedProjectsDir(h), filepath.Join(h, "Projects"); got != want {
		t.Fatalf("default projectsDir = %q, want %q", got, want)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	h := fakeHome(t)
	p := Path(h)
	var c Config
	if err := c.Set("projectsDir", "~/code"); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("SMARTGITPATH", `C:\Tools\SmartGit\bin\smartgit.exe`); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round trip: got %+v want %+v", got, c)
	}
	if d := got.ResolvedProjectsDir(h); d != filepath.Join(h, "code") {
		t.Fatalf("~ not expanded: %q", d)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "\r") {
		t.Fatal("config file must be LF")
	}
	if strings.Contains(string(b), "codePath") {
		t.Fatalf("unset keys should be omitted: %s", b)
	}
	// No temp files left behind by the atomic write.
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Fatalf("want only config.json in %s, got %d entries", filepath.Dir(p), len(ents))
	}
}

func TestSetEmptyUnsets(t *testing.T) {
	c := Config{CodePath: "x"}
	if err := c.Set("codePath", ""); err != nil {
		t.Fatal(err)
	}
	if c.CodePath != "" {
		t.Fatal("empty value must unset")
	}
}

func TestUnknownKeyListsValidOnes(t *testing.T) {
	var c Config
	err := c.Set("projectDir", "x")
	if err == nil {
		t.Fatal("want error")
	}
	for _, k := range KeyNames() {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not list key %s", err, k)
		}
	}
	if _, err := c.Get("nope"); err == nil {
		t.Fatal("Get unknown key: want error")
	}
}

func TestLoadInvalidJSONNamesFile(t *testing.T) {
	h := fakeHome(t)
	p := Path(h)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("{nope"), 0o644)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("want error naming %s, got %v", p, err)
	}
}
