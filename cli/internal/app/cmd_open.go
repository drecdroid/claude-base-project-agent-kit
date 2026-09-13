package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

func (a *app) openCommand() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "open a project in VS Code, SmartGit or Claude Desktop (Claude Code tab)",
		ArgsUsage: "<code|smartgit|claude> [project]",
		UsageText: "ckit open code                    # cwd in VS Code\n" +
			"ckit open smartgit tdm-app\n" +
			"ckit open claude ./my-app --prompt \"review the diff\"\n\n" +
			"claude opens claude://code/new?folder=<path>[&q=<prompt>] via the OS URL handler;\n" +
			"Claude Desktop ALWAYS shows a folder-trust confirmation before using the folder.\n" +
			"[project]: empty = cwd; a name = <projectsDir>/<name>; or a path",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "prompt", Aliases: []string{"p"}, Usage: "claude only: text to prefill in the prompt"},
		},
		Action: a.open,
	}
}

func (a *app) open(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() > 2 {
		return cli.Exit("usage: ckit open <code|smartgit|claude> [project] (quote paths with spaces)", 2)
	}
	appName := cmd.Args().Get(0)
	if appName == "" {
		if !a.interactive(cmd) {
			return cli.Exit("which app? ckit open <"+strings.Join(launch.Apps, "|")+"> [project]", 2)
		}
		pick, err := a.env.Prompter.Select("open in", launch.Apps)
		if err != nil {
			return err
		}
		appName = pick
	}
	if err := launch.ValidateApp(appName); err != nil {
		return cli.Exit(err.Error(), 2)
	}
	prompt := cmd.String("prompt")
	if prompt != "" && appName != "claude" {
		return cli.Exit("--prompt only applies to `ckit open claude`", 2)
	}
	dir, err := a.resolveProject(cmd, cmd.Args().Get(1))
	if err != nil {
		return err
	}
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
	}
	if err := a.exec(ctx, cmd, x, true); err != nil {
		return err
	}
	if !cmd.Bool("dry-run") && appName == "claude" {
		fmt.Fprintln(a.env.Stdout, "opened Claude Desktop; confirm the folder-trust prompt there")
	}
	return nil
}

func (a *app) checkSmartGit(override string) error {
	prog := launch.SmartGitProgram(a.env.GOOS, override)
	if strings.ContainsAny(prog, `/\`) {
		if _, err := os.Stat(prog); err != nil {
			return cli.Exit(fmt.Sprintf("SmartGit not found at %s; install it or: ckit config set smartgitPath <path>", prog), 1)
		}
		return nil
	}
	if _, err := a.env.Runner.LookPath(prog); err != nil {
		return cli.Exit(fmt.Sprintf("%s not on PATH; install SmartGit or: ckit config set smartgitPath <path>", prog), 1)
	}
	return nil
}
