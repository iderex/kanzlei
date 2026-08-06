package treefmt

import "testing"

func TestCheckReturnsOneResultPerFileIncludingTheCleanOnes(t *testing.T) {
	// A caller writing bytes back walks the results. Leaving the clean files
	// out would make the caller look up which ones were skipped, which is the
	// same decision taken twice in two places.
	set := mustParse(t, "root = true\n\n[*]\ntrim_trailing_whitespace = true\ninsert_final_newline = true\n")
	results := Check(set, []File{
		{Name: "clean.md", Bytes: []byte("one\n")},
		{Name: "dirty.md", Bytes: []byte("two  \n")},
	})

	if len(results) != 2 {
		t.Fatalf("want a result per file, got %d", len(results))
	}
	if results[0].Name != "clean.md" || len(results[0].Findings) != 0 {
		t.Errorf("the clean file came back as %+v", results[0])
	}
	if string(results[1].Formatted) != "two\n" {
		t.Errorf("the dirty file came back as %q", results[1].Formatted)
	}
}

func TestFindingsAreOrderedByPathAndThenByLine(t *testing.T) {
	// The order somebody fixing them works in. Results come back in the order
	// the files were listed, which is git's order and not a reader's.
	set := mustParse(t, "root = true\n\n[*]\ntrim_trailing_whitespace = true\n")
	results := Check(set, []File{
		{Name: "z.md", Bytes: []byte("a \n")},
		{Name: "a.md", Bytes: []byte("a\nb \nc  \n")},
	})

	all := Findings(results)
	if len(all) != 3 {
		t.Fatalf("want 3 findings, got %v", all)
	}
	if all[0].File != "a.md" || all[0].Line != 2 {
		t.Errorf("first is %v", all[0])
	}
	if all[1].File != "a.md" || all[1].Line != 3 {
		t.Errorf("second is %v", all[1])
	}
	if all[2].File != "z.md" {
		t.Errorf("third is %v", all[2])
	}
}

func TestFindingsOverACleanTreeIsEmptyRatherThanNil(t *testing.T) {
	// The caller counts it. An empty list and no list have to read the same way
	// at the one place that decides the exit status.
	set := mustParse(t, "root = true\n\n[*]\ntrim_trailing_whitespace = true\n")
	if got := Findings(Check(set, []File{{Name: "a.md", Bytes: []byte("a\n")}})); len(got) != 0 {
		t.Fatalf("a clean tree reported %v", got)
	}
}
