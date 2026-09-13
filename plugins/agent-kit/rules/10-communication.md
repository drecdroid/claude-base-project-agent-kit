## Communication & research
- Extreme concision in chat and commit messages; sacrifice grammar. Why: faster to read, fewer tokens.
- End every plan with a concise unresolved-questions list. Why: user answers in one batch.
- Asking the user: every option carries its argument/trade-off; end with one free-form open question. Why: bare options hide cost; the open question catches what options missed.
- Source of truth = code AND its comments (maintained with the code, useful to agents). Docs/memory are NOT authoritative: grep/read the real file before acting on a doc claim (path, mechanism, "what reads this"); flag or fix a stale doc in passing. Why: docs drift silently.
- Reproduce before believing: cheaply re-check any "flaky / environmental / pre-existing" diagnosis (yours or a subagent's) before acting on it. Why: such diagnoses have hidden real bugs.
- "Trimmed for now / see ticket" in delivered work = scope cut: surface it, never count it done.
