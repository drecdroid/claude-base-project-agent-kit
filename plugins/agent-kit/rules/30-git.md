## Git
- Never reset/destructively overwrite the user's checkout (`reset --hard`, `checkout -- <paths>`, `clean`, `stash`, branch switch) without first reading `git status --short` and preserving changes (WIP commit on a branch, or copy aside; say so). Prefer history surgery in a separate worktree. Why: the user edits there live; uncommitted work is unrecoverable by git.
- LF everywhere, repo and working trees: `.gitattributes` `* text=auto eol=lf` + `.editorconfig` `end_of_line = lf`. Write new files LF; never convert to CRLF "to match". A CRLF file is the bug.
