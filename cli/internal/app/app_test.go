package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

type call struct {
	kind string // run | start | output
	cmd  execx.Cmd
}

type fakeRunner struct {
	calls []call
	code  int
	paths map[string]string
}

func (f *fakeRunner) Run(_ context.Context, c execx.Cmd) (int, error) {
	f.calls = append(f.calls, call{"run", c})
	return f.code, nil
}
func (f *fakeRunner) Start(_ context.Context, c execx.Cmd) error {
	f.calls = append(f.calls, call{"start", c})
	return nil
}
func (f *fakeRunner) Output(_ context.Context, c execx.Cmd) (string, int, error) {
	f.calls = append(f.calls, call{"output", c})
	return "", 1, nil
}
func (f *fakeRunner) LookPath(p string) (string, error) {
	if v, ok := f.paths[p]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

type fakePrompter struct {
	selectAnswer  string
	confirmAnswer bool
	asked         []string
}

func (p *fakePrompter) Select(title string, _ []string) (string, error) {
	p.asked = append(p.asked, title)
	return p.selectAnswer, nil
}
func (p *fakePrompter) Confirm(title string) (bool, error) {
	p.asked = append(p.asked, title)
	return p.confirmAnswer, nil
}
func (p *fakePrompter) EditConfig(c *config.Config, _, _ string) error {
	c.ProjectsDir = "  ~/edited  "
	return nil
}

type harness struct {
	home, projects, cwd string
	out, errb           bytes.Buffer
	runner              *fakeRunner
	prompter            *fakePrompter
	tty                 bool
	goos                string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{home: t.TempDir(), runner: &fakeRunner{}, prompter: &fakePrompter{}, goos: "windows"}
	h.projects = filepath.Join(h.home, "Projects")
	h.cwd = filepath.Join(h.projects, "tdm-app")
	for _, d := range []string{"tdm-app", "My App & 100%"} {
		os.MkdirAll(filepath.Join(h.projects, d), 0o755)
	}
	return h
}

func (h *harness) run(args ...string) int {
	h.out.Reset()
	h.errb.Reset()
	env := Env{
		Stdin: strings.NewReader(""), Stdout: &h.out, Stderr: &h.errb,
		Runner: h.runner, GOOS: h.goos,
		Getwd:       func() (string, error) { return h.cwd, nil },
		Home:        func() (string, error) { return h.home, nil },
		Interactive: func() bool { return h.tty },
		Prompter:    h.prompter,
	}
	return Main(context.Background(), env, append([]string{"ckit"}, args...))
}

func (h *harness) lastCall(t *testing.T) call {
	t.Helper()
	if len(h.runner.calls) == 0 {
		t.Fatalf("no command ran; stdout=%s stderr=%s", h.out.String(), h.errb.String())
	}
	return h.runner.calls[len(h.runner.calls)-1]
}

func TestSourceCommands(t *testing.T) {
	h := newHarness(t)
	cases := map[string][]string{
		// HTTPS git URL, never the owner/repo shorthand (claude clones that over
		// SSH: fails without github.com in known_hosts, confirmed live).
		"add":    {"claude", "plugin", "marketplace", "add", "https://github.com/drecdroid/claude-base-project-agent-kit.git"},
		"update": {"claude", "plugin", "marketplace", "update", "claude-base-project-agent-kit"},
		"remove": {"claude", "plugin", "marketplace", "remove", "claude-base-project-agent-kit"},
	}
	for action, want := range cases {
		if code := h.run("source", action, "--yes"); code != 0 {
			t.Fatalf("%s: exit %d %s", action, code, h.errb.String())
		}
		if c := h.lastCall(t); c.kind != "run" || !reflect.DeepEqual(c.cmd.Argv(), want) {
			t.Errorf("%s: got %s %q", action, c.kind, c.cmd.Argv())
		}
	}
}

func TestSourceRemoveNeedsYesWithoutTTY(t *testing.T) {
	h := newHarness(t)
	if code := h.run("source", "remove"); code != 2 || len(h.runner.calls) != 0 {
		t.Fatalf("exit %d calls %v", code, h.runner.calls)
	}
	if !strings.Contains(h.errb.String(), "--yes") {
		t.Errorf("stderr: %s", h.errb.String())
	}
	h.tty = true
	h.prompter.confirmAnswer = false
	if code := h.run("source", "remove"); code != 1 || len(h.runner.calls) != 0 {
		t.Fatalf("declined confirm must not run: exit %d", code)
	}
}

func TestPluginInstallDryRunInProjectDir(t *testing.T) {
	h := newHarness(t)
	h.runner.paths = map[string]string{"claude": `C:\Users\x\AppData\Roaming\npm\claude.cmd`}
	if code := h.run("plugin", "install", "--dry-run"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	if len(h.runner.calls) != 0 {
		t.Fatal("dry-run must not run anything")
	}
	out := h.out.String()
	for _, s := range []string{
		"[dry-run] in " + h.cwd,
		"[dry-run] claude plugin install agent-kit@claude-base-project-agent-kit --scope project -y",
		"batch shim",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}
}

func TestPluginScopesAndProjectName(t *testing.T) {
	h := newHarness(t)
	h.cwd = h.home
	if code := h.run("plugin", "update", "tdm-app", "--scope", "local"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	c := h.lastCall(t)
	want := []string{"claude", "plugin", "update", "agent-kit@claude-base-project-agent-kit", "--scope", "local"}
	if !reflect.DeepEqual(c.cmd.Argv(), want) || c.cmd.Dir != filepath.Join(h.projects, "tdm-app") {
		t.Errorf("got %q in %s", c.cmd.Argv(), c.cmd.Dir)
	}
	if code := h.run("plugin", "uninstall", "--yes", "--scope", "user"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := h.lastCall(t).cmd.Argv(); !reflect.DeepEqual(got[1:], []string{"plugin", "uninstall", "agent-kit@claude-base-project-agent-kit", "--scope", "user", "-y"}) {
		t.Errorf("uninstall: %q", got)
	}
	if code := h.run("plugin", "install", "--scope", "global"); code != 2 {
		t.Errorf("bad scope exit %d", code)
	}
}

func TestPluginChildExitCodePropagates(t *testing.T) {
	h := newHarness(t)
	h.runner.code = 3
	if code := h.run("plugin", "install"); code != 3 {
		t.Fatalf("exit %d", code)
	}
}

func TestOpenClaudeDryRunURL(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.projects, "My App & 100%")
	if code := h.run("open", "claude", dir, "--prompt", "hi & 100%", "--dry-run"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	out := h.out.String()
	wantURL := launch.ClaudeCodeURL(dir, "hi & 100%")
	if !strings.HasSuffix(wantURL, "&q=hi%20%26%20100%25") || !strings.Contains(wantURL, "My%20App%20%26%20100%25") {
		t.Fatalf("encoding: %s", wantURL)
	}
	if !strings.Contains(out, "[dry-run] url: "+wantURL+"\n") {
		t.Errorf("url line missing; want %s in:\n%s", wantURL, out)
	}
	if !strings.Contains(out, "[dry-run] rundll32 url.dll,FileProtocolHandler "+wantURL) {
		t.Errorf("launcher line missing:\n%s", out)
	}
	if len(h.runner.calls) != 0 {
		t.Fatal("dry-run launched something")
	}
}

func TestOpenCodeStartsDetachedAndSmartGitOverride(t *testing.T) {
	h := newHarness(t)
	if code := h.run("open", "code"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	if c := h.lastCall(t); c.kind != "start" || !reflect.DeepEqual(c.cmd.Argv(), []string{"code", h.cwd}) {
		t.Errorf("got %s %q", c.kind, c.cmd.Argv())
	}
	sg := filepath.Join(h.home, "smartgit.exe")
	os.WriteFile(sg, []byte("x"), 0o755)
	if code := h.run("config", "set", "smartgitPath", sg); code != 0 {
		t.Fatal(h.errb.String())
	}
	if code := h.run("open", "smartgit", "tdm-app"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errb.String())
	}
	if got := h.lastCall(t).cmd.Argv(); !reflect.DeepEqual(got, []string{sg, filepath.Join(h.projects, "tdm-app")}) {
		t.Errorf("smartgit: %q", got)
	}
}

func TestOpenErrorsNeverDeadEnd(t *testing.T) {
	h := newHarness(t)
	if code := h.run("open"); code != 2 || !strings.Contains(h.errb.String(), "code|smartgit|claude") {
		t.Errorf("no app: %d %s", code, h.errb.String())
	}
	if code := h.run("open", "code", "tdm"); code == 0 || !strings.Contains(h.errb.String(), "did you mean: tdm-app") {
		t.Errorf("typo: %d %s", code, h.errb.String())
	}
	if code := h.run("open", "code", "--prompt", "x"); code != 2 {
		t.Errorf("--prompt on code: %d", code)
	}
	// In a TTY the miss becomes a picker.
	h.tty = true
	h.prompter.selectAnswer = "tdm-app"
	if code := h.run("open", "code", "tdm"); code != 0 {
		t.Fatalf("picker: %d %s", code, h.errb.String())
	}
	if got := h.lastCall(t).cmd.Args[0]; got != filepath.Join(h.projects, "tdm-app") {
		t.Errorf("picked dir %s", got)
	}
	// --yes disables prompts even in a TTY.
	if code := h.run("--yes", "open", "code", "tdm"); code == 0 {
		t.Error("--yes must not prompt")
	}
}

func TestConfigSetShowGetInTempHome(t *testing.T) {
	h := newHarness(t)
	scratch := filepath.Join(h.home, "scratch")
	if code := h.run("config", "set", "projectsDir", scratch); code != 0 {
		t.Fatalf("set: %s", h.errb.String())
	}
	p := filepath.Join(h.home, ".ckit", "config.json")
	b, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(b), `"projectsDir"`) {
		t.Fatalf("file %s: %v %s", p, err, b)
	}
	h.run("config")
	for _, s := range []string{"config file: " + p, "projectsDir   " + scratch + "  (set)", "codePath      code  (default)"} {
		if !strings.Contains(h.out.String(), s) {
			t.Errorf("show missing %q:\n%s", s, h.out.String())
		}
	}
	h.run("config", "get", "projectsDir")
	if strings.TrimSpace(h.out.String()) != scratch {
		t.Errorf("get: %q", h.out.String())
	}
	if code := h.run("config", "set", "nope", "x"); code != 2 {
		t.Errorf("unknown key exit %d", code)
	}
	// dry-run does not write.
	h.run("config", "set", "codePath", "x", "--dry-run")
	if b2, _ := os.ReadFile(p); !bytes.Equal(b, b2) {
		t.Error("dry-run wrote the file")
	}
}

func TestConfigEditNeedsTTY(t *testing.T) {
	h := newHarness(t)
	if code := h.run("config", "edit"); code != 2 || !strings.Contains(h.errb.String(), "config set") {
		t.Errorf("non-tty: %d %s", code, h.errb.String())
	}
	h.tty = true
	if code := h.run("config", "edit"); code != 0 {
		t.Fatal(h.errb.String())
	}
	c, _ := config.Load(config.Path(h.home))
	if c.ProjectsDir != "~/edited" {
		t.Errorf("edited value not trimmed/saved: %q", c.ProjectsDir)
	}
}

func TestDoctorJSONExitCode(t *testing.T) {
	h := newHarness(t)
	code := h.run("doctor", "--json", "--for", "open-claude")
	if code != 0 || !strings.Contains(h.out.String(), `"ok": true`) {
		t.Errorf("open-claude needs nothing: %d %s", code, h.out.String())
	}
	if code := h.run("doctor"); code != 1 || !strings.Contains(h.out.String(), "missing required tools") {
		t.Errorf("default set with no claude: %d %s", code, h.out.String())
	}
	if code := h.run("doctor", "--for", "bogus"); code != 2 {
		t.Errorf("bogus --for: %d", code)
	}
}
