# claude-base-project-agent-kit

A Claude Code plugin (`agent-kit`) plus a project starter template.

- **Session rules**: a `SessionStart` hook (on startup, resume, `/clear`, compact) injects generic
  agent rules into context — communication & research, agent fleet, git, process hygiene, Windows
  gotchas. Source: `plugins/agent-kit/rules/*.md` (~5.7 KB total).
- **Subagents**: `orchestrator` (do-it-yourself vs delegate, model/effort table, concurrency plan,
  delegation-plan approval, dead-worker recovery) and `worker` (leaf, never waits by ending its
  turn, checkpoint commits, verified-vs-assumed returns).
- **Template** (`template/`): `.gitattributes`/`.editorconfig` (LF), a short `CLAUDE.md` skeleton for
  project specifics, `.claude/settings.json` enabling the plugin.

Requires `node` on PATH (the hook runs `node <script>` directly, no shell, so it works on
Windows/macOS/Linux alike).

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

1. Edit rules/agents; keep rules concise (every session pays for them).
2. Bump `version` in `plugins/agent-kit/.claude-plugin/plugin.json` and `.claude-plugin/marketplace.json`.
3. `claude plugin validate --strict ./plugins/agent-kit && claude plugin validate --strict .`, commit, push.
4. Users: `/plugin marketplace update claude-base-project-agent-kit` then
   `claude plugin update agent-kit@claude-base-project-agent-kit` (restart to apply). Private repos
   need a git credential helper for background auto-update (see Claude Code's plugin-marketplaces docs).
