package build_test

import (
	"strings"
	"testing"

	"github.com/iderex/kanzlei/internal/build"
)

func TestNothingIsEverBlank(t *testing.T) {
	t.Parallel()

	// A blank field in a version block reads as a truncated line rather than as
	// an answer with a gap in it, and somebody then reports a defect against a
	// build nobody can identify. Under `go test` neither field is stamped and
	// no build information is embedded, so this is the unstamped path: what it
	// asserts is that the unstamped path still answers.
	for name, got := range map[string]string{
		"Version":   build.Version(),
		"Commit":    build.Commit(),
		"GoVersion": build.GoVersion(),
	} {
		if got == "" {
			t.Errorf("%s() is empty", name)
		}
	}
}

func TestStringIsThreeLabelledLines(t *testing.T) {
	t.Parallel()

	got := build.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("String() does not end in a newline: %q", got)
	}

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("String() has %d lines, want 3: %q", len(lines), got)
	}

	// Each line is one fact, and a script cuts it on the first space.
	for i, wantLabel := range []string{"kanzlei", "commit", "go"} {
		label, value, found := strings.Cut(lines[i], " ")
		if !found || value == "" {
			t.Errorf("line %d = %q, want a label and a value", i+1, lines[i])
			continue
		}
		if label != wantLabel {
			t.Errorf("line %d label = %q, want %q", i+1, label, wantLabel)
		}
	}
}
