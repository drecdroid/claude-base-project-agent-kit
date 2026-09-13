//go:build windows

package execx

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// comspec is the command interpreter to route batch files through.
func comspec() string {
	if c := os.Getenv("ComSpec"); c != "" {
		return c
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		return root + `\System32\cmd.exe`
	}
	return `C:\Windows\System32\cmd.exe`
}

// batchCommand builds the cmd.exe invocation for a .cmd/.bat program with a
// hand-built command line — see cmdline.go for why Go's own quoting cannot be
// used here.
func batchCommand(prog string, args []string) (*exec.Cmd, error) {
	for _, a := range args {
		if i := indexAnyByte(a, "\r\n\x00"); i >= 0 {
			return nil, fmt.Errorf("argument %q contains a newline, which cannot be passed through a Windows .cmd shim", a)
		}
	}
	cmd := exec.Command(comspec())
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: buildBatchCmdLine(prog, args)}
	return cmd, nil
}

func indexAnyByte(s, chars string) int {
	for i := 0; i < len(s); i++ {
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				return i
			}
		}
	}
	return -1
}
