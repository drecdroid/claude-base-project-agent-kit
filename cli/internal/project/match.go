package project

// Tiered matcher adapted from drecdroid/tdm-app tools/gw/match.go (same
// author), over plain project names instead of worktrees.

import (
	"sort"
	"strings"
)

// Tier is how well a query matched, best (TierExact) to worst (TierFuzzy).
type Tier int

const (
	TierExact Tier = iota
	TierPrefix
	TierSubstring
	TierFuzzy
	TierNone
)

func (t Tier) String() string {
	switch t {
	case TierExact:
		return "exact"
	case TierPrefix:
		return "prefix"
	case TierSubstring:
		return "substring"
	case TierFuzzy:
		return "fuzzy"
	}
	return "none"
}

// NameMatch is one name a query matched.
type NameMatch struct {
	Name  string
	Tier  Tier
	score int // lower is better, within a tier
}

// Match returns only the BEST tier that produced any hit (case-insensitive),
// so an exact name never drags in names it is also a substring of, while a
// typo still falls through to the subsequence tier.
func Match(names []string, query string) (Tier, []NameMatch) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return TierNone, nil
	}
	best := TierNone
	var all []NameMatch
	for _, n := range names {
		t, s := scoreKey(strings.ToLower(n), q)
		if t == TierNone {
			continue
		}
		if t < best {
			best = t
		}
		all = append(all, NameMatch{Name: n, Tier: t, score: s})
	}
	var out []NameMatch
	for _, m := range all {
		if m.Tier == best {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].Name < out[j].Name
	})
	return best, out
}

func scoreKey(key, q string) (Tier, int) {
	if key == q {
		return TierExact, 0
	}
	if strings.HasPrefix(key, q) {
		return TierPrefix, len(key) - len(q)
	}
	if i := strings.Index(key, q); i >= 0 {
		return TierSubstring, i*100 + (len(key) - len(q))
	}
	if span, ok := subsequenceSpan(key, q); ok {
		return TierFuzzy, span
	}
	return TierNone, 0
}

// subsequenceSpan reports whether every rune of q appears in key in order, and
// how many key runes the match spans (tighter is better).
func subsequenceSpan(key, q string) (int, bool) {
	kr, qr := []rune(key), []rune(q)
	if len(qr) == 0 {
		return 0, false
	}
	first, last, qi := -1, -1, 0
	for i := 0; i < len(kr) && qi < len(qr); i++ {
		if kr[i] == qr[qi] {
			if first < 0 {
				first = i
			}
			last = i
			qi++
		}
	}
	if qi < len(qr) {
		return 0, false
	}
	return last - first + 1, true
}
