#!/usr/bin/env node
// SessionStart hook: concatenates rules/*.md (sorted by name) and prints it as
// SessionStart additionalContext JSON. Plain Node, no shell, no deps, so the
// exec-form hook (`node <this file>`) behaves the same on Windows/macOS/Linux.
// `--text` prints the raw rules instead (for eyeballing / size checks).
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = process.env.CLAUDE_PLUGIN_ROOT || join(dirname(fileURLToPath(import.meta.url)), "..");
const rulesDir = join(root, "rules");

let body;
try {
  body = readdirSync(rulesDir)
    .filter((f) => f.endsWith(".md"))
    .sort()
    .map((f) => readFileSync(join(rulesDir, f), "utf8").replace(/\r\n/g, "\n").trim())
    .join("\n\n");
} catch (err) {
  // Never block a session over missing rules; say so on stderr and exit clean.
  process.stderr.write(`agent-kit: could not read rules: ${err.message}\n`);
  process.exit(0);
}

const context = `# Agent rules (agent-kit plugin; generic, project CLAUDE.md wins on conflict)\n\n${body}\n`;

if (process.argv.includes("--text")) {
  process.stdout.write(context);
} else {
  process.stdout.write(
    JSON.stringify({ hookSpecificOutput: { hookEventName: "SessionStart", additionalContext: context } }) + "\n",
  );
}
