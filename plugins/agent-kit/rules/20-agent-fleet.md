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
