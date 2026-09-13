# claude-base-project-agent-kit

A Claude Code plugin (`agent-kit`) plus a project starter template.

- **Session rules**: a `SessionStart` hook (on startup, resume, `/clear`, compact) injects generic
  agent rules into context — communication & research, agent fleet, git, process hygiene, Windows
  gotchas. Edit `plugins/agent-kit/rules/*.md`; the hook prints the combined
  `plugins/agent-kit/rules.md` (~5.7 KB) with `cat`, no runtime needed.
- **Subagents**: `orchestrator` (do-it-yourself vs delegate, model/effort table, concurrency plan,
  delegation-plan approval, dead-worker recovery) and `worker` (leaf, never waits by ending its
  turn, checkpoint commits, verified-vs-assumed returns).
- **Template** (`template/`): `.gitattributes`/`.editorconfig` (LF), a short `CLAUDE.md` skeleton for
  project specifics, `.claude/settings.json` enabling the plugin.

No dependencies. The hook is a shell-form `cat` (Git Bash on Windows, `sh` on macOS/Linux); it
also works when Claude Code falls back to PowerShell (no Git Bash), where `cat` is `Get-Content`.
Rules must stay ASCII-only: Windows PowerShell 5 reads the BOM-less file as ANSI and would mangle
anything else (`scripts/check.sh` enforces it).

## Install

Private repo: git must be able to clone it (`gh auth login` + `gh auth setup-git`).

```
/plugin marketplace add drecdroid/claude-base-project-agent-kit
/plugin install agent-kit@claude-base-project-agent-kit
```

Or try once without installing: `claude --plugin-dir <clone>/plugins/agent-kit`.

## Enable per project

Commit this to the project's `.claude/settings.json` (already in `template/`); Claude Code prompts
to add the marketplace and enable the plugin when the project is trusted:

```json
{
  "extraKnownMarketplaces": {
    "claude-base-project-agent-kit": {
      "source": { "source": "github", "repo": "drecdroid/claude-base-project-agent-kit" }
    }
  },
  "enabledPlugins": { "agent-kit@claude-base-project-agent-kit": true }
}
```

Or `claude plugin install agent-kit@claude-base-project-agent-kit --scope project`.

## New project from the template

Copy `template/` contents (including dotfiles) into the new repo root, fill in `CLAUDE.md`
(architecture, commands, testing, gotchas — project specifics only; generic rules come from the
plugin). Use the orchestrator as the main agent with `claude --agent <name>` (check `/agents` for
the plugin-namespaced name) or let the main agent spawn it.

## Update

1. Edit rules/agents; keep rules concise (every session pays for them) and ASCII-only.
2. Rebuild the combined file: `sh scripts/build-rules.sh`. PowerShell without `sh`:
   ```powershell
   $h = '# Agent rules (agent-kit plugin; generic, project CLAUDE.md wins on conflict)'
   $b = Get-ChildItem plugins/agent-kit/rules/*.md | Sort-Object Name | ForEach-Object { (Get-Content $_ -Raw).Trim() }
   [IO.File]::WriteAllText("$PWD/plugins/agent-kit/rules.md", ((@($h) + $b) -join "`n`n") + "`n")
   ```
   `sh scripts/check.sh` (CI runs it too) fails if `rules.md` is stale or non-ASCII, a JSON file is
   invalid, the two versions differ, or a tracked file is CRLF.
3. Bump `version` in `plugins/agent-kit/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`.
4. `claude plugin validate --strict ./plugins/agent-kit && claude plugin validate --strict .`, commit, push.
5. Users: `/plugin marketplace update claude-base-project-agent-kit` then
   `claude plugin update agent-kit@claude-base-project-agent-kit` (restart to apply). Private repos
   need a git credential helper for background auto-update (see Claude Code's plugin-marketplaces docs).
