#!/bin/sh
# Rebuilds plugins/agent-kit/rules.md (the file the SessionStart hook prints)
# from plugins/agent-kit/rules/*.md, sorted by name. POSIX sh, no node.
# `--check`: exit 1 if rules.md is stale instead of rewriting it.
set -eu
cd "$(dirname "$0")/../plugins/agent-kit"
HEADER='# Agent rules (agent-kit plugin; generic, project CLAUDE.md wins on conflict)'

render() {
  printf '%s\n' "$HEADER"
  for f in $(LC_ALL=C ls rules/*.md); do
    printf '\n'
    # strip CR, trailing blank lines
    tr -d '\r' < "$f" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
  done
}

if [ "${1:-}" = "--check" ]; then
  if render | cmp -s - rules.md; then
    echo "rules.md up to date"
  else
    echo "rules.md is stale: run scripts/build-rules.sh" >&2
    render | diff rules.md - >&2 || true
    exit 1
  fi
else
  render > rules.md
  echo "wrote plugins/agent-kit/rules.md ($(wc -l < rules.md) lines)"
fi
