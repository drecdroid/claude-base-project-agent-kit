# ckit

A small Go CLI that automates using the agent kit. It can create a new project from the kit
template, manage its config, check your tools (`doctor`), add the Claude Code plugin marketplace
("source"), install the `agent-kit` plugin into a project, and open a project in VS Code, SmartGit
or Claude Desktop.

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

### `ckit new [name]`

Creates a project from the kit's `template/`. In a terminal it asks each question; every question
also has a flag. `--yes` (or no terminal) uses the defaults, and `--dry-run` prints every step
without doing anything.

```sh
ckit new                                          # interactive
ckit new my-app -d "what it is" --yes             # defaults: no GitHub repo, commit, plugin, open none
ckit new my-app --github --visibility public --topics cli,go --homepage https://x.dev --yes
ckit new my-app --github --open claude --dry-run  # print the whole plan
ckit new my-app --template-source ../claude-base-project-agent-kit --yes   # local kit checkout
ckit new my-app --ref v0.2.0 --yes                # a tag, branch or sha of the kit
```

The steps, in order. Nothing is created until all the answers are in and preflight passes:

1. **Name**: letters, digits, `.`, `_` or `-`; not a Windows reserved name. The target is
   `<projectsDir>/<name>` (or `--dir`) and must not already exist with files in it.
2. **Preflight**: git is always required. gh and `gh auth status` are required only with
   `--github`, and claude only when installing the plugin.
3. **Template**: fetched first, so a failure leaves nothing behind. By default it is
   `https://codeload.github.com/drecdroid/claude-base-project-agent-kit/tar.gz/<ref>` (ref `main`),
   downloaded over HTTPS with no git, curl or login. Only `*/template/**` is extracted. Dotfiles
   (`.claude/settings.json`, `.gitattributes`, `.editorconfig`) are kept byte for byte, so LF stays
   LF. Entries with `..`, absolute paths, drive letters, backslashes, or links are rejected.
   `--template-source` takes a local kit checkout, a template directory, or another `.tar.gz` URL
   (forks, tests). If the repo is private or missing, the error says so and suggests
   `--template-source`.
4. `git init -b main` (never `master`), then the template is written and placeholders are filled.
5. **GitHub** (`--github`; off by default): runs
   `gh repo create <owner>/<repo> --private|--public|--internal [--description] [--homepage] [--disable-wiki] --source <dir> --remote origin`.
   The repo is not pushed at this point. Topics are then added with `gh repo edit --add-topic`. The
   owner defaults to your gh user (`--owner` can be an org) and the repo name to the project name.
   The wiki is disabled by default. If gh isn't installed or logged in, ckit prints
   `gh auth login` and continues without creating the repo.
6. **Plugin** (`--plugin`, default yes): if the kit marketplace is missing, it is added first
   (`--add-source`), then `claude plugin install agent-kit@claude-base-project-agent-kit --scope project -y`
   runs in the new folder. This happens before the commit because the install rewrites
   `.claude/settings.json`; otherwise the new repo would be dirty right after its first commit.
7. **First commit** (`--commit`, default yes): `git add -A && git commit -m "chore: init from agent-kit template"`,
   using your own git identity (ckit never sets `user.email`). If a repo was created, it then runs
   `git push -u origin main` (`--push`, default yes).
8. **Open** (`--open none|code|smartgit|claude`, default none). For `claude`, `--prompt` defaults
   to "Fill in CLAUDE.md for this project".

If a step fails, ckit stops and lists which steps were done and which weren't. It never deletes
the folder; instead it prints the exact command to remove it (plus `gh repo delete` if a repo was
already created).

**Template placeholders.** Files in `template/` may contain `{{project_name}}` and
`{{description}}` (one line). ckit replaces them with plain string replacement, not a template
engine, so user text containing `{{`, `$` or `%` is inserted as-is. Binary files and unknown
`{{...}}` are left alone.

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
`--for` accepts: `source`, `plugin`, `open-code`, `open-smartgit`, `open-claude`, `new` (git),
`github` (gh + auth), `all`.

### `ckit source add|remove|update`

```sh
ckit source add        # claude plugin marketplace add https://github.com/drecdroid/claude-base-project-agent-kit.git
ckit source update     # claude plugin marketplace update claude-base-project-agent-kit
ckit source remove -y  # claude plugin marketplace remove claude-base-project-agent-kit
```

`add` uses the HTTPS git URL, not the `owner/repo` shorthand. Claude clones the shorthand over SSH,
and that fails on machines where github.com isn't in `known_hosts`. Both forms give the marketplace
the same name (`claude-base-project-agent-kit`), because the name comes from `marketplace.json`.

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
comes in through `app.Env`; `new` is split into `cmd_new.go`, `new_gather.go` and `new_run.go`),
and `internal/{config,project,launch,doctor,execx,kit,scaffold}` (pure or narrow packages with
their own tests; `scaffold` handles template fetch, extraction, placeholders and name
validation).
