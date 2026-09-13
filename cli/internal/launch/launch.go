// Package launch builds (never runs) the argv for opening a project in an
// app: VS Code, SmartGit, or Claude Desktop via its claude:// deep link. Pure
// functions of (goos, inputs) so every OS is unit-testable from any OS.
package launch

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/execx"
)

// Apps accepted by `ckit open`.
var Apps = []string{"code", "smartgit", "claude"}

// encode percent-encodes a query value. url.QueryEscape encodes a space as
// "+", which a non-form parser may keep literally, so it is rewritten to
// %20 (a literal "+" is already %2B, so this is unambiguous).
func encode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// ClaudeCodeURL is the Claude Desktop deep link opening a new Claude Code
// session in folder (absolute path), with prompt prefilled when non-empty.
// See https://support.claude.com/en/articles/14729294-open-claude-desktop-with-a-link
// Desktop always asks the user to confirm trusting the folder.
func ClaudeCodeURL(folder, prompt string) string {
	u := "claude://code/new?folder=" + encode(folder)
	if prompt != "" {
		u += "&q=" + encode(prompt)
	}
	return u
}

// OpenURL is the argv that hands a URL to the OS's protocol handler.
//
// Windows uses rundll32 url.dll,FileProtocolHandler, NOT `cmd /c start`:
// cmd's parser would treat the URL's & as a command separator and expand %xx
// sequences that look like variables. rundll32.exe is a real exe, so the URL
// (which after encoding has no spaces or quotes) arrives as one argument.
func OpenURL(goos, u string) execx.Cmd {
	switch goos {
	case "windows":
		return execx.Cmd{Prog: "rundll32", Args: []string{"url.dll,FileProtocolHandler", u}}
	case "darwin":
		return execx.Cmd{Prog: "open", Args: []string{u}}
	default:
		return execx.Cmd{Prog: "xdg-open", Args: []string{u}}
	}
}

// Code is the argv opening dir in VS Code. codePath overrides `code`.
func Code(codePath, dir string) execx.Cmd {
	prog := codePath
	if prog == "" {
		prog = "code"
	}
	return execx.Cmd{Prog: prog, Args: []string{dir}}
}

// SmartGitDefault is where SmartGit lives when not overridden: an absolute
// path on Windows/macOS, a PATH name on Linux.
func SmartGitDefault(goos string) string {
	switch goos {
	case "windows":
		return `C:\Program Files\SmartGit\bin\smartgit.exe`
	case "darwin":
		return "/Applications/SmartGit.app"
	default:
		return "smartgit"
	}
}

// SmartGitProgram is the configured override or the per-OS default.
func SmartGitProgram(goos, override string) string {
	if override != "" {
		return override
	}
	return SmartGitDefault(goos)
}

// SmartGit is the argv opening dir in SmartGit. A macOS .app bundle is
// launched via `open -a <app> <dir>`; anything else is executed with <dir>
// (SmartGit's default command-line action is --open <path>).
func SmartGit(goos, override, dir string) execx.Cmd {
	prog := SmartGitProgram(goos, override)
	if goos == "darwin" && strings.HasSuffix(strings.TrimRight(prog, "/"), ".app") {
		return execx.Cmd{Prog: "open", Args: []string{"-a", prog, dir}}
	}
	return execx.Cmd{Prog: prog, Args: []string{dir}}
}

// ValidateApp checks an app name, listing the valid ones on error.
func ValidateApp(app string) error {
	for _, a := range Apps {
		if a == app {
			return nil
		}
	}
	return fmt.Errorf("unknown app %q; choose one of: %s", app, strings.Join(Apps, ", "))
}
