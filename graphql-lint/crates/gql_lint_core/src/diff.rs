//! Ports `internal/graphql/lint/diff.go`'s `New`: compares two
//! [`Violation`] sets for `--diff` mode, reporting only violations that
//! are new. Pure logic, no git dependency at all — `gql_lint_cli`'s own
//! `gitdiff` module is what actually reads a merge-base commit's file
//! content; this module only ever compares two `Vec<Violation>`s.

use crate::Violation;
use std::collections::HashMap;

/// Identifies a violation independent of its position, matching Go's
/// `violationKey`. Deliberately **not** a derived `Hash`/`Eq` on
/// [`Violation`] itself: `Violation` carries `line`/`column`, which must
/// never participate in this comparison (an unrelated line shift
/// elsewhere in a file must not manufacture a false "new" violation), so
/// giving `Violation` a real `Hash`/`Eq` impl would be actively wrong and
/// a trap for future code that assumes it means full equality.
type ViolationKey<'a> = (&'a str, &'a str, &'a str);

fn key_of(v: &Violation) -> ViolationKey<'_> {
    (&v.file, v.rule, &v.message)
}

/// Returns the violations in `current` that are not also present in
/// `baseline`, comparing by file, rule, and message but ignoring line and
/// column. `baseline` and `current` are typically the same files linted
/// at the merge-base and working-tree commits, respectively.
///
/// A message appearing more times in `current` than in `baseline` reports
/// only the excess occurrences, so a genuinely repeated violation isn't
/// masked by a single matching baseline occurrence.
#[must_use]
pub fn new(baseline: &[Violation], current: &[Violation]) -> Vec<Violation> {
    let mut remaining: HashMap<ViolationKey<'_>, usize> = HashMap::with_capacity(baseline.len());
    for v in baseline {
        *remaining.entry(key_of(v)).or_insert(0) += 1;
    }

    current
        .iter()
        .filter(|v| {
            let count = remaining.entry(key_of(v)).or_insert(0);
            if *count > 0 {
                *count -= 1;
                false
            } else {
                true
            }
        })
        .cloned()
        .collect()
}

#[cfg(test)]
mod tests {
    use super::new;
    use crate::Violation;

    fn v(file: &str, rule: &'static str, message: &str, line: usize) -> Violation {
        Violation {
            file: file.to_string(),
            line,
            column: 1,
            message: message.to_string(),
            rule,
        }
    }

    #[test]
    fn drops_a_violation_unchanged_since_the_baseline() {
        let baseline = vec![v("a.graphql", "alphabetize", "msg", 3)];
        let current = vec![v("a.graphql", "alphabetize", "msg", 3)];
        assert!(new(&baseline, &current).is_empty());
    }

    #[test]
    fn keeps_a_violation_whose_position_shifted_but_content_did_not() {
        let baseline = vec![v("a.graphql", "alphabetize", "msg", 3)];
        let current = vec![v("a.graphql", "alphabetize", "msg", 4)];
        assert!(new(&baseline, &current).is_empty());
    }

    #[test]
    fn keeps_a_violation_absent_from_the_baseline() {
        let baseline = vec![v("a.graphql", "alphabetize", "msg", 3)];
        let current = vec![
            v("a.graphql", "alphabetize", "msg", 3),
            v("a.graphql", "other-rule", "msg", 3),
        ];
        let diff = new(&baseline, &current);
        assert_eq!(diff.len(), 1);
        assert_eq!(diff[0].rule, "other-rule");
    }

    #[test]
    fn keeps_only_the_excess_occurrences_of_a_repeated_violation() {
        let baseline = vec![v("a.graphql", "alphabetize", "msg", 3)];
        let current = vec![
            v("a.graphql", "alphabetize", "msg", 3),
            v("a.graphql", "alphabetize", "msg", 3),
        ];
        assert_eq!(new(&baseline, &current).len(), 1);
    }
}
