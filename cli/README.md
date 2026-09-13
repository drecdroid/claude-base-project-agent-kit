# ckit

A small Go CLI that automates using the agent kit: config, a tool doctor, the Claude Code plugin
marketplace ("source"), installing the `agent-kit` plugin into a project, and opening a project in
VS Code, SmartGit or Claude Desktop. (`ckit new`, scaffolding a project from `template/`, comes next.)

## Install

Needs Go 1.27+.

```sh
go install github.com/drecdroid/claude-base-project-agent-kit/cli/cmd/ckit@latest
```

The binary is `ckit`, in `$(go env GOPATH)/bin` (add it to PATH). While the repo is private, Go
has to be allowed to fetch it: `GOPRIVATE=github.com/drecdroid/*` and git credentials for GitHub
(`gh auth setup-git`). From a clone: `cd cli && go install ./cmd/ckit`.

## Global flags

- `--dry-run`, `-n`: print the exact argv (or the config write) instead of doing it. Works on every
  command that launches or changes anything.
- `--yes`, `-y`: never prompt. Confirmations count as accepted, and interactive pickers are
  skipped (you get an error listing the choices instead). Without a terminal, ckit never prompts
  either; `source remove` and `plugin uninstall` then need `--yes`.

## `[project]` argument

- empty: the current directory
- a name: `<projectsDir>/<name>` (then `./<name>`)
- a path: `./app`, `../x`, `~/code/x`, `C:\work\x`

If a name isn't found, ckit suggests close matches (exact, prefix, substring, then fuzzy) and lists
every project. In a terminal it also offers the matches in a picker.

## Commands

### `ckit config`

```sh
ckit config                              # current values, where each comes from, and the file path
ckit config --json
ckit config get projectsDir
ckit config set projectsDir ~/code       # "" unsets a key
ckit config set smartgitPath "D:\Apps\SmartGit\bin\smartgit.exe"
ckit config edit                         # interactive form (needs a terminal)
ckit config path
```

Keys: `projectsDir` (default `~/Projects`), `smartgitPath` (default per OS, see `open`),
`codePath` (default `code` on PATH).

**Config file:** `~/.ckit/config.json`, found through the home directory (`USERPROFILE` on Windows,
`HOME` elsewhere). It deliberately does not go under `%APPDATA%`: on Windows, a terminal running
with an MSIX package identity (the Claude desktop app's terminal, for example) has `%APPDATA%`
writes redirected to a private copy, which would split your config in two.

### `ckit doctor`

```sh
ckit doctor                              # required = what `source` + `plugin` need
ckit doctor --for all
ckit doctor --for open-code,open-smartgit --json
```

Reports ok/missing, the version, the path and a fix hint for: git (and whether
`init.defaultBranch` is set, just for information), gh and `gh auth status`, `claude --version`,
whether the kit marketplace has been added, VS Code `code`, and SmartGit. Every check is always
shown; `*` marks the ones `--for` requires. The exit code is 1 only when a required check fails.
`--for` accepts: `source`, `plugin`, `open-code`, `open-smartgit`, `open-claude`, `new`, `all`.

### `ckit source add|remove|update`

```sh
ckit source add        # claude plugin marketplace add drecdroid/claude-base-project-agent-kit
ckit source update     # claude plugin marketplace update claude-base-project-agent-kit
ckit source remove -y  # claude plugin marketplace remove claude-base-project-agent-kit
```

### `ckit plugin install|uninstall|update [project]`

These run inside the project directory. `--scope user|project|local` (default `project`).

```sh
ckit plugin install                  # claude plugin install agent-kit@claude-base-project-agent-kit --scope project -y
ckit plugin install tdm-app --scope local
ckit plugin update ./my-app          # claude plugin update agent-kit@... --scope project   (restart claude to apply)
ckit plugin uninstall --yes          # claude plugin uninstall agent-kit@... --scope project -y
```

### `ckit open code|smartgit|claude [project]`

```sh
ckit open code                       # code <cwd>
ckit open smartgit tdm-app           # <smartgit> <projectsDir>/tdm-app
ckit open claude ./my-app --prompt "review the diff" --dry-run
```

- `code`: runs `code <dir>`, or `codePath` if set.
- `smartgit`: Windows `C:\Program Files\SmartGit\bin\smartgit.exe <dir>`; macOS
  `open -a /Applications/SmartGit.app <dir>`; Linux `smartgit <dir>` from PATH. `smartgitPath`
  overrides all three.
- `claude`: opens the Claude Desktop deep link
  `claude://code/new?folder=<url-encoded absolute path>[&q=<url-encoded prompt>]` with the OS URL
  handler: Windows `rundll32 url.dll,FileProtocolHandler <url>` (not `cmd /c start`, because
  cmd's parser mangles `&` and `%`), macOS `open <url>`, Linux `xdg-open <url>`. **Claude Desktop
  always asks you to confirm trusting the folder before it uses it.** See
  https://support.claude.com/en/articles/14729294-open-claude-desktop-with-a-link

## Windows `.cmd` shims

`code`, and `claude` when installed through npm, are `.cmd` batch shims. Running one with a plain
exec sends its arguments through cmd.exe's parser: `a&b` runs `b`, `^` gets dropped, `%VAR%` gets
expanded. So ckit runs every external command through one runner (`internal/execx`). It resolves
the program with PATHEXT, and for a batch file it builds the `cmd.exe /d /s /c "..."` line itself,
quoting each argument and splitting around `%`. That code is copied, along with its tests, from
tdm-app's `tools/gw`. A bare program name is never resolved to a binary in the current directory.

## Develop

```sh
cd cli
go test ./...
go vet ./... && gofmt -l .
GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/ckit
```

Layout: `cmd/ckit` (main), `internal/app` (urfave/cli v3 commands, huh prompts; everything external
comes in through `app.Env`), `internal/{config,project,launch,doctor,execx,kit}` (pure or narrow
packages with their own tests). Adding `ckit new`: a new `internal/app/cmd_new.go` added to
`NewCommand`'s `Commands`, using `resolveProject`/`exec`/`confirm`, `kit.*`, and `doctor`'s
existing `new` requirement set.
