package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/scaffold"
)

// GhCreateArgs is `gh repo create` for the plan: from the local repo, remote
// origin, NOT pushed (the push is its own later step).
func GhCreateArgs(p *newPlan) []string {
	args := []string{"repo", "create", p.Owner + "/" + p.Repo, "--" + p.Visibility}
	if p.RepoDesc != "" {
		args = append(args, "--description", p.RepoDesc)
	}
	if p.Homepage != "" {
		args = append(args, "--homepage", p.Homepage)
	}
	if p.DisableWiki {
		args = append(args, "--disable-wiki")
	}
	return append(args, "--source", p.Dir, "--remote", "origin")
}

// GhEditTopicsArgs is `gh repo edit` adding topics (nil when there are none).
func GhEditTopicsArgs(p *newPlan) []string {
	if len(p.Topics) == 0 {
		return nil
	}
	args := []string{"repo", "edit", p.Owner + "/" + p.Repo}
	for _, t := range p.Topics {
		args = append(args, "--add-topic", t)
	}
	return args
}

type newStep struct {
	name string
	run  func() error
}

func (a *app) runNew(ctx context.Context, cmd *cli.Command, p *newPlan) error {
	dry := cmd.Bool("dry-run")
	w := a.env.Stdout
	var (
		fetched   *scaffold.Fetched
		files     []string
		ghCreated bool
		steps     []newStep
	)
	add := func(name string, run func() error) { steps = append(steps, newStep{name, run}) }
	git := func(args ...string) error {
		return a.exec(ctx, cmd, execx.Cmd{Dir: p.Dir, Prog: "git", Args: args}, false)
	}

	// Fetch first: a private/missing repo fails before any folder exists.
	add("fetch template", func() error {
		if dry {
			fmt.Fprintf(w, "[dry-run] fetch template %s (only template/**)\n", p.Source)
			return nil
		}
		var err error
		fetched, err = scaffold.Fetch(ctx, a.env.HTTPClient, p.Source)
		return err
	})
	add("create folder", func() error {
		if dry {
			fmt.Fprintf(w, "[dry-run] mkdir %s\n", p.Dir)
			return nil
		}
		return os.MkdirAll(p.Dir, 0o755)
	})
	add("git init", func() error { return git("init", "-b", "main") })
	add("write template", func() error {
		if dry {
			fmt.Fprintf(w, "[dry-run] write template files (dotfiles included) into %s\n", p.Dir)
			return nil
		}
		var err error
		files, err = fetched.WriteTo(p.Dir)
		if err == nil {
			fmt.Fprintf(w, "template: %s\n", strings.Join(files, ", "))
		}
		return err
	})
	add("fill placeholders", func() error {
		if dry {
			fmt.Fprintf(w, "[dry-run] fill %s=%q %s=%q\n", scaffold.PHProjectName, p.Name, scaffold.PHDescription, p.Description)
			return nil
		}
		_, err := scaffold.Fill(p.Dir, files, scaffold.Values{ProjectName: p.Name, Description: p.Description})
		return err
	})
	if p.GitHub {
		add("gh repo create", func() error {
			err := a.exec(ctx, cmd, execx.Cmd{Dir: p.Dir, Prog: "gh", Args: GhCreateArgs(p)}, false)
			ghCreated = err == nil && !dry
			return err
		})
		if topics := GhEditTopicsArgs(p); topics != nil {
			add("gh repo edit (topics)", func() error {
				return a.exec(ctx, cmd, execx.Cmd{Dir: p.Dir, Prog: "gh", Args: topics}, false)
			})
		}
	}
	// Plugin BEFORE the first commit: `claude plugin install --scope project`
	// rewrites .claude/settings.json (confirmed live), which otherwise leaves
	// the brand-new repo dirty right after its first commit.
	if p.Plugin {
		if p.AddSource {
			add("add kit marketplace", func() error {
				args, _ := SourceArgs("add")
				return a.exec(ctx, cmd, execx.Cmd{Prog: "claude", Args: args}, false)
			})
		}
		add("install plugin", func() error {
			args, _ := PluginArgs("install", "project", false)
			return a.exec(ctx, cmd, execx.Cmd{Dir: p.Dir, Prog: "claude", Args: args}, false)
		})
	}
	if p.Commit {
		add("git add", func() error { return git("add", "-A") })
		add("git commit", func() error { return git("commit", "-m", initCommitMessage) })
		if p.Push {
			add("git push", func() error { return git("push", "-u", "origin", "main") })
		}
	}
	if p.Open != "none" {
		add("open in "+p.Open, func() error { return a.openDir(ctx, cmd, p.Open, p.Dir, p.Prompt) })
	}

	for i, s := range steps {
		fmt.Fprintf(w, "==> %s\n", s.name)
		if err := s.run(); err != nil {
			return a.newFailure(p, steps, i, err, ghCreated)
		}
	}
	if dry {
		fmt.Fprintln(w, "[dry-run] nothing was created")
		return nil
	}
	fmt.Fprintf(w, "\ncreated %s\n", p.Dir)
	if !p.Commit {
		fmt.Fprintln(w, "  next: git add -A && git commit -m \""+initCommitMessage+"\"")
	}
	if p.GitHub && !p.Push {
		fmt.Fprintln(w, "  push when ready: git push -u origin main")
	}
	return nil
}

// newFailure stops the flow: what was done, what wasn't, and how to clean up
// by hand. ckit never deletes the folder itself.
func (a *app) newFailure(p *newPlan, steps []newStep, i int, err error, ghCreated bool) error {
	names := func(ss []newStep) string {
		if len(ss) == 0 {
			return "(nothing)"
		}
		var n []string
		for _, s := range ss {
			n = append(n, s.name)
		}
		return strings.Join(n, ", ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "step %q failed: %v\n", steps[i].name, err)
	fmt.Fprintf(&b, "  done:     %s\n", names(steps[:i]))
	fmt.Fprintf(&b, "  not done: %s\n", names(steps[i+1:]))
	if i > 1 { // the folder step ran
		fmt.Fprintf(&b, "  the folder was kept; fix the cause and finish by hand, or remove it: %s\n", removeCommand(a.env.GOOS, p.Dir))
	}
	if ghCreated {
		fmt.Fprintf(&b, "  the GitHub repo %s/%s exists; keep it, or: gh repo delete %s/%s --yes\n", p.Owner, p.Repo, p.Owner, p.Repo)
	}
	code := 1
	var ec cli.ExitCoder
	if errors.As(err, &ec) && ec.ExitCode() != 0 {
		code = ec.ExitCode()
	}
	return cli.Exit(strings.TrimRight(b.String(), "\n"), code)
}

func removeCommand(goos, dir string) string {
	if goos == "windows" {
		return "Remove-Item -Recurse -Force '" + strings.ReplaceAll(dir, "'", "''") + "'"
	}
	return "rm -rf " + quoteArg(dir)
}

func quoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// openDir launches (or dry-runs) one app for dir, shared by `open` and `new`.
func (a *app) openDir(ctx context.Context, cmd *cli.Command, appName, dir, prompt string) error {
	c, _, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	var x execx.Cmd
	switch appName {
	case "code":
		x = launch.Code(c.CodePath, dir)
	case "smartgit":
		x = launch.SmartGit(a.env.GOOS, c.SmartgitPath, dir)
		if !cmd.Bool("dry-run") {
			if err := a.checkSmartGit(c.SmartgitPath); err != nil {
				return err
			}
		}
	case "claude":
		u := launch.ClaudeCodeURL(dir, prompt)
		x = launch.OpenURL(a.env.GOOS, u)
		if cmd.Bool("dry-run") {
			fmt.Fprintf(a.env.Stdout, "[dry-run] url: %s\n", u)
		}
	default:
		return launch.ValidateApp(appName)
	}
	if err := a.exec(ctx, cmd, x, true); err != nil {
		return err
	}
	if !cmd.Bool("dry-run") && appName == "claude" {
		fmt.Fprintln(a.env.Stdout, "opened Claude Desktop; confirm the folder-trust prompt there")
	}
	return nil
}
