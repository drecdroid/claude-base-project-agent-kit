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
```

LF everywhere (`.gitattributes`, `.editorconfig`).
