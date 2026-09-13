package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// Placeholders understood in template files. Plain string replacement, no
// template engine: user text containing "{{", "$" or "%" is inserted
// verbatim and can never break or execute anything.
const (
	PHProjectName = "{{project_name}}"
	PHDescription = "{{description}}"
)

// Values fills the placeholders.
type Values struct {
	ProjectName string
	Description string // one line; newlines are collapsed to spaces
}

// OneLine collapses a description to a single trimmed line.
func OneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// Fill replaces placeholders in the given template files (relative to dir)
// and returns the ones it changed. Binary files (a NUL byte) are skipped.
func Fill(dir string, files []string, v Values) ([]string, error) {
	r := strings.NewReplacer(
		PHProjectName, v.ProjectName,
		PHDescription, OneLine(v.Description),
	)
	var changed []string
	for _, rel := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		b, err := os.ReadFile(p)
		if err != nil {
			return changed, err
		}
		if bytes.IndexByte(b, 0) >= 0 || !bytes.Contains(b, []byte("{{")) {
			continue
		}
		out := r.Replace(string(b))
		if out == string(b) {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			return changed, err
		}
		if err := os.WriteFile(p, []byte(out), st.Mode().Perm()); err != nil {
			return changed, err
		}
		changed = append(changed, rel)
	}
	return changed, nil
}
