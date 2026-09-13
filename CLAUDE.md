# claude-base-project-agent-kit

Claude Code plugin marketplace with one plugin, `agent-kit`, plus a project starter `template/`.
Be extremely concise; sacrifice grammar.

## Layout

- `.claude-plugin/marketplace.json` — marketplace `claude-base-project-agent-kit`, lists `./plugins/agent-kit`.
- `plugins/agent-kit/.claude-plugin/plugin.json` — manifest (bump `version` here AND in marketplace.json; users only get updates when it changes).
- `plugins/agent-kit/hooks/hooks.json` — SessionStart (`startup|resume|clear|compact`), exec form `node ${CLAUDE_PLUGIN_ROOT}/hooks/inject-rules.mjs` (no shell → same on Windows/macOS/Linux; needs `node` on PATH).
- `plugins/agent-kit/hooks/inject-rules.mjs` — concatenates `rules/*.md` sorted by name → `hookSpecificOutput.additionalContext` JSON.
- `plugins/agent-kit/rules/*.md` — the injected rules. `NN-` prefix = order.
- `plugins/agent-kit/agents/{orchestrator,worker}.md` — subagents.
- `template/` — copied into new projects (LF config, short CLAUDE.md skeleton, settings enabling the plugin).

## Rules for editing rules

- Injected into EVERY session's context: keep total small (currently ~40 lines / ~5.7 KB; stay under ~150 lines). Concise imperative bullets, one-clause "why". Cut words, not information.
- Generic only: no project names, paths, packages, ticket numbers.
- Plugins cannot ship CLAUDE.md; project specifics belong in the project's own CLAUDE.md.

## Test

```sh
node plugins/agent-kit/hooks/inject-rules.mjs --text | wc -l -c   # rules + size
node plugins/agent-kit/hooks/inject-rules.mjs | node -e "JSON.parse(require('fs').readFileSync(0,'utf8'))"
claude plugin validate --strict ./plugins/agent-kit
claude plugin validate --strict .
claude --plugin-dir ./plugins/agent-kit   # try it for one session, no install
git ls-files --eol | grep -v "i/lf" # must print nothing (binary excepted)
```

LF everywhere (`.gitattributes`, `.editorconfig`).
