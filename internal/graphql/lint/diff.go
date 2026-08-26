// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Compares two Violation sets for --diff mode, reporting
// only violations that are new.

package lint

// New returns the violations in current that are not also present in
// baseline, comparing by file, rule, and message but ignoring line and
// column. Ignoring position keeps the result stable across unrelated
// line shifts elsewhere in a file; a violation is "new" only when its
// file/rule/message combination didn't already exist at the baseline,
// not merely when it moved. baseline and current are typically the
// same files linted at the merge-base and working-tree commits,
// respectively.
//
// A message appearing more times in current than in baseline reports
// only the excess occurrences, so a genuinely repeated violation isn't
// masked by a single matching baseline occurrence.
func New(baseline, current []Violation) []Violation {
	remaining := make(map[violationKey]int, len(baseline))
	for _, v := range baseline {
		remaining[keyOf(v)]++
	}

	var newViolations []Violation
	for _, v := range current {
		k := keyOf(v)
		if remaining[k] > 0 {
			remaining[k]--
			continue
		}
		newViolations = append(newViolations, v)
	}
	return newViolations
}

// violationKey identifies a violation independent of its position,
// for New's baseline/current comparison.
type violationKey struct {
	file, rule, message string
}

func keyOf(v Violation) violationKey {
	return violationKey{file: v.File(), rule: v.Rule, message: v.Message()}
}
