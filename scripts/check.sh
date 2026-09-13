#!/bin/sh
# Node-free repo checks (CI runs this; run locally too): rules.md in sync with
# rules/*.md, every JSON file parses, plugin structure present, versions match,
# no CRLF in tracked files. Needs sh + python3 (or python) + git.
set -eu
cd "$(dirname "$0")/.."
fail=0
PY=$(command -v python3 || command -v python) || { echo "need python3 for JSON checks" >&2; exit 2; }

sh scripts/build-rules.sh --check || fail=1

# ASCII-only rules: Windows PowerShell 5 (the hook's fallback shell) reads a
# BOM-less UTF-8 file as ANSI, so any non-ASCII char reaches the model mangled.
if LC_ALL=C grep -n '[^[:print:][:space:]]' plugins/agent-kit/rules.md >&2; then
  echo "non-ASCII in rules (breaks the PowerShell hook fallback); use ASCII (-> ... !=)" >&2
  fail=1
fi

for f in $(git ls-files '*.json'); do
  "$PY" -c 'import json,sys; json.load(open(sys.argv[1], encoding="utf-8"))' "$f" \
    || { echo "invalid JSON: $f" >&2; fail=1; }
done

for f in .claude-plugin/marketplace.json plugins/agent-kit/.claude-plugin/plugin.json \
  plugins/agent-kit/hooks/hooks.json plugins/agent-kit/rules.md \
  plugins/agent-kit/agents/orchestrator.md plugins/agent-kit/agents/worker.md; do
  [ -f "$f" ] || { echo "missing: $f" >&2; fail=1; }
done

"$PY" - <<'PY' || fail=1
import json, sys
m = json.load(open(".claude-plugin/marketplace.json", encoding="utf-8"))
p = json.load(open("plugins/agent-kit/.claude-plugin/plugin.json", encoding="utf-8"))
h = json.load(open("plugins/agent-kit/hooks/hooks.json", encoding="utf-8"))
entry = next((e for e in m["plugins"] if e["name"] == p["name"]), None)
errs = []
if entry is None:
    errs.append("plugin %s not listed in marketplace.json" % p["name"])
elif entry.get("version") != p.get("version"):
    errs.append("version mismatch: marketplace %s vs plugin %s" % (entry.get("version"), p.get("version")))
cmds = [x["command"] for g in h["hooks"]["SessionStart"] for x in g["hooks"]]
if not any("rules.md" in c for c in cmds):
    errs.append("SessionStart hook no longer prints rules.md")
for e in errs:
    print(e, file=sys.stderr)
sys.exit(1 if errs else 0)
PY

# ckit (cli/): gofmt, vet, tests. Skipped with a note when go is not on PATH
# (CI installs it with setup-go); REQUIRE_GO=1 turns the skip into a failure.
if [ -f cli/go.mod ]; then
  GO=$(command -v go || true)
  if [ -n "$GO" ]; then
    unformatted=$(cd cli && gofmt -l .)
    [ -z "$unformatted" ] || { echo "gofmt needed in cli/:" >&2; echo "$unformatted" >&2; fail=1; }
    (cd cli && "$GO" vet ./...) || fail=1
    (cd cli && "$GO" test ./...) || fail=1
  elif [ "${REQUIRE_GO:-0}" = 1 ]; then
    echo "go not on PATH (REQUIRE_GO=1)" >&2; fail=1
  else
    echo "note: go not on PATH; skipped cli/ checks" >&2
  fi
fi

crlf=$(git ls-files --eol | grep -v 'i/lf' | grep -v 'i/-text' || true)
[ -z "$crlf" ] || { echo "non-LF tracked files:" >&2; echo "$crlf" >&2; fail=1; }

[ "$fail" = 0 ] && echo "all checks passed"
exit "$fail"
