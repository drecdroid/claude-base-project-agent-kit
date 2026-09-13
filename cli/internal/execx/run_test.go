package execx

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestFormat(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"claude", "plugin", "install", "a@b", "--scope", "project", "-y"}, "claude plugin install a@b --scope project -y"},
		{[]string{"rundll32", "url.dll,FileProtocolHandler", "claude://code/new?folder=C%3A&q=x"}, "rundll32 url.dll,FileProtocolHandler claude://code/new?folder=C%3A&q=x"},
		{[]string{"code", `C:\My Projects\a`}, `code 'C:\My Projects\a'`},
		{[]string{"x", ""}, "x ''"},
		{[]string{"x", "it's"}, `x 'it'\''s'`},
	}
	for _, tc := range cases {
		if got := Format(tc.in); got != tc.want {
			t.Errorf("Format(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestCmdArgv(t *testing.T) {
	c := Cmd{Prog: "git", Args: []string{"status"}}
	if !reflect.DeepEqual(c.Argv(), []string{"git", "status"}) {
		t.Fatal(c.Argv())
	}
}

func TestResolveProgramMissing(t *testing.T) {
	if _, err := ResolveProgram("", "ckit-definitely-not-a-program-xyz"); err == nil {
		t.Fatal("want error")
	}
}

// A bare name must not resolve to a same-named binary in the current
// directory (exec.ErrDot), e.g. a hostile ./claude.exe inside a project.
func TestResolveProgramRejectsDotBinary(t *testing.T) {
	dir := t.TempDir()
	name := "ckitdot"
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("PATH", "."+string(os.PathListSeparator)+os.Getenv("PATH"))
	if p, err := ResolveProgram("", name); err == nil {
		t.Fatalf("resolved %q from the current directory", p)
	}
}

func TestRealRunExitCode(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := Real{}
	out, code, err := r.Output(context.Background(), Cmd{Prog: self, Args: []string{"-test.run=^$"}})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out)
	}
}
