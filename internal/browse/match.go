package browse

import (
	"path"
	"strings"
	"unicode"
)

// A query is what is typed to narrow a folder's names: usually letters that
// must appear in order, ranked by how well they fit; or a glob such as
// "*.png" when it has a wildcard.
type query struct {
	raw   string
	runes []rune
	exact bool // Case counts: the query has a capital.
	glob  bool
}

func parseQuery(s string) query {
	q := query{raw: s, runes: []rune(s)}
	for _, r := range q.runes {
		if unicode.IsUpper(r) {
			q.exact = true
		}
	}
	if strings.ContainsAny(s, "*?[") {
		_, err := path.Match(s, "")
		q.glob = err == nil
	}
	return q
}

func (q query) empty() bool { return len(q.runes) == 0 }

// Scores: each letter found earns a base; the rest favor the ways people
// mean a name, the start of it, the start of a word, a run of letters.
const (
	scoreMatch       = 16
	bonusConsecutive = 16
	bonusFirst       = 14
	bonusBoundary    = 8
	bonusCamel       = 6
	penaltyGap       = 2
	penaltyLead      = 1
	maxLead          = 6
	bonusPrefix      = 30
	bonusWhole       = 30
	unreached        = -1 << 30
)

// match says whether the name fits the query, how well (higher is better),
// and which of the name's letters (by index among its runes) were matched,
// for drawing them.
func (q query) match(name string) (score int, at []int, ok bool) {
	if q.empty() {
		return 0, nil, true
	}
	if q.glob {
		pattern, subject := q.raw, name
		if !q.exact {
			pattern, subject = strings.ToLower(pattern), strings.ToLower(subject)
		}
		if m, err := path.Match(pattern, subject); err == nil && m {
			return 100 - len(name)/4, nil, true
		}
		return 0, nil, false
	}
	n := []rune(name)
	m := len(q.runes)
	if len(n) < m {
		return 0, nil, false
	}
	fold := func(r rune) rune {
		if q.exact {
			return r
		}
		return unicode.ToLower(r)
	}
	// A quick look: do the letters appear in order at all?
	i := 0
	for _, r := range n {
		if i < m && fold(r) == q.runes[i] {
			i++
		}
	}
	if i < m {
		return 0, nil, false
	}

	// Then the best way to lay them down, by dynamic programming over where
	// each query letter lands.
	width := len(n)
	best := make([]int, m*width)
	from := make([]int, m*width)
	for k := range best {
		best[k], from[k] = unreached, -1
	}
	bonus := func(j int) int {
		if j == 0 {
			return bonusFirst
		}
		prev, cur := n[j-1], n[j]
		switch {
		case strings.ContainsRune(" -_./()[]", prev) && unicode.IsLetter(cur) || strings.ContainsRune(" -_./()[]", prev) && unicode.IsDigit(cur):
			return bonusBoundary
		case unicode.IsLower(prev) && unicode.IsUpper(cur):
			return bonusCamel
		}
		return 0
	}
	for r := 0; r < m; r++ {
		run, runAt := unreached, -1
		for j := r; j < width; j++ {
			// run is the best way to have laid down letters before r with
			// the last one at least two places back, less the gap.
			if r > 0 && j >= 2 {
				prev := best[(r-1)*width+j-2]
				if prev > unreached {
					if run > unreached {
						run -= penaltyGap
					}
					if prev-penaltyGap > run {
						run, runAt = prev-penaltyGap, j-2
					}
				} else if run > unreached {
					run -= penaltyGap
				}
			}
			if fold(n[j]) != q.runes[r] {
				continue
			}
			if r == 0 {
				best[j] = scoreMatch + bonus(j) - min(j*penaltyLead, maxLead)
				continue
			}
			score, origin := unreached, -1
			if c := best[(r-1)*width+j-1]; j > 0 && c > unreached {
				score, origin = c+scoreMatch+bonusConsecutive, j-1
			}
			if run > unreached {
				if g := run + scoreMatch + bonus(j); g > score {
					score, origin = g, runAt
				}
			}
			best[r*width+j], from[r*width+j] = score, origin
		}
	}
	end, top := -1, unreached
	for j := m - 1; j < width; j++ {
		if s := best[(m-1)*width+j]; s > top {
			end, top = j, s
		}
	}
	if end < 0 {
		return 0, nil, false
	}
	at = make([]int, m)
	for r, j := m-1, end; r >= 0; r-- {
		at[r] = j
		j = from[r*width+j]
	}
	score = top - (width-m)/4
	if at[0] == 0 && at[m-1] == m-1 {
		score += bonusPrefix
		if width == m {
			score += bonusWhole
		}
	}
	return score, at, true
}
