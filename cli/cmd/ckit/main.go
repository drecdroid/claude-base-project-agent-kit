// Command ckit automates using the Claude agent kit.
//
//	go install github.com/drecdroid/claude-base-project-agent-kit/cli/cmd/ckit@latest
package main

import (
	"context"
	"os"

	"github.com/drecdroid/claude-base-project-agent-kit/cli/internal/app"
)

func main() {
	os.Exit(app.Main(context.Background(), app.DefaultEnv(), os.Args))
}
