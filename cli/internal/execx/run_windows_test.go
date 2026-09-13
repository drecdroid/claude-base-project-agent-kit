//go:build windows

package execx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCkitShimHelper is not a test: it is the far end of the .cmd shim built by
// TestBatchShimPassesArgumentsVerbatim. When the environment marker is set it
// prints its own argv and exits, standing in for the real executable a shim
// like pnpm.cmd forwards to.
func TestCkitShimHelper(t *testing.T) {
	if os.Getenv("CKIT_SHIM_HELPER") != "1" {
		t.Skip("helper process, not a test")
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	for _, a := range args {
		fmt.Printf("ARG<%s>\n", a)
	}
	os.Exit(0)
}

// writeShim creates a .cmd file shaped exactly like the npm/pnpm shims: a batch
// file that forwards %* to a real executable.
func writeShim(t *testing.T, dir string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "ckitshim.cmd")
	body := "@echo off\r\n\"" + self + "\" -test.run=TestCkitShimHelper -- %*\r\n"
	if err := os.WriteFile(shim, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return shim
}

func runShim(t *testing.T, shim, dir string, args []string) []string {
	t.Helper()
	cmd, err := BuildCommand(Cmd{Dir: dir, Prog: shim, Args: args, Env: []string{"CKIT_SHIM_HELPER=1"}})
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\noutput:\n%s", err, out.String())
	}
	var got []string
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimRight(line, "\r")
		if s, ok := strings.CutPrefix(line, "ARG<"); ok && strings.HasSuffix(s, ">") {
			got = append(got, strings.TrimSuffix(s, ">"))
		}
	}
	return got
}

// TestBatchShimPassesArgumentsVerbatim is the regression test for the single
// most likely thing to be broken on Windows.
//
// `pnpm`, `npx` and friends are .cmd shims. Go's own exec quoting is
// CommandLineToArgvW-shaped, cmd.exe re-parses it with different rules, and the
// result — confirmed live against Go 1.27 before gw (tdm-app) existed — is that "a&b"
// executes b, "^" disappears, "%PATH%" is expanded and an embedded quote is a
// syntax error. Every case below FAILED with plain exec.Command.
func TestBatchShimPassesArgumentsVerbatim(t *testing.T) {
	dir := t.TempDir()
	shim := writeShim(t, dir)

	cases := [][]string{
		{"plain"},
		{"with space"},
		{"run", "dev"},
		{"amp&echo pwned"},     // command injection through cmd.exe
		{"pipe|echo pwned"},    // ditto, via a pipe
		{"redirect>pwned.txt"}, // ditto, via redirection
		{"car^et"},             // cmd's own escape character
		{"pct%PATH%pct"},       // environment expansion inside the argument
		{"%PATH%"},
		{"--format=%H"}, // a lone % is fine and must stay
		{"100%"},
		{`C:\some\path\`}, // trailing backslash vs the closing quote
		{`say "hi"`},      // embedded quotes
		{"--filter", "@tdm/frontend", "run", "test"},
		{"(paren)", "[bracket]", "{brace}", "semi;colon", "tilde~"},
		{""},
	}
	for _, args := range cases {
		got := runShim(t, shim, dir, args)
		if len(got) != len(args) {
			t.Errorf("args %q: got %d arguments back (%q), want %d", args, len(got), got, len(args))
			continue
		}
		for i := range args {
			if got[i] != args[i] {
				t.Errorf("args %q: argv[%d] = %q, want %q", args, i, got[i], args[i])
			}
		}
	}
	// The injection cases must also not have had any side effect.
	if _, err := os.Stat(filepath.Join(dir, "pwned.txt")); err == nil {
		t.Fatal("redirection escaped the quoting: pwned.txt was created")
	}
}

// TestBatchShimRunsInTargetDirectory is the other half of running a command:
// the command has to run in the WORKTREE, not where ckit was invoked.
func TestBatchShimRunsInTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	shim := writeShim(t, dir)
	target := filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := BuildCommand(Cmd{Dir: target, Prog: shim, Args: []string{"x"}, Env: []string{"CKIT_SHIM_HELPER=1"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(filepath.Clean(cmd.Dir), filepath.Clean(target)) {
		t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, target)
	}
}

// TestResolveProgramFindsCmdShimOnPath covers the PATHEXT half: ckit
// must resolve "pnpm" to "pnpm.cmd" without the caller naming the extension.
func TestResolveProgramFindsCmdShimOnPath(t *testing.T) {
	dir := t.TempDir()
	writeShim(t, dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := ResolveProgram(dir, "ckitshim")
	if err != nil {
		t.Fatalf("ResolveProgram: %v", err)
	}
	if !strings.EqualFold(filepath.Ext(got), ".cmd") {
		t.Errorf("resolved %q, want the .cmd shim", got)
	}
	if !isBatchPath(got) {
		t.Errorf("%q must be recognised as a batch file", got)
	}
}

func TestBatchCommandRejectsNewlineArgument(t *testing.T) {
	// A newline cannot survive cmd's line-oriented parsing at all; failing
	// loudly beats silently truncating the command.
	if _, err := batchCommand(`C:\x\pnpm.cmd`, []string{"a\nb"}); err == nil {
		t.Fatal("want an error for an argument containing a newline")
	}
}
