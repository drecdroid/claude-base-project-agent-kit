package execx

import (
	"strings"
	"testing"
)

func TestIsBatchPath(t *testing.T) {
	cases := map[string]bool{
		`C:\Users\x\AppData\Roaming\npm\pnpm.cmd`: true,
		`C:\tools\thing.BAT`:                      true,
		`C:\Program Files\Git\cmd\git.exe`:        false,
		`/usr/bin/git`:                            false,
		`pnpm`:                                    false,
	}
	for p, want := range cases {
		if got := isBatchPath(p); got != want {
			t.Errorf("isBatchPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestArgvQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`with space`, `"with space"`},
		{``, `""`},
		// A trailing backslash must not escape the closing quote.
		{`C:\some\path\`, `"C:\some\path\\"`},
		{`C:\a\\`, `"C:\a\\\\"`},
		// An embedded quote is escaped, and the backslashes before it doubled.
		{`say "hi"`, `"say \"hi\""`},
		{`a\"b`, `"a\\\"b"`},
	}
	for _, tc := range cases {
		if got := argvQuote(tc.in); got != tc.want {
			t.Errorf("argvQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCmdQuoteArgLeavesOrdinaryArgsAlone(t *testing.T) {
	// No % means nothing for cmd to expand, so the plain argv quoting stands.
	for _, in := range []string{"run", "--filter", "@tdm/frontend", "a&b", "a|b", "a^b", "a>b"} {
		got := cmdQuoteArg(in)
		if got != argvQuote(in) {
			t.Errorf("cmdQuoteArg(%q) = %q, want %q", in, got, argvQuote(in))
		}
		if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
			t.Errorf("cmdQuoteArg(%q) = %q is not fully quoted; cmd would treat & | ^ > as syntax", in, got)
		}
	}
}

func TestCmdQuoteArgBreaksUpPercent(t *testing.T) {
	// The only metacharacter cmd still honours inside a quoted run is %, and
	// there is no in-quote escape for it — so the quoted run is ended, the %
	// emitted as ^% outside quotes, and the run reopened. cmd glues the pieces
	// back into one token.
	cases := []struct{ in, want string }{
		{`pct%PATH%pct`, `"pct"^%"PATH"^%"pct"`},
		{`%PATH%`, `""^%"PATH"^%""`},
		{`100%`, `"100"^%""`},
		{`--format=%H`, `"--format="^%"H"`},
	}
	for _, tc := range cases {
		if got := cmdQuoteArg(tc.in); got != tc.want {
			t.Errorf("cmdQuoteArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildBatchCmdLine(t *testing.T) {
	got := buildBatchCmdLine(`C:\npm\pnpm.cmd`, []string{"run", "dev"})
	want := `/d /s /c ""C:\npm\pnpm.cmd" "run" "dev""`
	if got != want {
		t.Errorf("buildBatchCmdLine =\n  %s\nwant\n  %s", got, want)
	}
	// /d must be there: without it a user's AutoRun registry command would run
	// inside every a ckit child process.
	if !strings.HasPrefix(got, "/d ") {
		t.Error("missing /d")
	}
	// /s plus the outer quote pair is what stops cmd re-balancing the inner
	// per-argument quotes.
	if !strings.Contains(got, "/s /c \"") || !strings.HasSuffix(got, `"`) {
		t.Error("the command string must be wrapped in its own quote pair for /s")
	}
}

func TestBuildBatchCmdLineNoArgs(t *testing.T) {
	got := buildBatchCmdLine(`C:\npm\pnpm.cmd`, nil)
	if got != `/d /s /c ""C:\npm\pnpm.cmd""` {
		t.Errorf("got %s", got)
	}
}
