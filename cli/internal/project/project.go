// Package project resolves the optional [project] argument every ckit
// command takes: empty -> cwd; an existing path -> that path; a bare name ->
// <projectsDir>/<name>. A miss never dead-ends: the error carries fuzzy
// suggestions and the full candidate list.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NotFoundError is returned when a name matches no project. It is meant to
// be printed as-is, and its Suggestions offered in an interactive picker.
type NotFoundError struct {
	Query       string
	ProjectsDir string
	DirMissing  bool
	Suggestions []string // best fuzzy tier, best first
	All         []string // every project in ProjectsDir, sorted
}

func (e *NotFoundError) Error() string {
	var b strings.Builder
	if e.DirMissing {
		fmt.Fprintf(&b, "project %q not found: projects directory %s does not exist", e.Query, e.ProjectsDir)
		b.WriteString("\n  set it: ckit config set projectsDir <dir>, or pass a path")
		return b.String()
	}
	fmt.Fprintf(&b, "project %q not found in %s", e.Query, e.ProjectsDir)
	if len(e.Suggestions) > 0 {
		fmt.Fprintf(&b, "\n  did you mean: %s", strings.Join(e.Suggestions, ", "))
	}
	if len(e.All) > 0 {
		list := e.All
		more := ""
		if len(list) > 30 {
			more = fmt.Sprintf(" ... (+%d more)", len(list)-30)
			list = list[:30]
		}
		fmt.Fprintf(&b, "\n  projects: %s%s", strings.Join(list, ", "), more)
	} else {
		b.WriteString("\n  (no projects there yet)")
	}
	b.WriteString("\n  or pass a path (./name, an absolute path)")
	return b.String()
}

// looksLikePath reports whether arg is written as a path rather than a bare
// project name.
func looksLikePath(arg string) bool {
	if arg == "." || arg == ".." || filepath.IsAbs(arg) || strings.HasPrefix(arg, "~") {
		return true
	}
	return strings.ContainsAny(arg, `/\`) || filepath.VolumeName(arg) != ""
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Resolve turns [project] into an absolute directory.
func Resolve(arg, projectsDir, cwd, home string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return filepath.Abs(cwd)
	}
	if looksLikePath(arg) {
		p := arg
		if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
			p = filepath.Join(home, strings.TrimLeft(p[1:], `/\`))
		} else if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		p = filepath.Clean(p)
		st, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("path %s does not exist", p)
		}
		if !st.IsDir() {
			return "", fmt.Errorf("path %s is a file, not a project directory", p)
		}
		return p, nil
	}
	// A bare name: projectsDir first, then a directory of that name in cwd.
	if cand := filepath.Join(projectsDir, arg); isDir(cand) {
		return filepath.Abs(cand)
	}
	if cand := filepath.Join(cwd, arg); isDir(cand) {
		return filepath.Abs(cand)
	}
	nf := &NotFoundError{Query: arg, ProjectsDir: projectsDir}
	names, err := List(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			nf.DirMissing = true
			return "", nf
		}
		return "", fmt.Errorf("reading %s: %w", projectsDir, err)
	}
	nf.All = names
	nf.Suggestions = Suggest(names, arg)
	return "", nf
}

// List returns the non-hidden directory names in projectsDir, sorted.
func List(projectsDir string) ([]string, error) {
	ents, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out, nil
}

// Suggest returns the names in the best matching tier for query (exact,
// prefix, substring, then subsequence), best first, at most 5.
func Suggest(names []string, query string) []string {
	_, ms := Match(names, query)
	if len(ms) > 5 {
		ms = ms[:5]
	}
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}
