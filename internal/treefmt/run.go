package treefmt

import "sort"

// A File is one path and the bytes it holds. The caller reads them, so the
// decision in this package can be put in front of fixtures that were never
// written to a disk.
type File struct {
	Name  string
	Bytes []byte
}

// A Result is what one file departs from and what it should hold instead.
//
// Formatted is byte-identical to the input wherever Findings is empty, and
// callers may rely on that direction: it is what makes the writing half of the
// tool and the checking half one decision rather than two that can disagree.
// The reverse does not hold, because some departures are reported and
// deliberately not repaired.
type Result struct {
	Name      string
	Formatted []byte
	Findings  []Finding
}

// Check applies the rule set to every file, in the order given, and returns one
// Result per file. Files with nothing to report are included, so a caller
// writing bytes back does not have to look up which ones were skipped.
func Check(set *Set, files []File) []Result {
	out := make([]Result, 0, len(files))
	for _, f := range files {
		formatted, findings := Format(f.Name, f.Bytes, set.RulesFor(f.Name))
		out = append(out, Result{Name: f.Name, Formatted: formatted, Findings: findings})
	}
	return out
}

// Findings collects every finding across a set of results, ordered by path and
// then by line, which is the order somebody fixing them works in.
func Findings(results []Result) []Finding {
	var all []Finding
	for _, r := range results {
		all = append(all, r.Findings...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].File != all[j].File {
			return all[i].File < all[j].File
		}
		return all[i].Line < all[j].Line
	})
	return all
}
