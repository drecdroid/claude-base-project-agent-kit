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
	Select(title string, options []string) (string, error)
	Confirm(title string) (bool, error)
	// EditConfig edits c in place (projectsDir, tool path overrides).
	EditConfig(c *config.Config, home, goos string) error
}

type huhPrompter struct{}

func (huhPrompter) Select(title string, options []string) (string, error) {
	var v string
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(title).Options(huh.NewOptions(options...)...).Value(&v),
	)).Run()
	return v, err
}

func (huhPrompter) Confirm(title string) (bool, error) {
	var v bool
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Affirmative("Yes").Negative("No").Value(&v),
	)).Run()
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
