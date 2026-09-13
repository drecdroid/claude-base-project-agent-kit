//go:build !windows

package execx

import (
	"errors"
	"os/exec"
)

// batchCommand never applies off Windows; .cmd/.bat are not executable there.
func batchCommand(prog string, args []string) (*exec.Cmd, error) {
	return nil, errors.New("batch files are only executable on Windows")
}
