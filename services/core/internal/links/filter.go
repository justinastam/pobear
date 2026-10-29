package links

import "github.com/bmatcuk/doublestar/v4"

// MatchesIncludeExclude reports whether candidateURL survives an include/exclude
// glob pair (JOB_SPEC_REFERENCE.md §9): any exclude match rejects it, and a
// non-empty include list must then match at least once.
func MatchesIncludeExclude(candidateURL string, include, exclude []string) bool {
	for _, pattern := range exclude {
		if ok, _ := doublestar.Match(pattern, candidateURL); ok {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if ok, _ := doublestar.Match(pattern, candidateURL); ok {
			return true
		}
	}
	return false
}
