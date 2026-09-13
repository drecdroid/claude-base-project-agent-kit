# claude-base-project-agent-kit

Claude Code plugin marketplace with one plugin, `agent-kit`, plus a project starter `template/`.
Be extremely concise; sacrifice grammar.

## Layout

- `.claude-plugin/marketplace.json` — marketplace `claude-base-project-agent-kit`, lists `./plugins/agent-kit`.
- `plugins/agent-kit/.claude-plugin/plugin.json` — manifest (bump `version` here AND in marketplace.json; users only get updates when it changes).
- `plugins/agent-kit/hooks/hooks.json` — SessionStart (`startup|resume|clear|compact`), shell form `cat "${CLAUDE_PLUGIN_ROOT}/rules.md"`; plain-text stdout → context. No runtime: Git Bash/sh, or PowerShell fallback (`cat` = Get-Content; verified live).
- `plugins/agent-kit/rules/*.md` — editing source. `NN-` prefix = order. ASCII only (PowerShell 5 reads BOM-less files as ANSI → mangled).
- `plugins/agent-kit/rules.md` — GENERATED: header + `rules/*.md` sorted. `sh scripts/build-rules.sh` after every rules edit; never hand-edit.
- `scripts/check.sh` — node-free checks (rules.md in sync + ASCII, JSON parses, versions match, LF); CI `.github/workflows/check.yml` runs it.
- `plugins/agent-kit/agents/{orchestrator,worker}.md` — subagents.
- `template/` — copied into new projects (LF config, short CLAUDE.md skeleton, settings enabling the plugin).
- `cli/` — `ckit` Go CLI, own module `github.com/drecdroid/claude-base-project-agent-kit/cli`, main at `cli/cmd/ckit`. Usage: `cli/README.md`.

## ckit (`cli/`)

- urfave/cli v3 + charmbracelet/huh. Every prompt has a flag / non-TTY path (`--yes`); every launch/mutation has `--dry-run`.
- ALL external commands go through `internal/execx` (Windows `.cmd` shims → hand-built `cmd.exe /d /s /c` line; copied from tdm-app `tools/gw`, keep in step). Never `exec.Command` directly.
- Config `~/.ckit/config.json` via `os.UserHomeDir`; never `os.UserConfigDir` (%APPDATA% MSIX redirection).
- Claude Desktop: `claude://code/new?folder=..&q=..`; Windows launch via `rundll32 url.dll,FileProtocolHandler`, never `cmd /c start`.
- Tests: fake `execx.Runner` + temp home (`app.Env`); never touch real `~/.claude` / `~/.ckit`. Real plugin runs only with `CLAUDE_CONFIG_DIR=<scratch>`.
- Kit identifiers (repo, marketplace, plugin) live in `internal/kit` — update there if renamed.
- Part 2 (`ckit new`) = new `internal/app/cmd_new.go`; doctor already has a `new` requirement set.

## Rules for editing rules

- Injected into EVERY session's context: keep total small (currently ~40 lines / ~5.7 KB; stay under ~150 lines). Concise imperative bullets, one-clause "why". Cut words, not information.
- Generic only: no project names, paths, packages, ticket numbers.
- Plugins cannot ship CLAUDE.md; project specifics belong in the project's own CLAUDE.md.

## Test

```sh
sh scripts/build-rules.sh && wc -l -c plugins/agent-kit/rules.md   # rebuild + size
sh scripts/check.sh
claude plugin validate --strict ./plugins/agent-kit
claude plugin validate --strict .
claude --plugin-dir ./plugins/agent-kit   # try it for one session, no install
git ls-files --eol | grep -v "i/lf" # must print nothing (binary excepted)
cd cli && go test ./... && go vet ./... && gofmt -l .   # check.sh runs these when go is on PATH
```

LF everywhere (`.gitattributes`, `.editorconfig`).
