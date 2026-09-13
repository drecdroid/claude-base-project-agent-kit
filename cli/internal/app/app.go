// Package app wires ckit's commands (urfave/cli v3) onto the pure packages.
// Everything that touches the outside world comes in through Env, so the
// whole command surface is testable with a fake runner, a temp home and no TTY.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/urfave/cli/v3"
	"golang.org/x/term"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/project"
)

// Env is ckit's view of the outside world.
type Env struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Runner      execx.Runner
	GOOS        string
	Getwd       func() (string, error)
	Home        func() (string, error)
	Interactive func() bool // stdin AND stdout are terminals
	Prompter    Prompter
	HTTPClient  *http.Client // nil = default client with a timeout
}

// DefaultEnv is the production Env.
func DefaultEnv() Env {
	return Env{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Runner: execx.Real{},
		GOOS:   runtime.GOOS,
		Getwd:  os.Getwd,
		Home:   config.Home,
		Interactive: func() bool {
			return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		},
		Prompter: huhPrompter{},
	}
}

type app struct{ env Env }

// Version is the module version when installed via `go install ...@vX`, else "dev".
func Version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// NewCommand builds the root command.
func NewCommand(env Env) *cli.Command {
	a := &app{env: env}
	return &cli.Command{
		Name:    "ckit",
		Usage:   "automate the Claude agent kit: new project, config, doctor, marketplace source, plugin, open project in apps",
		Version: Version(),
		Reader:  env.Stdin,
		Writer:  env.Stdout,
		// ErrWriter/ExitErrHandler: errors are printed once, by Main.
		ErrWriter:       io.Discard,
		ExitErrHandler:  func(context.Context, *cli.Command, error) {},
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "print the exact argv (or file write) instead of running it"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "never prompt; accept confirmations (non-TTY/scripts)"},
		},
		Commands: []*cli.Command{
			a.newCommand(),
			a.configCommand(),
			a.doctorCommand(),
			a.sourceCommand(),
			a.pluginCommand(),
			a.openCommand(),
		},
	}
}

// Main runs ckit and returns the process exit code.
func Main(ctx context.Context, env Env, args []string) int {
	err := NewCommand(env).Run(ctx, args)
	if err == nil {
		return 0
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		if msg := ec.Error(); msg != "" {
			fmt.Fprintln(env.Stderr, "ckit:", msg)
		}
		return ec.ExitCode()
	}
	fmt.Fprintln(env.Stderr, "ckit:", err)
	return 1
}

// interactive: a TTY and no --yes.
func (a *app) interactive(cmd *cli.Command) bool {
	return !cmd.Bool("yes") && a.env.Interactive != nil && a.env.Interactive()
}

func (a *app) loadConfig() (config.Config, string, string, error) {
	home, err := a.env.Home()
	if err != nil {
		return config.Config{}, "", "", err
	}
	p := config.Path(home)
	c, err := config.Load(p)
	return c, p, home, err
}

// resolveProject resolves [project]; on a miss in a TTY it offers the
// suggestions (or every project) in a picker.
func (a *app) resolveProject(cmd *cli.Command, arg string) (string, error) {
	c, _, home, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	cwd, err := a.env.Getwd()
	if err != nil {
		return "", err
	}
	pd := c.ResolvedProjectsDir(home)
	dir, err := project.Resolve(arg, pd, cwd, home)
	var nf *project.NotFoundError
	if err == nil || !errors.As(err, &nf) || !a.interactive(cmd) {
		return dir, err
	}
	opts := nf.Suggestions
	if len(opts) == 0 {
		opts = nf.All
	}
	if len(opts) == 0 {
		return "", err
	}
	pick, perr := a.env.Prompter.Select(fmt.Sprintf("project %q not found in %s; pick one", arg, pd), opts)
	if perr != nil {
		return "", err
	}
	return project.Resolve(pick, pd, cwd, home)
}

// exec runs (or with --dry-run prints) one external command. detach=true for
// GUI launchers: start and do not wait.
func (a *app) exec(ctx context.Context, cmd *cli.Command, c execx.Cmd, detach bool) error {
	if cmd.Bool("dry-run") {
		a.printDryRun(c)
		return nil
	}
	if detach {
		return a.env.Runner.Start(ctx, c)
	}
	code, err := a.env.Runner.Run(ctx, c)
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.Exit(fmt.Sprintf("%s exited with code %d", c.Prog, code), code)
	}
	return nil
}

func (a *app) printDryRun(c execx.Cmd) {
	w := a.env.Stdout
	if c.Dir != "" {
		fmt.Fprintf(w, "[dry-run] in %s\n", c.Dir)
	}
	fmt.Fprintf(w, "[dry-run] %s\n", execx.Format(c.Argv()))
	if a.env.GOOS == "windows" && a.env.Runner != nil {
		if p, err := a.env.Runner.LookPath(c.Prog); err == nil && execx.IsBatchPath(p) {
			fmt.Fprintf(w, "[dry-run] %s is a batch shim: run via cmd.exe /d /s /c with per-argument quoting\n", p)
		}
	}
}

// confirm asks in a TTY; without one, --yes is required.
func (a *app) confirm(cmd *cli.Command, question string) error {
	if cmd.Bool("yes") || cmd.Bool("dry-run") {
		return nil
	}
	if !a.interactive(cmd) {
		return cli.Exit(question+" (not a terminal: pass --yes to confirm)", 2)
	}
	ok, err := a.env.Prompter.Confirm(question, false)
	if err != nil {
		return err
	}
	if !ok {
		return cli.Exit("aborted", 1)
	}
	return nil
}
