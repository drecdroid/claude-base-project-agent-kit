package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/doctor"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

func (a *app) doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "check git, gh (+auth), claude, the kit marketplace, VS Code, SmartGit",
		UsageText: "ckit doctor                   # required = what `source` and `plugin` need\n" +
			"ckit doctor --for all --json\n" +
			"ckit doctor --for open-code,open-smartgit\n\n" +
			"--for values: " + strings.Join(doctor.ForNames(), ", ") + "\n" +
			"exit code 1 only if a check REQUIRED by --for is not ok; everything is always reported",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "machine-readable output"},
			&cli.StringSliceFlag{Name: "for", Usage: "command sets whose requirements decide the exit code (default: source,plugin)"},
		},
		Action: a.doctor,
	}
}

func (a *app) doctor(ctx context.Context, cmd *cli.Command) error {
	c, _, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	var forList []string
	for _, f := range cmd.StringSlice("for") {
		forList = append(forList, strings.Split(f, ",")...)
	}
	probe := doctor.OSProbe{
		LookPathFn: a.env.Runner.LookPath,
		OutputFn: func(ctx context.Context, prog string, args ...string) (string, int, error) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return a.env.Runner.Output(ctx, execx.Cmd{Prog: prog, Args: args})
		},
	}
	r, err := doctor.Run(ctx, probe, doctor.Options{
		GOOS:         a.env.GOOS,
		CodePath:     c.CodePath,
		SmartgitPath: launch.SmartGitProgram(a.env.GOOS, c.SmartgitPath),
		For:          forList,
	})
	if err != nil {
		return cli.Exit(err.Error(), 2)
	}
	if cmd.Bool("json") {
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Fprintln(a.env.Stdout, string(b))
	} else {
		doctor.WriteText(a.env.Stdout, r)
	}
	if !r.OK {
		return cli.Exit("", 1)
	}
	return nil
}
