package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

// scriptPrompter answers by substring of the question title; unscripted
// questions take their default. It records the order questions were asked.
type scriptPrompter struct {
	confirms map[string]bool
	inputs   map[string]string
	selects  map[string]string
	asked    []string
}

func match[T any](m map[string]T, title string) (T, bool) {
	for k, v := range m {
		if strings.Contains(title, k) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func (s *scriptPrompter) Select(title string, options []string) (string, error) {
	s.asked = append(s.asked, title)
	if v, ok := match(s.selects, title); ok {
		return v, nil
	}
	return options[0], nil
}
func (s *scriptPrompter) Confirm(title string, def bool) (bool, error) {
	s.asked = append(s.asked, title)
	if v, ok := match(s.confirms, title); ok {
		return v, nil
	}
	return def, nil
}
func (s *scriptPrompter) Input(title, _, def string, validate func(string) error) (string, error) {
	s.asked = append(s.asked, title)
	v := def
	if x, ok := match(s.inputs, title); ok {
		v = x
	}
	if validate != nil {
		if err := validate(v); err != nil {
			return "", err
		}
	}
	return v, nil
}
func (s *scriptPrompter) EditConfig(*config.Config, string, string) error { return nil }

const kitListed = `[{"name":"claude-plugins-official"},{"name":"claude-base-project-agent-kit"}]`

// kitTemplate makes a fake kit checkout with a template/ like the real one.
func kitTemplate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tpl := filepath.Join(root, "template")
	os.MkdirAll(filepath.Join(tpl, ".claude"), 0o755)
	os.WriteFile(filepath.Join(tpl, "CLAUDE.md"), []byte("# {{project_name}}\n\n{{description}}\n"), 0o644)
	os.WriteFile(filepath.Join(tpl, ".gitattributes"), []byte("* text=auto eol=lf\n"), 0o644)
	os.WriteFile(filepath.Join(tpl, ".claude", "settings.json"), []byte("{}\n"), 0o644)
	return root
}

func newFlowHarness(t *testing.T) *harness {
	h := newHarness(t)
	h.runner.paths = map[string]string{"git": "/bin/git", "claude": "/bin/claude", "gh": "/bin/gh"}
	h.runner.outs = map[string]fakeOut{
		"claude plugin marketplace list --json": {kitListed, 0},
		"gh auth status":                        {"Logged in", 0},
		"gh api user --jq .login":               {"octo\n", 0},
		"git config user.email":                 {"me@example.com\n", 0},
	}
	return h
}

// actions returns run/start argv in order (probes excluded), with the dir
// shown as <dir> when it is the project dir.
func actions(h *harness, dir string) []string {
	var out []string
	for _, c := range h.runner.calls {
		if c.kind == "output" {
			continue
		}
		s := strings.ReplaceAll(strings.Join(c.cmd.Argv(), " "), dir, "<dir>")
		if c.cmd.Dir == dir {
			s = "(in <dir>) " + s
		}
		out = append(out, c.kind+": "+s)
	}
	return out
}

func assertActions(t *testing.T, h *harness, dir string, want []string) {
	t.Helper()
	if got := actions(h, dir); !reflect.DeepEqual(got, want) {
		t.Fatalf("actions:\n  got  %q\n  want %q\nstdout:\n%s\nstderr:\n%s", got, want, h.out.String(), h.errb.String())
	}
}

const (
	aInit    = "run: (in <dir>) git init -b main"
	aAdd     = "run: (in <dir>) git add -A"
	aCommit  = "run: (in <dir>) git commit -m chore: init from agent-kit template"
	aPush    = "run: (in <dir>) git push -u origin main"
	aInstall = "run: (in <dir>) claude plugin install agent-kit@claude-base-project-agent-kit --scope project -y"
	aSource  = "run: claude plugin marketplace add https://github.com/drecdroid/claude-base-project-agent-kit.git"
)

func TestNewYesDefaults(t *testing.T) {
	h := newFlowHarness(t)
	dir := filepath.Join(h.projects, "demo")
	if code := h.run("new", "demo", "--template-source", kitTemplate(t), "--description", "demo app", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	assertActions(t, h, dir, []string{aInit, aAdd, aCommit, aInstall})
	b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(b) != "# demo\n\ndemo app\n" {
		t.Errorf("CLAUDE.md = %q", b)
	}
	for _, f := range []string{".gitattributes", ".claude/settings.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
}

func TestNewAddsMarketplaceWhenMissing(t *testing.T) {
	h := newFlowHarness(t)
	h.runner.outs["claude plugin marketplace list --json"] = fakeOut{`[{"name":"claude-plugins-official"}]`, 0}
	dir := filepath.Join(h.projects, "demo")
	if code := h.run("new", "demo", "--template-source", kitTemplate(t), "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	assertActions(t, h, dir, []string{aInit, aAdd, aCommit, aSource, aInstall})

	h2 := newFlowHarness(t)
	h2.runner.outs["claude plugin marketplace list --json"] = fakeOut{`[]`, 0}
	if code := h2.run("new", "demo", "--template-source", kitTemplate(t), "--add-source=false", "--yes"); code != 0 {
		t.Fatal(h2.errb.String())
	}
	assertActions(t, h2, filepath.Join(h2.projects, "demo"), []string{aInit, aAdd, aCommit})
	if !strings.Contains(h2.errb.String(), "skipping plugin install") {
		t.Errorf("stderr: %s", h2.errb.String())
	}
}

func TestNewGitHubFlow(t *testing.T) {
	h := newFlowHarness(t)
	dir := filepath.Join(h.projects, "demo")
	code := h.run("new", "demo", "--template-source", kitTemplate(t), "-d", "demo app", "--github",
		"--topics", "cli,go", "--homepage", "https://x.dev", "--plugin=false", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	assertActions(t, h, dir, []string{
		aInit,
		"run: (in <dir>) gh repo create octo/demo --private --description demo app --homepage https://x.dev --disable-wiki --source <dir> --remote origin",
		"run: (in <dir>) gh repo edit octo/demo --add-topic cli --add-topic go",
		aAdd, aCommit, aPush,
	})
}

func TestNewGitHubOptionsAndNoCommitMeansNoPush(t *testing.T) {
	h := newFlowHarness(t)
	dir := filepath.Join(h.projects, "demo")
	code := h.run("new", "demo", "--template-source", kitTemplate(t), "--github", "--owner", "my-org",
		"--repo-name", "demo-repo", "--visibility", "public", "--disable-wiki=false", "--commit=false", "--plugin=false", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	assertActions(t, h, dir, []string{aInit, "run: (in <dir>) gh repo create my-org/demo-repo --public --source <dir> --remote origin"})
	if !strings.Contains(h.out.String(), "next: git add -A") {
		t.Errorf("missing next-step hint:\n%s", h.out.String())
	}
}

func TestNewGhNotAuthenticatedSkipsRepoOnly(t *testing.T) {
	h := newFlowHarness(t)
	h.runner.outs["gh auth status"] = fakeOut{"You are not logged into any GitHub hosts", 1}
	dir := filepath.Join(h.projects, "demo")
	if code := h.run("new", "demo", "--template-source", kitTemplate(t), "--github", "--plugin=false", "--yes"); code != 0 {
		t.Fatalf("must not fail the whole flow: exit %d: %s", code, h.errb.String())
	}
	assertActions(t, h, dir, []string{aInit, aAdd, aCommit})
	if !strings.Contains(h.errb.String(), "gh auth login") {
		t.Errorf("stderr must give the exact login command: %s", h.errb.String())
	}
}

func TestNewInteractiveAnswers(t *testing.T) {
	h := newFlowHarness(t)
	h.tty = true
	sp := &scriptPrompter{
		inputs:   map[string]string{"Description": "typed desc", "Prompt for Claude": "hello & 100%"},
		confirms: map[string]bool{"GitHub": false, "first commit": true, "plugin": false},
		selects:  map[string]string{"Open the project in": "claude"},
	}
	h.script = sp
	h.goos = "linux"
	dir := filepath.Join(h.projects, "demo")
	if code := h.run("new", "demo", "--template-source", kitTemplate(t)); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	wantAsked := []string{"Description (one line)", "Create a GitHub repository?",
		"Make the first commit (\"chore: init from agent-kit template\")?",
		"Install the agent-kit plugin for this project (project scope)?", "Open the project in", "Prompt for Claude"}
	if !reflect.DeepEqual(sp.asked, wantAsked) {
		t.Fatalf("asked %q", sp.asked)
	}
	// No plugin install (answered no); the typed prompt reaches the deep link.
	var runs []string
	for _, c := range h.runner.calls {
		if c.kind != "output" {
			runs = append(runs, c.kind+": "+strings.Join(c.cmd.Argv(), " "))
		}
	}
	want := []string{
		"run: git init -b main", "run: git add -A", "run: git commit -m " + initCommitMessage,
		"start: xdg-open " + launch.ClaudeCodeURL(dir, "hello & 100%"),
	}
	if !reflect.DeepEqual(runs, want) {
		t.Fatalf("runs:\n  got  %q\n  want %q", runs, want)
	}
	if !strings.HasSuffix(want[3], "&q=hello%20%26%20100%25") {
		t.Fatalf("prompt encoding: %s", want[3])
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); !strings.Contains(string(b), "typed desc") {
		t.Errorf("CLAUDE.md = %q", b)
	}
}

func TestNewDryRunFullFlowCreatesNothing(t *testing.T) {
	h := newFlowHarness(t)
	h.runner.outs["claude plugin marketplace list --json"] = fakeOut{`[]`, 0}
	dir := filepath.Join(h.projects, "demo")
	code := h.run("new", "demo", "-d", "demo app", "--github", "--topics", "cli", "--open", "code", "--dry-run", "--yes")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("dry-run created the folder")
	}
	for _, c := range h.runner.calls {
		if c.kind != "output" {
			t.Fatalf("dry-run ran %q", c.cmd.Argv())
		}
	}
	out := h.out.String()
	for _, s := range []string{
		"[dry-run] fetch template https://codeload.github.com/drecdroid/claude-base-project-agent-kit/tar.gz/main",
		"[dry-run] mkdir " + dir,
		"[dry-run] git init -b main",
		"[dry-run] gh repo create octo/demo --private --description 'demo app' --disable-wiki --source " + dir + " --remote origin",
		"[dry-run] gh repo edit octo/demo --add-topic cli",
		"[dry-run] git commit -m 'chore: init from agent-kit template'",
		"[dry-run] git push -u origin main",
		"[dry-run] claude plugin marketplace add https://github.com/drecdroid/claude-base-project-agent-kit.git",
		"[dry-run] claude plugin install agent-kit@claude-base-project-agent-kit --scope project -y",
		"[dry-run] code " + dir,
		"[dry-run] nothing was created",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}
}

func TestNewFailureMidwayKeepsFolderAndReports(t *testing.T) {
	h := newFlowHarness(t)
	h.runner.codes = map[string]int{"git commit -m chore: init from agent-kit template": 1}
	dir := filepath.Join(h.projects, "demo")
	code := h.run("new", "demo", "--template-source", kitTemplate(t), "--yes")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	assertActions(t, h, dir, []string{aInit, aAdd, aCommit}) // plugin install never ran
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal("folder must be kept")
	}
	e := h.errb.String()
	for _, s := range []string{`step "git commit" failed`, "done:     fetch template, create folder, git init, write template, fill placeholders, git add",
		"not done: install plugin", "Remove-Item -Recurse -Force '" + dir + "'"} {
		if !strings.Contains(e, s) {
			t.Errorf("missing %q in:\n%s", s, e)
		}
	}
}

func TestNewValidation(t *testing.T) {
	h := newFlowHarness(t)
	os.WriteFile(filepath.Join(h.projects, "tdm-app", "x"), []byte("x"), 0o644)
	cases := map[string][]string{
		"not empty":             {"new", "tdm-app", "--yes"},
		"use letters":           {"new", "a b", "--yes"},
		"reserved":              {"new", "CON", "--yes"},
		"project name required": {"new", "--yes"},
		"--ref only applies":    {"new", "demo", "--template-source", kitTemplate(t), "--ref", "v1", "--yes"},
		"--visibility":          {"new", "demo", "--template-source", kitTemplate(t), "--github", "--visibility", "secret", "--yes"},
		"--open":                {"new", "demo", "--template-source", kitTemplate(t), "--open", "vim", "--yes"},
	}
	for want, args := range cases {
		if code := h.run(args...); code != 2 || !strings.Contains(h.errb.String(), want) {
			t.Errorf("%v: exit %d, stderr %q (want %q)", args, code, h.errb.String(), want)
		}
	}
	for _, c := range h.runner.calls {
		if c.kind != "output" {
			t.Fatalf("validation failure ran %q", c.cmd.Argv())
		}
	}
}

func TestNewPreflightFailsBeforeCreating(t *testing.T) {
	h := newFlowHarness(t)
	delete(h.runner.paths, "git")
	dir := filepath.Join(h.projects, "demo")
	if code := h.run("new", "demo", "--template-source", kitTemplate(t), "--yes"); code != 1 || !strings.Contains(h.errb.String(), "preflight failed, nothing was created") {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("folder created despite failed preflight")
	}
	// claude is only required when installing the plugin.
	h2 := newFlowHarness(t)
	delete(h2.runner.paths, "claude")
	if code := h2.run("new", "demo", "--template-source", kitTemplate(t), "--plugin=false", "--yes"); code != 0 {
		t.Fatalf("plugin off must not need claude: %s", h2.errb.String())
	}
}

func TestNewTemplateDownloadFailsBeforeCreating(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	h := newFlowHarness(t)
	dir := filepath.Join(h.projects, "demo")
	code := h.run("new", "demo", "--template-source", srv.URL+"/kit.tar.gz", "--yes")
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	e := h.errb.String()
	if !strings.Contains(e, "private or missing") || !strings.Contains(e, "--template-source") {
		t.Errorf("stderr: %s", e)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("folder created before the template was fetched")
	}
	if len(actions(h, dir)) != 0 {
		t.Fatalf("ran %q", actions(h, dir))
	}
}
