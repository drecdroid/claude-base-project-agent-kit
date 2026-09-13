// Package kit holds the identifiers of the agent kit itself, shared by every
// command (and by part 2's `ckit new`).
package kit

const (
	// Repo is the GitHub source for `claude plugin marketplace add`.
	Repo = "drecdroid/claude-base-project-agent-kit"
	// Marketplace is the marketplace name declared in .claude-plugin/marketplace.json.
	Marketplace = "claude-base-project-agent-kit"
	// Plugin is the plugin name.
	Plugin = "agent-kit"
)

// PluginRef is plugin@marketplace.
func PluginRef() string { return Plugin + "@" + Marketplace }
