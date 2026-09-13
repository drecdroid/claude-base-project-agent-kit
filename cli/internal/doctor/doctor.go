// Package doctor checks the tools ckit drives and shapes the result for
// humans and --json. Probing goes through a small interface so result shaping
// is unit-testable without any of the tools installed.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/kit"
)

// Status of one check.
type Status string

const (
	OK      Status = "ok"
	Missing Status = "missing"
	Warn    Status = "warn" // present but not in the wanted state
	Info    Status = "info" // informational, never fails
)

// Check is one line of the report.
type Check struct {
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Required bool   `json:"required"`
	Version  string `json:"version,omitempty"`
	Path     string `json:"path,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// Report is the whole doctor output.
type Report struct {
	OK     bool     `json:"ok"`
	For    []string `json:"for"`
	Checks []Check  `json:"checks"`
}

// Probe is what doctor needs from the outside world.
type Probe interface {
	LookPath(prog string) (string, error)
	// Output runs prog and returns combined output and exit code; err means
	// it could not run at all.
	Output(ctx context.Context, prog string, args ...string) (string, int, error)
	Exists(path string) bool
}

// Options select what is checked and what counts as required.
type Options struct {
	GOOS         string
	CodePath     string // config override
	SmartgitPath string // config override (resolved by caller if empty)
	For          []string
}

// Check names.
const (
	CGit           = "git"
	CGitBranch     = "git.defaultBranch"
	CGh            = "gh"
	CGhAuth        = "gh.auth"
	CClaude        = "claude"
	CMarketplace   = "marketplace"
	CCode          = "code"
	CSmartGit      = "smartgit"
	defaultForList = "source,plugin"
)

// Requirements maps a command set (`--for`) to the checks it needs. "new" is
// part 2's `ckit new`, listed now so doctor already answers for it.
var Requirements = map[string][]string{
	"source":        {CClaude},
	"plugin":        {CClaude, CMarketplace},
	"open-code":     {CCode},
	"open-smartgit": {CSmartGit},
	"open-claude":   {},
	"new":           {CGit, CGh, CGhAuth, CClaude, CMarketplace},
}

// DefaultFor is the command set checked when --for is not given.
func DefaultFor() []string { return strings.Split(defaultForList, ",") }

// ForNames lists valid --for values, sorted.
func ForNames() []string {
	var out []string
	for k := range Requirements {
		out = append(out, k)
	}
	out = append(out, "all")
	sort.Strings(out)
	return out
}

// RequiredSet expands --for values ("all" = every set) into check names.
func RequiredSet(forList []string) (map[string]bool, []string, error) {
	req := map[string]bool{}
	var norm []string
	for _, f := range forList {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "" {
			continue
		}
		if f == "all" {
			for k, v := range Requirements {
				norm = append(norm, k)
				for _, c := range v {
					req[c] = true
				}
			}
			continue
		}
		v, ok := Requirements[f]
		if !ok {
			return nil, nil, fmt.Errorf("unknown --for value %q; valid: %s", f, strings.Join(ForNames(), ", "))
		}
		norm = append(norm, f)
		for _, c := range v {
			req[c] = true
		}
	}
	sort.Strings(norm)
	return req, dedupe(norm), nil
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// Run performs every check and shapes the report.
func Run(ctx context.Context, p Probe, o Options) (Report, error) {
	forList := o.For
	if len(forList) == 0 {
		forList = DefaultFor()
	}
	req, norm, err := RequiredSet(forList)
	if err != nil {
		return Report{}, err
	}
	var cs []Check
	add := func(c Check) { c.Required = req[c.Name]; cs = append(cs, c) }

	// git
	if path, err := p.LookPath("git"); err != nil {
		add(Check{Name: CGit, Status: Missing, Fix: "install git: https://git-scm.com/downloads"})
	} else {
		out, _, _ := p.Output(ctx, "git", "--version")
		add(Check{Name: CGit, Status: OK, Path: path, Version: firstLine(out)})
		b, code, _ := p.Output(ctx, "git", "config", "--global", "init.defaultBranch")
		if code == 0 && strings.TrimSpace(b) != "" {
			add(Check{Name: CGitBranch, Status: Info, Detail: "init.defaultBranch=" + strings.TrimSpace(b)})
		} else {
			add(Check{Name: CGitBranch, Status: Info, Detail: "init.defaultBranch not set (ckit new uses `git init -b main` regardless)",
				Fix: "optional: git config --global init.defaultBranch main"})
		}
	}

	// gh
	if path, err := p.LookPath("gh"); err != nil {
		add(Check{Name: CGh, Status: Missing, Fix: "install GitHub CLI: https://cli.github.com"})
		add(Check{Name: CGhAuth, Status: Missing, Detail: "gh not installed", Fix: "install gh, then: gh auth login"})
	} else {
		out, _, _ := p.Output(ctx, "gh", "--version")
		add(Check{Name: CGh, Status: OK, Path: path, Version: firstLine(out)})
		if _, code, err := p.Output(ctx, "gh", "auth", "status"); err == nil && code == 0 {
			add(Check{Name: CGhAuth, Status: OK, Detail: "logged in"})
		} else {
			add(Check{Name: CGhAuth, Status: Warn, Detail: "not logged in", Fix: "gh auth login && gh auth setup-git"})
		}
	}

	// claude + marketplace
	if path, err := p.LookPath("claude"); err != nil {
		add(Check{Name: CClaude, Status: Missing, Fix: "install Claude Code: https://docs.claude.com/en/docs/claude-code/setup"})
		add(Check{Name: CMarketplace, Status: Missing, Detail: "claude not installed", Fix: "install claude, then: ckit source add"})
	} else {
		out, _, _ := p.Output(ctx, "claude", "--version")
		add(Check{Name: CClaude, Status: OK, Path: path, Version: firstLine(out)})
		add(marketplaceCheck(ctx, p))
	}

	// VS Code
	codeProg := o.CodePath
	if codeProg == "" {
		codeProg = "code"
	}
	if path, err := p.LookPath(codeProg); err != nil {
		add(Check{Name: CCode, Status: Missing, Detail: "`" + codeProg + "` not found",
			Fix: "VS Code: Command Palette > \"Shell Command: Install 'code' command in PATH\", or ckit config set codePath <path>"})
	} else {
		out, _, _ := p.Output(ctx, codeProg, "--version")
		add(Check{Name: CCode, Status: OK, Path: path, Version: firstLine(out)})
	}

	// SmartGit: existence only, never launched.
	sg := o.SmartgitPath
	if sgPath, ok := findSmartGit(p, o.GOOS, sg); ok {
		add(Check{Name: CSmartGit, Status: OK, Path: sgPath})
	} else {
		add(Check{Name: CSmartGit, Status: Missing, Detail: sg + " not found",
			Fix: "install SmartGit (https://www.syntevo.com/smartgit/) or ckit config set smartgitPath <path>"})
	}

	return Shape(norm, cs), nil
}

// findSmartGit: a path that exists, or a bare name on PATH.
func findSmartGit(p Probe, goos, prog string) (string, bool) {
	if prog == "" {
		return "", false
	}
	if strings.ContainsAny(prog, `/\`) {
		return prog, p.Exists(prog)
	}
	path, err := p.LookPath(prog)
	return path, err == nil
}

func marketplaceCheck(ctx context.Context, p Probe) Check {
	c := Check{Name: CMarketplace}
	out, code, err := p.Output(ctx, "claude", "plugin", "marketplace", "list", "--json")
	if err != nil || code != 0 {
		c.Status, c.Detail = Warn, "could not list marketplaces: "+firstLine(out)
		c.Fix = "claude plugin marketplace list"
		return c
	}
	found, perr := HasMarketplace(out, kit.Marketplace)
	switch {
	case perr != nil:
		c.Status, c.Detail = Warn, "unexpected `claude plugin marketplace list --json` output: "+perr.Error()
	case found:
		c.Status, c.Detail = OK, kit.Marketplace+" added"
	default:
		c.Status, c.Detail, c.Fix = Missing, kit.Marketplace+" not added", "ckit source add"
	}
	return c
}

// HasMarketplace parses `claude plugin marketplace list --json`.
func HasMarketplace(jsonOut, name string) (bool, error) {
	s := strings.TrimSpace(jsonOut)
	if i := strings.Index(s, "["); i > 0 {
		s = s[i:] // tolerate a warning line before the JSON
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return false, err
	}
	for _, m := range list {
		if m.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// Shape computes Report.OK: false iff a REQUIRED check is not ok. Info checks
// never fail; a Warn on a required check does.
func Shape(forList []string, cs []Check) Report {
	ok := true
	for _, c := range cs {
		if c.Required && c.Status != OK && c.Status != Info {
			ok = false
		}
	}
	return Report{OK: ok, For: forList, Checks: cs}
}

// WriteText renders the human report.
func WriteText(w io.Writer, r Report) {
	for _, c := range r.Checks {
		mark := map[Status]string{OK: "ok  ", Missing: "MISS", Warn: "WARN", Info: "info"}[c.Status]
		req := " "
		if c.Required {
			req = "*"
		}
		line := fmt.Sprintf("%s %s %-18s", mark, req, c.Name)
		var bits []string
		for _, b := range []string{c.Version, c.Detail, c.Path} {
			if b != "" {
				bits = append(bits, b)
			}
		}
		fmt.Fprintf(w, "%s %s\n", line, strings.Join(bits, " | "))
		if c.Fix != "" && c.Status != OK {
			fmt.Fprintf(w, "         %-18s fix: %s\n", "", c.Fix)
		}
	}
	verdict := "all required checks passed"
	if !r.OK {
		verdict = "missing required tools"
	}
	fmt.Fprintf(w, "\n* = required for: %s (--for). %s\n", strings.Join(r.For, ","), verdict)
}

// OSProbe is the real Probe; runner-backed Output is supplied by the caller.
type OSProbe struct {
	LookPathFn func(string) (string, error)
	OutputFn   func(ctx context.Context, prog string, args ...string) (string, int, error)
}

func (o OSProbe) LookPath(p string) (string, error) { return o.LookPathFn(p) }
func (o OSProbe) Output(ctx context.Context, p string, a ...string) (string, int, error) {
	return o.OutputFn(ctx, p, a...)
}
func (o OSProbe) Exists(path string) bool { _, err := os.Stat(path); return err == nil }
