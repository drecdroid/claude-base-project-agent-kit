package app

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/kit"
)

// SourceArgs is the `claude` argv for `ckit source <action>`.
func SourceArgs(action string) ([]string, error) {
	switch action {
	case "add":
		return []string{"plugin", "marketplace", "add", kit.Repo}, nil
	case "remove":
		return []string{"plugin", "marketplace", "remove", kit.Marketplace}, nil
	case "update":
		return []string{"plugin", "marketplace", "update", kit.Marketplace}, nil
	}
	return nil, fmt.Errorf("unknown source action %q", action)
}

// PluginArgs is the `claude` argv for `ckit plugin <action>`. -y on install
// and uninstall makes them non-interactive (required without a TTY); update
// only gets it with --yes.
func PluginArgs(action, scope string, yes bool) ([]string, error) {
	switch scope {
	case "user", "project", "local":
	default:
		return nil, fmt.Errorf("invalid --scope %q; use user, project or local", scope)
	}
	ref := kit.PluginRef()
	switch action {
	case "install":
		return []string{"plugin", "install", ref, "--scope", scope, "-y"}, nil
	case "uninstall":
		return []string{"plugin", "uninstall", ref, "--scope", scope, "-y"}, nil
	case "update":
		args := []string{"plugin", "update", ref, "--scope", scope}
		if yes {
			args = append(args, "-y")
		}
		return args, nil
	}
	return nil, fmt.Errorf("unknown plugin action %q", action)
}

func (a *app) sourceCommand() *cli.Command {
	sub := func(action, usage string) *cli.Command {
		return &cli.Command{
			Name:  action,
			Usage: usage,
			Action: func(ctx context.Context, cmd *cli.Command) error {
				args, err := SourceArgs(action)
				if err != nil {
					return err
				}
				if action == "remove" {
					if err := a.confirm(cmd, "remove marketplace "+kit.Marketplace+" (and its plugins)?"); err != nil {
						return err
					}
				}
				return a.exec(ctx, cmd, execx.Cmd{Prog: "claude", Args: args}, false)
			},
		}
	}
	return &cli.Command{
		Name:  "source",
		Usage: "add/remove/update the kit's Claude Code plugin marketplace (user-level)",
		UsageText: "ckit source add      # claude plugin marketplace add " + kit.Repo + "\n" +
			"ckit source update   # claude plugin marketplace update " + kit.Marketplace + "\n" +
			"ckit source remove --yes",
		Commands: []*cli.Command{
			sub("add", "claude plugin marketplace add "+kit.Repo),
			sub("remove", "claude plugin marketplace remove "+kit.Marketplace+" (confirms; --yes without a TTY)"),
			sub("update", "claude plugin marketplace update "+kit.Marketplace),
		},
	}
}

func (a *app) pluginCommand() *cli.Command {
	sub := func(action, usage string) *cli.Command {
		return &cli.Command{
			Name:      action,
			Usage:     usage,
			ArgsUsage: "[project]",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "scope", Aliases: []string{"s"}, Value: "project", Usage: "user, project or local"},
			},
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.NArg() > 1 {
					return cli.Exit("expected at most one [project] argument", 2)
				}
				args, err := PluginArgs(action, cmd.String("scope"), cmd.Bool("yes"))
				if err != nil {
					return cli.Exit(err.Error(), 2)
				}
				dir, err := a.resolveProject(cmd, cmd.Args().First())
				if err != nil {
					return err
				}
				if action == "uninstall" {
					if err := a.confirm(cmd, "uninstall "+kit.PluginRef()+" ("+cmd.String("scope")+" scope) in "+dir+"?"); err != nil {
						return err
					}
				}
				return a.exec(ctx, cmd, execx.Cmd{Dir: dir, Prog: "claude", Args: args}, false)
			},
		}
	}
	return &cli.Command{
		Name:  "plugin",
		Usage: "install/uninstall/update " + kit.PluginRef() + " in a project",
		UsageText: "ckit plugin install              # in cwd, --scope project\n" +
			"ckit plugin install tdm-app      # <projectsDir>/tdm-app\n" +
			"ckit plugin update ./my-app --scope local\n" +
			"ckit plugin uninstall --yes\n\n" +
			"[project]: empty = cwd; a name = <projectsDir>/<name>; or a path",
		Commands: []*cli.Command{
			sub("install", "claude plugin install "+kit.PluginRef()+" --scope <scope> -y"),
			sub("uninstall", "claude plugin uninstall "+kit.PluginRef()+" --scope <scope> -y (confirms; --yes without a TTY)"),
			sub("update", "claude plugin update "+kit.PluginRef()+" --scope <scope> (restart claude to apply)"),
		},
	}
}
