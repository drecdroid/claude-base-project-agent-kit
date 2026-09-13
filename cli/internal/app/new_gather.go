package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/doctor"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/kit"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/scaffold"
)

// gatherNew asks (or reads flags for) every step, in flow order. Nothing is
// created here; the only commands run are read-only probes.
func (a *app) gatherNew(ctx context.Context, cmd *cli.Command) (*newPlan, error) {
	if cmd.NArg() > 1 {
		return nil, cli.Exit("usage: ckit new [name] [flags] (see ckit new --help)", 2)
	}
	c, _, home, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	cwd, err := a.env.Getwd()
	if err != nil {
		return nil, err
	}
	pd := c.ResolvedProjectsDir(home)
	dirFlag := cmd.String("dir")
	if dirFlag != "" {
		d := config.ExpandHome(dirFlag, home)
		if !filepath.IsAbs(d) {
			d = filepath.Join(cwd, d)
		}
		dirFlag = filepath.Clean(d)
	}
	target := func(name string) string {
		if dirFlag != "" {
			return dirFlag
		}
		return filepath.Join(pd, name)
	}
	validName := func(n string) error {
		if err := scaffold.ValidateName(n); err != nil {
			return err
		}
		return scaffold.CheckTarget(target(n))
	}

	p := &newPlan{}
	// 1. name -> target dir
	name := cmd.Args().First()
	if name == "" && dirFlag != "" {
		name = filepath.Base(dirFlag)
	}
	if cmd.Args().First() == "" && a.interactive(cmd) {
		if name, err = a.env.Prompter.Input("Project name", "created under "+pd, name, validName); err != nil {
			return nil, err
		}
	}
	if name == "" {
		return nil, cli.Exit("project name required: ckit new <name> (or --dir <path>)", 2)
	}
	if err := validName(name); err != nil {
		return nil, cli.Exit(err.Error(), 2)
	}
	p.Name, p.Dir = name, target(name)

	if p.Description, err = a.askString(cmd, "description", "Description (one line)", cmd.String("description"), nil); err != nil {
		return nil, err
	}
	p.Description = scaffold.OneLine(p.Description)

	if p.Source, err = scaffold.ResolveSource(cmd.String("template-source"), cmd.String("ref")); err != nil {
		return nil, cli.Exit(err.Error(), 2)
	}

	// 6. GitHub repo
	if p.GitHub, err = a.askBool(cmd, "github", "Create a GitHub repository?"); err != nil {
		return nil, err
	}
	if p.GitHub && !a.ghReady(ctx, p) {
		p.GitHub = false
	}
	if p.GitHub {
		if err := a.gatherGitHub(ctx, cmd, p); err != nil {
			return nil, err
		}
	}

	// 7. first commit (+ push)
	if p.Commit, err = a.askBool(cmd, "commit", "Make the first commit (\""+initCommitMessage+"\")?"); err != nil {
		return nil, err
	}
	if p.GitHub && p.Commit {
		if p.Push, err = a.askBool(cmd, "push", "Push to GitHub (git push -u origin main)?"); err != nil {
			return nil, err
		}
	}

	// 8. plugin (+ marketplace)
	if p.Plugin, err = a.askBool(cmd, "plugin", "Install the agent-kit plugin for this project (project scope)?"); err != nil {
		return nil, err
	}
	if p.Plugin {
		if added, known := a.marketplaceAdded(ctx); known && !added {
			if p.AddSource, err = a.askBool(cmd, "add-source", "The kit marketplace isn't added yet. Add it first ("+kit.RepoGitURL+")?"); err != nil {
				return nil, err
			}
			if !p.AddSource {
				fmt.Fprintln(a.env.Stderr, "ckit: skipping plugin install: the kit marketplace is not added (ckit source add)")
				p.Plugin = false
			}
		}
	}

	// 9. open
	if p.Open, err = a.askChoice(cmd, "open", "Open the project in", openChoices); err != nil {
		return nil, err
	}
	p.Prompt = cmd.String("prompt")
	if p.Open == "claude" {
		if p.Prompt, err = a.askString(cmd, "prompt", "Prompt for Claude", p.Prompt, nil); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// ghReady: gh installed and logged in. Otherwise print how to fix it and
// return false, so the rest of the flow still runs without the repo.
func (a *app) ghReady(ctx context.Context, p *newPlan) bool {
	if _, err := a.env.Runner.LookPath("gh"); err != nil {
		fmt.Fprintln(a.env.Stderr, "ckit: gh is not installed; skipping the GitHub repository (install: https://cli.github.com)")
		return false
	}
	if _, code, err := a.output(ctx, "gh", "auth", "status"); err != nil || code != 0 {
		fmt.Fprintf(a.env.Stderr, "ckit: gh is not logged in; skipping the GitHub repository.\n"+
			"  log in:        gh auth login\n"+
			"  then, later:   gh repo create <owner>/%s --private --source %s --remote origin --push\n", p.Name, quoteArg(p.Dir))
		return false
	}
	return true
}

func (a *app) gatherGitHub(ctx context.Context, cmd *cli.Command, p *newPlan) error {
	var err error
	owner := cmd.String("owner")
	if owner == "" {
		if out, code, oerr := a.output(ctx, "gh", "api", "user", "--jq", ".login"); oerr == nil && code == 0 {
			owner = strings.TrimSpace(out)
		}
	}
	repo := cmd.String("repo-name")
	if repo == "" {
		repo = p.Name
	}
	if p.Repo, err = a.askString(cmd, "repo-name", "GitHub repository name", repo, scaffold.ValidateName); err != nil {
		return err
	}
	if p.Owner, err = a.askString(cmd, "owner", "GitHub owner (user or org)", owner, nonEmpty("GitHub owner (could not read the gh user; pass --owner)")); err != nil {
		return err
	}
	if p.Visibility, err = a.askChoice(cmd, "visibility", "Visibility", visibilities); err != nil {
		return err
	}
	desc := cmd.String("repo-description")
	if desc == "" {
		desc = p.Description
	}
	if p.RepoDesc, err = a.askString(cmd, "repo-description", "GitHub description", desc, nil); err != nil {
		return err
	}
	p.RepoDesc = scaffold.OneLine(p.RepoDesc)
	if p.Homepage, err = a.askString(cmd, "homepage", "Homepage URL (optional)", cmd.String("homepage"), nil); err != nil {
		return err
	}
	topics, err := a.askString(cmd, "topics", "Topics (comma-separated, optional)", strings.Join(cmd.StringSlice("topics"), ","), nil)
	if err != nil {
		return err
	}
	p.Topics = splitList(topics)
	p.DisableWiki, err = a.askBool(cmd, "disable-wiki", "Disable the wiki?")
	return err
}

// marketplaceAdded reports (added, known); known=false when claude is missing
// or its output could not be read (preflight / the install itself will say).
func (a *app) marketplaceAdded(ctx context.Context) (bool, bool) {
	if _, err := a.env.Runner.LookPath("claude"); err != nil {
		return false, false
	}
	out, code, err := a.output(ctx, "claude", "plugin", "marketplace", "list", "--json")
	if err != nil || code != 0 {
		return false, false
	}
	added, perr := doctor.HasMarketplace(out, kit.Marketplace)
	return added, perr == nil
}

func (a *app) probe() doctor.Probe {
	return doctor.OSProbe{LookPathFn: a.env.Runner.LookPath, OutputFn: a.output}
}

// preflightNew: git always; gh+auth only with a GitHub repo; claude only
// with the plugin install. Fails before anything is created.
func (a *app) preflightNew(ctx context.Context, p *newPlan) error {
	forList := []string{"new"}
	if p.GitHub {
		forList = append(forList, "github")
	}
	if p.Plugin {
		forList = append(forList, "source")
	}
	c, _, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	r, err := doctor.Run(ctx, a.probe(), doctor.Options{
		GOOS: a.env.GOOS, CodePath: c.CodePath,
		SmartgitPath: launch.SmartGitProgram(a.env.GOOS, c.SmartgitPath), For: forList,
	})
	if err != nil {
		return err
	}
	if !r.OK {
		var b strings.Builder
		b.WriteString("preflight failed, nothing was created:")
		for _, ch := range r.Checks {
			if ch.Required && ch.Status != doctor.OK && ch.Status != doctor.Info {
				fmt.Fprintf(&b, "\n  %s: %s %s", ch.Name, ch.Status, ch.Detail)
				if ch.Fix != "" {
					fmt.Fprintf(&b, "  (fix: %s)", ch.Fix)
				}
			}
		}
		return cli.Exit(b.String(), 1)
	}
	if p.Commit {
		if out, code, err := a.output(ctx, "git", "config", "user.email"); err != nil || code != 0 || strings.TrimSpace(out) == "" {
			fmt.Fprintln(a.env.Stderr, "ckit: warning: git user.email is not set here; the first commit may fail (git config --global user.email you@example.com)")
		}
	}
	return nil
}
