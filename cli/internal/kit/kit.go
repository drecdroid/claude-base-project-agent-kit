// Package kit holds the identifiers of the agent kit itself, shared by every
// command (and by part 2's `ckit new`).
package kit

const (
	// Repo is the GitHub owner/name.
	Repo = "drecdroid/claude-base-project-agent-kit"
	// RepoGitURL is the source for `claude plugin marketplace add`. HTTPS, not
	// the owner/repo shorthand: claude clones the shorthand over SSH, which
	// fails on machines without github.com in known_hosts (confirmed live).
	// The marketplace NAME comes from .claude-plugin/marketplace.json, so it
	// is the same for either source (confirmed live).
	RepoGitURL = "https://github.com/" + Repo + ".git"
	// Marketplace is the marketplace name declared in .claude-plugin/marketplace.json.
	Marketplace = "claude-base-project-agent-kit"
	// Plugin is the plugin name.
	Plugin = "agent-kit"
)

// PluginRef is plugin@marketplace.
func PluginRef() string { return Plugin + "@" + Marketplace }
