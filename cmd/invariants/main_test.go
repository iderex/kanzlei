package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree writes the files given into a temporary directory. No git checkout is
// needed here: this command walks the tree rather than reading an index,
// because an invariant about the source has to hold in a file that is there
// whether or not somebody remembered to add it.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

const rules = `invariant: no-route-outside-the-router
shape: call-only-in
names: Handle HandleFunc
paths: internal/server
reason: a route registered outside the routing table is an entry the table does not show
`

func TestATreeThatHoldsTheInvariantsPassesAndSaysWhatItRead(t *testing.T) {
	dir := tree(t, map[string]string{
		"invariants.txt": rules,
		"internal/server/server.go": `package server

import "net/http"

func Routes(mux *http.ServeMux) { mux.HandleFunc("GET /livez", nil) }
`,
	})

	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	for _, want := range []string{"1 invariant(s)", "1 Go file(s)", "no violations"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("the pass line does not say what it read, wanted %q: %q", want, out.String())
		}
	}
}

func TestAViolationIsPrintedWithItsPositionAndItsReasonAndTheRunIsRed(t *testing.T) {
	dir := tree(t, map[string]string{
		"invariants.txt": rules,
		"internal/api/routes.go": `package api

import "net/http"

func Register(mux *http.ServeMux) { mux.HandleFunc("GET /ask", nil) }
`,
	})

	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("want a red run, got no error")
	}
	for _, want := range []string{"internal/api/routes.go", ":5:", "no-route-outside-the-router", "the table does not show"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("wanted %q in the report: %q", want, out.String())
		}
	}
	if !strings.Contains(err.Error(), "1 violation(s)") {
		t.Fatalf("the summary does not count the violations: %v", err)
	}
}

func TestADeclarationFileThatIsNotThereIsAnError(t *testing.T) {
	dir := tree(t, map[string]string{"internal/server/server.go": "package server\n"})
	var out, errOut strings.Builder
	if err := run([]string{"-root", dir}, &out, &errOut); err == nil {
		t.Fatal("want a missing declaration file refused, got no error")
	}
}

func TestADeclarationFileNoOperatorCanReadIsAnError(t *testing.T) {
	dir := tree(t, map[string]string{
		"invariants.txt": "invariant: no-panic\nshape: no-panics\nreason: a panic in a handler is an outage\n",
	})
	var out, errOut strings.Builder
	err := run([]string{"-root", dir}, &out, &errOut)
	if err == nil {
		t.Fatal("want a shape no operator implements refused, got no error")
	}
	if !strings.Contains(err.Error(), "no-panics") {
		t.Fatalf("the refusal does not name the shape: %v", err)
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"extra"}, &out, &errOut); err == nil {
		t.Fatal("want an unexpected argument refused, got no error")
	}
}
