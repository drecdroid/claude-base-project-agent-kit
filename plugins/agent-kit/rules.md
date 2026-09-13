# Agent rules (agent-kit plugin; generic, project CLAUDE.md wins on conflict)

## Communication & research
- Extreme concision in chat and commit messages; sacrifice grammar. Why: faster to read, fewer tokens.
- End every plan with a concise unresolved-questions list. Why: user answers in one batch.
- Asking the user: every option carries its argument/trade-off; end with one free-form open question. Why: bare options hide cost; the open question catches what options missed.
- Source of truth = code AND its comments (maintained with the code, useful to agents). Docs/memory are NOT authoritative: grep/read the real file before acting on a doc claim (path, mechanism, "what reads this"); flag or fix a stale doc in passing. Why: docs drift silently.
- Reproduce before believing: cheaply re-check any "flaky / environmental / pre-existing" diagnosis (yours or a subagent's) before acting on it. Why: such diagnoses have hidden real bugs.
- "Trimmed for now / see ticket" in delivered work = scope cut: surface it, never count it done.

## Agent fleet (recommended, your call)
- Delegation and serialization are recommended, not mandatory; decide per task (small, sequential, needs your context -> do it yourself).
- Going by delegation -> FIRST present the delegation plan for user approval: tree, one line per node = kind (self/worker/orchestrator), model/effort, what, concurrency. Why: fan-out multiplies token spend; user may choose otherwise.
- Delegating: main agent plans, answers, coordinates, reviews/merges; workers code. Launch workers in the background. Why: user keeps talking; a foreground subagent blocks the chat.
- Serial by default for work touching shared files; batch only disjoint-file work; parallelize for latency, never to "save". Why: parallel editors re-derive context N times and conflict.
- Every editing worker gets its own git worktree (`isolation: "worktree"`). Why: a non-isolated worker runs in the parent's checkout and overwrites it.
- Never wait by ending a turn (nothing resumes a stopped agent): suites/builds in the foreground with a big timeout; block on background jobs in-turn (`TaskOutput` block=true, or a foreground until-loop); `Monitor` is not a wait.
- Commit after every verified step (WIP fine, squash later). Why: a usage cap kills every in-flight agent; only committed work survives.
- Dead worker (cap/watchdog/stopped): commit its WIP in its worktree, kill its leftovers, relaunch a fresh isolated worker with a takeover brief (branch/sha to `merge --ff-only`/cherry-pick, done vs not done). Same step dies twice -> split it.
- Verification evidence when the situation calls for it (merge, claimed fix, final report), not ritually: name command + count ("vitest 222/222"), separate verified-live from assumed.

## Git
- Never reset/destructively overwrite the user's checkout (`reset --hard`, `checkout -- <paths>`, `clean`, `stash`, branch switch) without first reading `git status --short` and preserving changes (WIP commit on a branch, or copy aside; say so). Prefer history surgery in a separate worktree. Why: the user edits there live; uncommitted work is unrecoverable by git.
- LF everywhere, repo and working trees: `.gitattributes` `* text=auto eol=lf` + `.editorconfig` `end_of_line = lf`. Write new files LF; never convert to CRLF "to match". A CRLF file is the bug.

## Process hygiene
- Stop every process you start (servers, watchers, apps) before finishing; say so. Why: orphans hold ports, locks, worktrees.
- Find leftovers by command line containing your own worktree path, never by port. Why: the port may be a sibling agent's or the user's server.
- Agent-launched apps use an isolated, SHORT data dir, never the user's real profile/config. Why: agent test data once replaced the user's own app state.
- Agent-launched GUI apps never steal window focus (use/add a no-focus mode). Why: focus theft disrupts the user (e.g. minimizes a fullscreen game).

## Windows (mind even when building cross-platform)
- Shell tool: commands over ~7k chars fail before running -> split files into chunks, use a script or the Write tool. Heredocs collapse `\\` to `\` -> no backslash-heavy heredocs.
- Wait loops grepping a log: forward-slash path, no `2>/dev/null` (`until grep -q READY C:/tmp/x.log; do sleep 2; done`). Why: a backslash path fails silently under Git Bash; a loop spun 24 min.
- Package-identity redirection: processes under an MSIX package identity (e.g. a terminal inside the Claude desktop app) get `%APPDATA%` writes silently redirected to `%LOCALAPPDATA%\Packages\<id>\LocalCache\Roaming`; same app launched two ways -> two disconnected copies of state/keys. Keep app data in a plain folder outside `%APPDATA%`; pin third-party tools' config paths via their env vars.
- `.cmd`/`.bat` shims (npm, npx, pnpm, tsc...): plain exec with an argv array still goes through cmd.exe's parser -> arg `a&echo pwned` executes, `^` eaten, `%PATH%` expanded, `|`/`>` redirect. Resolve via PATHEXT and build the line yourself: `cmd.exe /d /s /c "<quoted prog> <cmd-escaped args>"` (split `%`), or run the underlying `.js` with `node`. Why: argument injection.
- Kill trees with `taskkill /PID <pid> /T /F` from the true top PID (look it up from Windows by command line); MSYS/Git Bash `$$`/`$!` != Windows PID; killing only the launcher leaves orphans.
- Each new binary path listening on `0.0.0.0` pops its own Firewall prompt (unclickable unattended; every worktree copy is a new path) -> bind `127.0.0.1` in dev/tests. Loopback never prompts.
- MAX_PATH (260): deep paths break tools (SQLite `CANTOPEN` in a deep `%TEMP%` dir) -> keep isolated data dirs short (`%TEMP%\app-qa`).
