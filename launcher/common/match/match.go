// Package match is a small, dependency-free fuzzy matcher for the launcher's picker: a
// case-insensitive subsequence test with a score that favours matches at the start of a
// name, at word boundaries (after - _ . / or space), and in contiguous runs, so the
// tightest, most obvious matches rank first. It is greedy (first-occurrence) rather than
// optimal - fine and fast for short app names.
package match

import (
	"sort"
	"strings"
)

// scoring weights
const (
	matchBonus      = 10 // each matched character
	startBonus      = 10 // the match is the first character of the target
	boundaryBonus   = 8  // the match follows a word boundary
	contiguousBonus = 5  // the match immediately follows the previous match
	gapPenalty      = 1  // each target character skipped before the next match
)

// Match reports whether query is a (case-insensitive) subsequence of target and, if so,
// a score where higher is a better match. An empty query matches everything with a
// neutral score, so the caller keeps its original ordering.
func Match(query, target string) (int, bool) {
	if query == "" {
		return 0, true
	}
	lowQuery := strings.ToLower(query)
	lowTarget := strings.ToLower(target)

	score := 0
	queryIdx := 0
	prevMatch := -2 // target index of the previous matched char (-2 so index 0 is never "contiguous")
	for targetIdx := 0; targetIdx < len(lowTarget) && queryIdx < len(lowQuery); targetIdx++ {
		if lowQuery[queryIdx] != lowTarget[targetIdx] {
			score -= gapPenalty
			continue
		}
		score += matchBonus
		switch {
		case targetIdx == 0:
			score += startBonus
		case isBoundary(lowTarget[targetIdx-1]):
			score += boundaryBonus
		}
		if targetIdx == prevMatch+1 {
			score += contiguousBonus
		}
		prevMatch = targetIdx
		queryIdx++
	}
	if queryIdx < len(lowQuery) {
		return 0, false // not every query character matched: not a subsequence
	}
	return score, true
}

// isBoundary reports whether byte is a separator that starts a new "word", so a match
// just after it reads as matching the start of that word.
func isBoundary(chr byte) bool {
	switch chr {
	case '-', '_', '.', '/', ' ':
		return true
	default:
		return false
	}
}

// Ranked is one matching target: its index in the input slice and its score.
type Ranked struct {
	Index int
	Score int
}

// Filter returns the targets that match query, ranked best-first. Ties keep the input
// order (so an empty query, or equally-good matches, stay in the caller's sort - usually
// alphabetical), which makes the picker stable as you type.
func Filter(query string, targets []string) []Ranked {
	var ranked []Ranked
	for index, target := range targets {
		if score, ok := Match(query, target); ok {
			ranked = append(ranked, Ranked{Index: index, Score: score})
		}
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].Score != ranked[right].Score {
			return ranked[left].Score > ranked[right].Score
		}
		return ranked[left].Index < ranked[right].Index
	})
	return ranked
}
