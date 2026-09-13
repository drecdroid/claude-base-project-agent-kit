package launch

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeCodeURLWindowsPathWithSpecials(t *testing.T) {
	folder := `C:\Users\Me\My Projects\a&b 100%\cañón`
	prompt := "hi & 100% done?+ok=#x"
	got := ClaudeCodeURL(folder, prompt)
	want := "claude://code/new?folder=C%3A%5CUsers%5CMe%5CMy%20Projects%5Ca%26b%20100%25%5Cca%C3%B1%C3%B3n" +
		"&q=hi%20%26%20100%25%20done%3F%2Bok%3D%23x"
	if got != want {
		t.Fatalf("got\n  %s\nwant\n  %s", got, want)
	}
	// Exactly one raw & (the separator) and no raw space/%-that-is-not-an-escape.
	if strings.Count(got, "&") != 1 || strings.ContainsAny(got, " +#\\") {
		t.Fatalf("unsafe characters left in %s", got)
	}
	// Round-trips through a standard parser to the exact inputs.
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "claude" || u.Host != "code" || u.Path != "/new" {
		t.Fatalf("parsed %+v", u)
	}
	q := u.Query()
	if q.Get("folder") != folder || q.Get("q") != prompt {
		t.Fatalf("round trip: folder=%q q=%q", q.Get("folder"), q.Get("q"))
	}
}

func TestClaudeCodeURLNoPrompt(t *testing.T) {
	got := ClaudeCodeURL("/home/me/p", "")
	if got != "claude://code/new?folder=%2Fhome%2Fme%2Fp" {
		t.Fatalf("got %s", got)
	}
}

func TestOpenURLPerOS(t *testing.T) {
	u := "claude://code/new?folder=x&q=y"
	cases := map[string][]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler", u},
		"darwin":  {"open", u},
		"linux":   {"xdg-open", u},
		"freebsd": {"xdg-open", u},
	}
	for goos, want := range cases {
		if got := OpenURL(goos, u).Argv(); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %q want %q", goos, got, want)
		}
	}
	// Never through cmd.exe: its parser mangles & and %.
	for _, a := range OpenURL("windows", u).Argv() {
		if strings.EqualFold(a, "cmd") || strings.EqualFold(a, "start") || strings.EqualFold(a, "/c") {
			t.Fatalf("windows launcher must not use cmd/start: %q", OpenURL("windows", u).Argv())
		}
	}
}

func TestCodeArgv(t *testing.T) {
	if got := Code("", `C:\p`).Argv(); !reflect.DeepEqual(got, []string{"code", `C:\p`}) {
		t.Errorf("default: %q", got)
	}
	if got := Code(`D:\VSCode\bin\code.cmd`, "/p").Argv(); got[0] != `D:\VSCode\bin\code.cmd` {
		t.Errorf("override: %q", got)
	}
}

func TestSmartGitArgvPerOS(t *testing.T) {
	cases := []struct {
		goos, override string
		want           []string
	}{
		{"windows", "", []string{`C:\Program Files\SmartGit\bin\smartgit.exe`, "D"}},
		{"darwin", "", []string{"open", "-a", "/Applications/SmartGit.app", "D"}},
		{"darwin", "/opt/sg/bin/smartgit.sh", []string{"/opt/sg/bin/smartgit.sh", "D"}},
		{"linux", "", []string{"smartgit", "D"}},
		{"linux", "/opt/smartgit/bin/smartgit.sh", []string{"/opt/smartgit/bin/smartgit.sh", "D"}},
	}
	for _, tc := range cases {
		if got := SmartGit(tc.goos, tc.override, "D").Argv(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s/%q: got %q want %q", tc.goos, tc.override, got, tc.want)
		}
	}
}

func TestValidateApp(t *testing.T) {
	for _, a := range Apps {
		if err := ValidateApp(a); err != nil {
			t.Error(err)
		}
	}
	if err := ValidateApp("vim"); err == nil || !strings.Contains(err.Error(), "code, smartgit, claude") {
		t.Errorf("got %v", err)
	}
}
