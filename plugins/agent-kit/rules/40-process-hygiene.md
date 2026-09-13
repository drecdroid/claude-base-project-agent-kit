## Process hygiene
- Stop every process you start (servers, watchers, apps) before finishing; say so. Why: orphans hold ports, locks, worktrees.
- Find leftovers by command line containing your own worktree path, never by port. Why: the port may be a sibling agent's or the user's server.
- Agent-launched apps use an isolated, SHORT data dir, never the user's real profile/config. Why: agent test data once replaced the user's own app state.
- Agent-launched GUI apps never steal window focus (use/add a no-focus mode). Why: focus theft disrupts the user (e.g. minimizes a fullscreen game).
