// Package execx is the ONE way ckit runs an external program. On Windows a
// .cmd/.bat shim (code.cmd, npm-installed claude.cmd, ...) is routed through a
// hand-built cmd.exe line (see cmdline.go) so arguments reach the far end
// verbatim; everything else is a plain exec.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Cmd is one external invocation.
type Cmd struct {
	Dir  string   // working directory ("" = inherit)
	Prog string   // bare name (PATH/PATHEXT lookup) or a path
	Args []string // arguments, passed verbatim
	Env  []string // extra KEY=VALUE entries on top of os.Environ
}

// Argv is Prog followed by Args.
func (c Cmd) Argv() []string { return append([]string{c.Prog}, c.Args...) }

// Runner runs commands. Real is the production one; tests use a fake.
type Runner interface {
	// Run runs to completion with inherited stdio and returns the exit code.
	Run(ctx context.Context, c Cmd) (int, error)
	// Start launches a detached-ish GUI/launcher process and does not wait.
	Start(ctx context.Context, c Cmd) error
	// Output runs to completion capturing stdout+stderr (for probes).
	Output(ctx context.Context, c Cmd) (string, int, error)
	// LookPath resolves a program name like the runner would.
	LookPath(prog string) (string, error)
}

// Real is the production Runner.
type Real struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

// ErrNotFound wraps a failed program lookup.
var ErrNotFound = errors.New("program not found")

// ResolveProgram turns a program name into a concrete path. A bare name goes
// through exec.LookPath (PATHEXT-aware on Windows, which finds code.cmd for
// "code"); a relative path with a separator is resolved against dir.
func ResolveProgram(dir, prog string) (string, error) {
	cand := prog
	if filepath.Base(prog) != prog && !filepath.IsAbs(prog) && dir != "" {
		cand = filepath.Join(dir, prog)
	}
	p, err := exec.LookPath(cand)
	if err != nil {
		if errors.Is(err, exec.ErrDot) {
			return p, nil
		}
		return "", fmt.Errorf("%w: %q (%v)", ErrNotFound, prog, err)
	}
	return p, nil
}

// BuildCommand prepares the child process: resolve, then .cmd/.bat via the
// safe cmd.exe line on Windows, else plain exec.
func BuildCommand(c Cmd) (*exec.Cmd, error) {
	resolved, err := ResolveProgram(c.Dir, c.Prog)
	if err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" && isBatchPath(resolved) {
		cmd, err = batchCommand(resolved, c.Args)
		if err != nil {
			return nil, err
		}
	} else {
		cmd = exec.Command(resolved, c.Args...)
	}
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd, nil
}

func (r Real) streams() (io.Reader, io.Writer, io.Writer) {
	in, out, errw := r.Stdin, r.Stdout, r.Stderr
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	if errw == nil {
		errw = os.Stderr
	}
	return in, out, errw
}

func (r Real) Run(ctx context.Context, c Cmd) (int, error) {
	cmd, err := BuildCommand(c)
	if err != nil {
		return 127, err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = r.streams()
	return exitCode(runCtx(ctx, cmd))
}

func (r Real) Start(ctx context.Context, c Cmd) error {
	cmd, err := BuildCommand(c)
	if err != nil {
		return err
	}
	_, out, errw := r.streams()
	cmd.Stdout, cmd.Stderr = out, errw
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (r Real) Output(ctx context.Context, c Cmd) (string, int, error) {
	cmd, err := BuildCommand(c)
	if err != nil {
		return "", 127, err
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	code, err := exitCode(runCtx(ctx, cmd))
	return buf.String(), code, err
}

func (r Real) LookPath(prog string) (string, error) { return ResolveProgram("", prog) }

func runCtx(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

// exitCode maps a run error to (exit code, error-that-is-not-just-a-code).
func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 126, err
}

// Format renders argv for a --dry-run line: each argument quoted only when it
// needs it, so the output is readable and unambiguous.
func Format(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && !strings.ContainsAny(a, " \t\"'&|<>^%;()$`\\*?!#~=") {
			parts[i] = a
			continue
		}
		if a != "" && !strings.ContainsAny(a, " \t\"'") {
			parts[i] = a // metachars but no whitespace/quotes: still one token
			continue
		}
		parts[i] = `'` + strings.ReplaceAll(a, `'`, `'\''`) + `'`
	}
	return strings.Join(parts, " ")
}
