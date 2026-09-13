package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type out struct {
	s    string
	code int
}

type fakeProbe struct {
	paths  map[string]string // prog -> path; absent = not found
	outs   map[string]out    // "prog arg arg" -> output
	exists map[string]bool
}

func (f fakeProbe) LookPath(p string) (string, error) {
	if v, ok := f.paths[p]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func (f fakeProbe) Output(_ context.Context, p string, a ...string) (string, int, error) {
	k := strings.Join(append([]string{p}, a...), " ")
	if o, ok := f.outs[k]; ok {
		return o.s, o.code, nil
	}
	return "", 1, nil
}

func (f fakeProbe) Exists(p string) bool { return f.exists[p] }

const sgWin = `C:\Program Files\SmartGit\bin\smartgit.exe`

func healthy() fakeProbe {
	return fakeProbe{
		paths: map[string]string{"git": "/usr/bin/git", "gh": "/usr/bin/gh", "claude": "/bin/claude", "code": "/bin/code"},
		outs: map[string]out{
			"git --version":                          {"git version 2.50.1\n", 0},
			"git config --global init.defaultBranch": {"main\n", 0},
			"gh --version":                           {"gh version 2.80.0 (2026-01-01)\nhttps://github.com/cli/cli/releases/tag/v2.80.0\n", 0},
			"gh auth status":                         {"Logged in", 0},
			"claude --version":                       {"2.1.251 (Claude Code)\n", 0},
			"claude plugin marketplace list --json":  {`[{"name":"claude-plugins-official"},{"name":"claude-base-project-agent-kit"}]`, 0},
			"code --version":                         {"1.105.0\nabc\nx64\n", 0},
		},
		exists: map[string]bool{sgWin: true},
	}
}

func run(t *testing.T, p fakeProbe, o Options) Report {
	t.Helper()
	if o.GOOS == "" {
		o.GOOS = "windows"
	}
	if o.SmartgitPath == "" {
		o.SmartgitPath = sgWin
	}
	r, err := Run(context.Background(), p, o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func byName(r Report) map[string]Check {
	m := map[string]Check{}
	for _, c := range r.Checks {
		m[c.Name] = c
	}
	return m
}

func TestHealthyAllOK(t *testing.T) {
	r := run(t, healthy(), Options{For: []string{"all"}})
	if !r.OK {
		t.Fatalf("want ok: %+v", r)
	}
	m := byName(r)
	if m[CGit].Version != "git version 2.50.1" || m[CGh].Version != "gh version 2.80.0 (2026-01-01)" || m[CClaude].Version != "2.1.251 (Claude Code)" || m[CCode].Version != "1.105.0" {
		t.Errorf("versions: %+v", m)
	}
	if m[CMarketplace].Status != OK || m[CSmartGit].Status != OK || m[CGhAuth].Status != OK {
		t.Errorf("statuses: %+v", m)
	}
	if len(r.For) != len(Requirements) {
		t.Errorf("for=all should expand to every set: %v", r.For)
	}
}

func TestDefaultForOnlyRequiresClaudeAndMarketplace(t *testing.T) {
	p := healthy()
	delete(p.paths, "code")
	delete(p.paths, "gh")
	p.exists = nil
	r := run(t, p, Options{})
	if !r.OK {
		t.Fatalf("code/gh/smartgit are not required by the default set: %+v", r)
	}
	m := byName(r)
	if !m[CClaude].Required || !m[CMarketplace].Required || m[CCode].Required || m[CSmartGit].Required {
		t.Errorf("required flags wrong: %+v", m)
	}
	if m[CCode].Status != Missing || m[CCode].Fix == "" || m[CSmartGit].Status != Missing {
		t.Errorf("missing tools must still be reported with a fix: %+v", m)
	}
}

func TestMarketplaceNotAddedFailsPlugin(t *testing.T) {
	p := healthy()
	p.outs["claude plugin marketplace list --json"] = out{`[{"name":"claude-plugins-official"}]`, 0}
	r := run(t, p, Options{For: []string{"plugin"}})
	c := byName(r)[CMarketplace]
	if r.OK || c.Status != Missing || c.Fix != "ckit source add" {
		t.Fatalf("got ok=%v %+v", r.OK, c)
	}
	if r2 := run(t, p, Options{For: []string{"source"}}); !r2.OK {
		t.Fatal("`source` does not need the marketplace already added")
	}
}

func TestClaudeMissingCascades(t *testing.T) {
	p := healthy()
	delete(p.paths, "claude")
	r := run(t, p, Options{For: []string{"source"}})
	m := byName(r)
	if r.OK || m[CClaude].Status != Missing || m[CMarketplace].Status != Missing {
		t.Fatalf("got %+v", r)
	}
}

func TestGhNotLoggedInIsWarnAndFailsGithub(t *testing.T) {
	p := healthy()
	p.outs["gh auth status"] = out{"You are not logged into any GitHub hosts", 1}
	r := run(t, p, Options{For: []string{"github"}})
	c := byName(r)[CGhAuth]
	if r.OK || c.Status != Warn || !strings.Contains(c.Fix, "gh auth login") {
		t.Fatalf("got ok=%v %+v", r.OK, c)
	}
	if !run(t, p, Options{}).OK {
		t.Fatal("gh auth is not required by the default set")
	}
	// `new` alone needs only git: no gh, no claude.
	delete(p.paths, "gh")
	delete(p.paths, "claude")
	if !run(t, p, Options{For: []string{"new"}}).OK {
		t.Fatal("new without github/plugin must only require git")
	}
}

func TestDefaultBranchUnsetIsInfoNeverFails(t *testing.T) {
	p := healthy()
	p.outs["git config --global init.defaultBranch"] = out{"", 1}
	r := run(t, p, Options{For: []string{"new"}})
	c := byName(r)[CGitBranch]
	if !r.OK || c.Status != Info || !strings.Contains(c.Detail, "git init -b main") {
		t.Fatalf("got ok=%v %+v", r.OK, c)
	}
}

func TestSmartGitOverrideAndLinuxPath(t *testing.T) {
	p := healthy()
	p.exists = map[string]bool{`D:\sg\smartgit.exe`: true}
	if c := byName(run(t, p, Options{SmartgitPath: `D:\sg\smartgit.exe`}))[CSmartGit]; c.Status != OK {
		t.Errorf("override: %+v", c)
	}
	p.paths["smartgit"] = "/usr/local/bin/smartgit"
	if c := byName(run(t, p, Options{GOOS: "linux", SmartgitPath: "smartgit"}))[CSmartGit]; c.Status != OK || c.Path != "/usr/local/bin/smartgit" {
		t.Errorf("linux PATH: %+v", c)
	}
}

func TestUnknownForListsValid(t *testing.T) {
	_, err := Run(context.Background(), healthy(), Options{GOOS: "linux", For: []string{"plugins"}})
	if err == nil || !strings.Contains(err.Error(), "open-code") {
		t.Fatalf("got %v", err)
	}
}

func TestHasMarketplaceToleratesPreamble(t *testing.T) {
	ok, err := HasMarketplace("warning: x\n[{\"name\":\"claude-base-project-agent-kit\"}]", "claude-base-project-agent-kit")
	if err != nil || !ok {
		t.Fatalf("got %v %v", ok, err)
	}
	if _, err := HasMarketplace("garbage", "x"); err == nil {
		t.Fatal("want parse error")
	}
}

func TestJSONShapeAndText(t *testing.T) {
	p := healthy()
	delete(p.paths, "code")
	r := run(t, p, Options{For: []string{"open-code", "source"}})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	json.Unmarshal(b, &back)
	if back["ok"] != false {
		t.Errorf("json ok: %s", b)
	}
	if f := back["for"].([]any); len(f) != 2 || f[0] != "open-code" || f[1] != "source" {
		t.Errorf("json for: %v", f)
	}
	first := back["checks"].([]any)[0].(map[string]any)
	for _, k := range []string{"name", "status", "required"} {
		if _, ok := first[k]; !ok {
			t.Errorf("check json missing %q: %v", k, first)
		}
	}
	var buf bytes.Buffer
	WriteText(&buf, r)
	txt := buf.String()
	for _, s := range []string{"MISS * code", "fix: VS Code", "missing required tools"} {
		if !strings.Contains(txt, s) {
			t.Errorf("text missing %q:\n%s", s, txt)
		}
	}
}
