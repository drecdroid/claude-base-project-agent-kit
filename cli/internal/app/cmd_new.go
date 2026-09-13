package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/kit"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/scaffold"
)

const (
	defaultClaudePrompt = "Fill in CLAUDE.md for this project"
	initCommitMessage   = "chore: init from agent-kit template"
)

var (
	openChoices  = []string{"none", "code", "smartgit", "claude"}
	visibilities = []string{"private", "public", "internal"}
)

// newPlan is every answer `ckit new` needs, gathered (and preflighted)
// BEFORE anything is created.
type newPlan struct {
	Name, Dir, Description string
	Source                 scaffold.Source

	GitHub                            bool
	Owner, Repo, Visibility, RepoDesc string
	Homepage                          string
	Topics                            []string
	DisableWiki, Commit, Push         bool
	Plugin, AddSource                 bool
	Open, Prompt                      string
}

func (a *app) newCommand() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "create a project from the kit template: git init, template, GitHub repo, first commit, plugin, open",
		ArgsUsage: "[name]",
		UsageText: "ckit new                                  # interactive (terminal)\n" +
			"ckit new my-app -d \"what it is\" --yes     # all defaults, no prompts\n" +
			"ckit new my-app --github --visibility public --topics cli,go --yes\n" +
			"ckit new my-app --template-source ../claude-base-project-agent-kit --plugin=false --yes\n" +
			"ckit new my-app --github --open claude --dry-run\n\n" +
			"Defaults with --yes: no GitHub repo, first commit yes, push yes (only with --github),\n" +
			"plugin install yes (adding the kit marketplace first if missing), open none.\n" +
			"Template: the kit's template/ from the GitHub tarball (--ref, default main), or --template-source.\n" +
			"Placeholders filled in template files: {{project_name}}, {{description}}.\n" +
			"On a failure ckit stops, lists what was and wasn't done, and never deletes the folder.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "dir", Usage: "target directory (default <projectsDir>/<name>)"},
			&cli.StringFlag{Name: "description", Aliases: []string{"d"}, Usage: "one-line description ({{description}})"},
			&cli.StringFlag{Name: "template-source", Usage: "local kit checkout or template dir, or a .tar.gz URL (default: kit GitHub tarball)"},
			&cli.StringFlag{Name: "ref", Usage: "kit branch, tag or sha for the default template source (default main)"},
			&cli.BoolFlag{Name: "github", Usage: "create a GitHub repository with gh (skipped with a hint if gh is not logged in)"},
			&cli.StringFlag{Name: "owner", Usage: "GitHub user or org (default: the gh user)"},
			&cli.StringFlag{Name: "repo-name", Usage: "GitHub repository name (default: project name)"},
			&cli.StringFlag{Name: "visibility", Value: "private", Usage: "private, public or internal"},
			&cli.StringFlag{Name: "repo-description", Usage: "GitHub description (default: --description)"},
			&cli.StringFlag{Name: "homepage", Usage: "GitHub homepage URL"},
			&cli.StringSliceFlag{Name: "topics", Usage: "GitHub topics, comma-separated"},
			&cli.BoolFlag{Name: "disable-wiki", Value: true, Usage: "disable the GitHub wiki"},
			&cli.BoolFlag{Name: "commit", Value: true, Usage: "make the first commit"},
			&cli.BoolFlag{Name: "push", Value: true, Usage: "git push -u origin main after the first commit (GitHub only)"},
			&cli.BoolFlag{Name: "plugin", Value: true, Usage: "install " + kit.PluginRef() + " at project scope"},
			&cli.BoolFlag{Name: "add-source", Value: true, Usage: "add the kit marketplace first if it is missing"},
			&cli.StringFlag{Name: "open", Value: "none", Usage: "none, code, smartgit or claude"},
			&cli.StringFlag{Name: "prompt", Value: defaultClaudePrompt, Usage: "prompt for --open claude"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			p, err := a.gatherNew(ctx, cmd)
			if err != nil {
				return err
			}
			if err := a.preflightNew(ctx, p); err != nil {
				return err
			}
			return a.runNew(ctx, cmd, p)
		},
	}
}

// ---- asking: a flag given explicitly, --yes, or no TTY = no prompt ----

func (a *app) askString(cmd *cli.Command, flag, title, def string, validate func(string) error) (string, error) {
	if !a.interactive(cmd) || cmd.IsSet(flag) {
		if validate != nil {
			if err := validate(def); err != nil {
				return "", cli.Exit(fmt.Sprintf("--%s: %v", flag, err), 2)
			}
		}
		return def, nil
	}
	return a.env.Prompter.Input(title, "", def, validate)
}

func (a *app) askBool(cmd *cli.Command, flag, title string) (bool, error) {
	def := cmd.Bool(flag)
	if !a.interactive(cmd) || cmd.IsSet(flag) {
		return def, nil
	}
	return a.env.Prompter.Confirm(title, def)
}

func (a *app) askChoice(cmd *cli.Command, flag, title string, options []string) (string, error) {
	def := strings.ToLower(cmd.String(flag))
	ok := false
	for _, o := range options {
		ok = ok || o == def
	}
	if !ok {
		return "", cli.Exit(fmt.Sprintf("--%s %q: choose one of %s", flag, def, strings.Join(options, ", ")), 2)
	}
	if !a.interactive(cmd) || cmd.IsSet(flag) {
		return def, nil
	}
	ordered := []string{def} // default first
	for _, o := range options {
		if o != def {
			ordered = append(ordered, o)
		}
	}
	return a.env.Prompter.Select(title, ordered)
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func nonEmpty(what string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
}

func (a *app) output(ctx context.Context, prog string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return a.env.Runner.Output(ctx, execx.Cmd{Prog: prog, Args: args})
}
