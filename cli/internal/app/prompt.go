package app

import (
	"path/filepath"

	"github.com/charmbracelet/huh"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/config"
	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/launch"
)

// Prompter is every interactive prompt ckit shows. Each has a flag/non-TTY
// equivalent; callers only reach a Prompter when Env.Interactive is true.
type Prompter interface {
	// Select picks one option; the first option is the default.
	Select(title string, options []string) (string, error)
	Confirm(title string, def bool) (bool, error)
	// Input asks for one line, prefilled with def; validate may be nil.
	Input(title, description, def string, validate func(string) error) (string, error)
	// EditConfig edits c in place (projectsDir, tool path overrides).
	EditConfig(c *config.Config, home, goos string) error
}

type huhPrompter struct{}

func (huhPrompter) Select(title string, options []string) (string, error) {
	var v string
	if len(options) > 0 {
		v = options[0]
	}
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(huh.NewOptions(options...)...).Value(&v),
	)).Run()
	return v, err
}

func (huhPrompter) Confirm(title string, def bool) (bool, error) {
	v := def
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Affirmative("Yes").Negative("No").Value(&v),
	)).Run()
	return v, err
}

func (huhPrompter) Input(title, description, def string, validate func(string) error) (string, error) {
	v := def
	in := huh.NewInput().Title(title).Value(&v)
	if description != "" {
		in = in.Description(description)
	}
	if validate != nil {
		in = in.Validate(validate)
	}
	err := huh.NewForm(huh.NewGroup(in)).Run()
	return v, err
}

func (huhPrompter) EditConfig(c *config.Config, home, goos string) error {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("projectsDir").
			Description("where your projects live; ~ allowed; empty = default").
			Placeholder(filepath.Join(home, "Projects")).Value(&c.ProjectsDir),
		huh.NewInput().Title("smartgitPath").
			Description("SmartGit override; empty = default").
			Placeholder(launch.SmartGitDefault(goos)).Value(&c.SmartgitPath),
		huh.NewInput().Title("codePath").
			Description("VS Code `code` CLI override; empty = `code` on PATH").
			Placeholder("code").Value(&c.CodePath),
	).Title("ckit config (" + config.Path(home) + ")")).Run()
}
