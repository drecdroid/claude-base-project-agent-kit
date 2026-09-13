// Provenance: copied from drecdroid/tdm-app tools/gw/cmdline.go (same author),
// with its tests (cmdline_test.go, run_windows_test.go). Keep the two in step.
package execx

import (
	"path/filepath"
	"strings"
)

// This file is the whole answer to the Windows ".cmd shim" problem, and it is
// pure string manipulation so that it can be unit-tested on any platform.
//
// The problem, confirmed live against Go 1.27 before any of this was written:
//
//   - `pnpm`, `npx`, `turbo`, `tsc` and friends are not .exe files on Windows,
//     they are .cmd shims. Go's exec.LookPath DOES find them (it honours
//     PATHEXT), and CreateProcess DOES run them (it hands a .bat/.cmd off to
//     cmd.exe implicitly), so the naive `exec.Command("pnpm", args...)` appears
//     to work.
//   - It only appears to. Go quotes the command line with CommandLineToArgvW
//     rules — which its own doc comment says are NOT cmd.exe's rules — and then
//     cmd.exe re-parses that line with its own. An argument containing "&" is
//     therefore not an argument at all: `gw x pnpm run "a&whoami"` executed
//     whoami. `^` was eaten, `%PATH%` was expanded, `|` and `>` were honoured
//     as redirection, and an embedded quote produced "The syntax of the command
//     is incorrect."
//
// The fix is the one Go's own documentation points at: build the command line
// by hand and hand it to cmd.exe ourselves via SysProcAttr.CmdLine.
//
//   cmd.exe /d /s /c "<argv-quoted prog> <cmd-safe arg> ..."
//
//   /d  skip AutoRun commands from the registry (a user's AutoRun would
//       otherwise run inside every single a ckit child process).
//   /s  take the string after /c, strip its outermost quote pair, and use the
//       rest verbatim — which is what makes the per-argument quotes below
//       survive cmd's own tokenizer instead of being re-balanced by it.
//
// Each argument is quoted twice over, for the two parsers it has to survive:
// argvQuote for the .exe at the far end of the shim, then percentSplit so that
// cmd.exe cannot expand %VAR% on the way through.

// isBatchPath reports whether an executable path is a batch file, i.e. one that
// Windows will run through cmd.exe rather than directly.
func isBatchPath(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".cmd", ".bat":
		return true
	}
	return false
}

// argvQuote quotes one argument with the CommandLineToArgvW rules that the
// final executable parses its command line with: wrap in quotes, escape an
// embedded quote as \", and double any run of backslashes that immediately
// precedes a quote (including the closing one, so a trailing backslash does not
// escape it).
func argvQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
			b.WriteByte('"')
			slashes = 0
		default:
			b.WriteString(strings.Repeat(`\`, slashes))
			slashes = 0
			b.WriteByte(c)
		}
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	b.WriteByte('"')
	return b.String()
}

// cmdQuoteArg makes one argument safe for the cmd.exe hop as well.
//
// Inside a double-quoted run, cmd leaves &, |, <, >, ^ and spaces alone — but
// it still expands %VAR%, and there is no escape for "%" that works inside
// quotes. The trick that does work is to end the quoted run before each "%",
// emit it as ^% outside quotes (where a caret does suppress it), and reopen:
//
//	pct%PATH%pct   ->   "pct"^%"PATH"^%"pct"
//
// cmd concatenates the adjacent runs back into a single token, and the far-end
// executable sees the literal text. Verified round-trip against a real .cmd
// shim for &, |, ^, <, >, %VAR%, a trailing backslash, an embedded quote, a
// bare %, and the empty string.
func cmdQuoteArg(s string) string {
	q := argvQuote(s)
	if !strings.Contains(q, "%") {
		return q
	}
	inner := q[1 : len(q)-1] // strip argvQuote's own outer quotes
	parts := strings.Split(inner, "%")
	for i := range parts {
		parts[i] = `"` + parts[i] + `"`
	}
	return strings.Join(parts, `^%`)
}

// buildBatchCmdLine is the full command line for cmd.exe, i.e. everything after
// the executable name itself.
func buildBatchCmdLine(prog string, args []string) string {
	var b strings.Builder
	b.WriteString(`/d /s /c "`)
	b.WriteString(argvQuote(prog))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(cmdQuoteArg(a))
	}
	b.WriteByte('"')
	return b.String()
}
