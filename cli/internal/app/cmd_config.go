package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

func (a *app) configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "show config and its file location; get/set/edit keys",
		UsageText: "ckit config                      # current values + file path\n" +
			"ckit config get projectsDir\n" +
			"ckit config set projectsDir ~/code   # empty value unsets\n" +
			"ckit config edit                 # interactive form (TTY)\n\n" +
			"keys: " + strings.Join(config.KeyNames(), ", "),
		Flags:  []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "machine-readable output"}},
		Action: a.configShow,
		Commands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "print a key's effective value (stored or default)",
				ArgsUsage: "<key>",
				Action:    a.configGet,
			},
			{
				Name:      "set",
				Usage:     "store a key (empty value unsets it)",
				ArgsUsage: "<key> <value>",
				Action:    a.configSet,
			},
			{
				Name:   "edit",
				Usage:  "edit every key in an interactive form (needs a terminal; else use set)",
				Action: a.configEdit,
			},
			{
				Name:  "path",
				Usage: "print the config file path",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					_, p, _, err := a.loadConfig()
					if err != nil {
						return err
					}
					fmt.Fprintln(a.env.Stdout, p)
					return nil
				},
			},
		},
	}
}

type configRow struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Default   string `json:"default"`
	Effective string `json:"effective"`
}

func (a *app) configRows(c config.Config, home string) []configRow {
	zero := config.Config{}
	defaults := map[string]string{
		"projectsDir":  zero.ResolvedProjectsDir(home),
		"smartgitPath": launch.SmartGitDefault(a.env.GOOS),
		"codePath":     "code",
	}
	var rows []configRow
	for _, k := range config.KeyNames() {
		v, _ := c.Get(k)
		eff := v
		if k == "projectsDir" {
			eff = c.ResolvedProjectsDir(home)
		} else if eff == "" {
			eff = defaults[k]
		}
		rows = append(rows, configRow{Key: k, Value: v, Default: defaults[k], Effective: eff})
	}
	return rows
}

func (a *app) configShow(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() > 0 {
		return cli.Exit(fmt.Sprintf("unknown config subcommand %q; use get, set, edit or path", cmd.Args().First()), 2)
	}
	c, p, home, err := a.loadConfig()
	if err != nil {
		return err
	}
	rows := a.configRows(c, home)
	if cmd.Bool("json") {
		b, _ := json.MarshalIndent(map[string]any{"path": p, "keys": rows}, "", "  ")
		fmt.Fprintln(a.env.Stdout, string(b))
		return nil
	}
	fmt.Fprintf(a.env.Stdout, "config file: %s\n", p)
	for _, r := range rows {
		src := "set"
		if r.Value == "" {
			src = "default"
		}
		fmt.Fprintf(a.env.Stdout, "  %-13s %s  (%s)\n", r.Key, r.Effective, src)
	}
	return nil
}

func (a *app) configGet(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return cli.Exit("usage: ckit config get <key>; keys: "+strings.Join(config.KeyNames(), ", "), 2)
	}
	c, _, home, err := a.loadConfig()
	if err != nil {
		return err
	}
	if _, err := c.Get(cmd.Args().First()); err != nil {
		return cli.Exit(err.Error(), 2)
	}
	for _, r := range a.configRows(c, home) {
		if strings.EqualFold(r.Key, cmd.Args().First()) {
			fmt.Fprintln(a.env.Stdout, r.Effective)
		}
	}
	return nil
}

func (a *app) configSet(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 2 {
		return cli.Exit("usage: ckit config set <key> <value> (use \"\" to unset); keys: "+strings.Join(config.KeyNames(), ", "), 2)
	}
	c, p, _, err := a.loadConfig()
	if err != nil {
		return err
	}
	key, val := cmd.Args().Get(0), cmd.Args().Get(1)
	if err := c.Set(key, val); err != nil {
		return cli.Exit(err.Error(), 2)
	}
	if cmd.Bool("dry-run") {
		fmt.Fprintf(a.env.Stdout, "[dry-run] would write %s = %q to %s\n", key, strings.TrimSpace(val), p)
		return nil
	}
	if err := config.Save(p, c); err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "%s = %q (%s)\n", key, strings.TrimSpace(val), p)
	return nil
}

func (a *app) configEdit(ctx context.Context, cmd *cli.Command) error {
	if !a.interactive(cmd) {
		return cli.Exit("ckit config edit needs a terminal; use: ckit config set <key> <value>", 2)
	}
	c, p, home, err := a.loadConfig()
	if err != nil {
		return err
	}
	if err := a.env.Prompter.EditConfig(&c, home, a.env.GOOS); err != nil {
		return err
	}
	for _, k := range config.KeyNames() { // normalise (trim) through Set
		v, _ := c.Get(k)
		_ = c.Set(k, v)
	}
	if cmd.Bool("dry-run") {
		fmt.Fprintf(a.env.Stdout, "[dry-run] would write %s: %+v\n", p, c)
		return nil
	}
	if err := config.Save(p, c); err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "saved %s\n", p)
	return nil
}
