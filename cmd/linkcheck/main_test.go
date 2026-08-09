package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// checkout builds a git checkout in a temporary directory holding the files
// given and adds them to the index. Nothing is committed: git ls-files reads
// the index, and a commit would need an identity and a signature this case has
// no business arranging.
func checkout(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	git(t, dir, "add", "-A")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// answering starts a server on this machine that says what each path is. It is
// the only thing in this package's cases that a request is made against, and it
// is what lets the request itself be exercised without a route off the machine.
func answering(t *testing.T, status map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, ok := status[r.URL.Path]
		if !ok {
			code = http.StatusOK
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The arguments every case uses. One attempt and no pause, because the
// tolerance is proved in internal/linkcheck against an injected answer and
// waiting for it here would buy nothing.
func args(dir string) []string {
	return []string{"-root", dir, "-attempts", "1", "-pause", "0", "-timeout", "5s"}
}

func TestATreeWhoseLinksAnswerPassesAndSaysWhatItRead(t *testing.T) {
	srv := answering(t, nil)
	dir := checkout(t, map[string]string{
		"README.md":      "see [the page](" + srv.URL + "/one)\n",
		"docs/layout.md": "and [another](" + srv.URL + "/two)\n",
	})

	var out, errOut strings.Builder
	if err := run(args(dir), &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "2 external link(s) across 2 tracked document(s)") {
		t.Fatalf("the pass line does not say what it read: %q", out.String())
	}
	if !strings.Contains(out.String(), "2 reachable, 0 gone, 0 not judged") {
		t.Fatalf("the pass line does not say what it found: %q", out.String())
	}
}

// The bite the contributor guide asks to be shown by running rather than
// asserted: a document pointing at an address the other end says it does not
// have, refused with the document and the line.
func TestADocumentPointingAtAnAddressThatIsGoneIsRefused(t *testing.T) {
	srv := answering(t, map[string]int{"/gone": http.StatusNotFound})
	dir := checkout(t, map[string]string{
		"docs/layout.md": "first\nsee [the page](" + srv.URL + "/gone)\n",
	})

	var out, errOut strings.Builder
	err := run(args(dir), &out, &errOut)
	if err == nil {
		t.Fatal("a document pointing at an address that is not there passed")
	}
	if !strings.Contains(out.String(), "docs/layout.md:2") || !strings.Contains(out.String(), "is gone") {
		t.Fatalf("the finding does not carry the document and the line: %q", out.String())
	}
	if !strings.Contains(err.Error(), "1 link(s)") {
		t.Fatalf("the exit does not say how many: %v", err)
	}
}

// A server error is not a dead link, and the difference has to survive the
// route through this command rather than only holding inside the rule.
func TestAnAddressThatCouldNotBeJudgedIsPrintedAndIsNotARefusal(t *testing.T) {
	srv := answering(t, map[string]int{"/slow": http.StatusServiceUnavailable})
	dir := checkout(t, map[string]string{
		"docs/layout.md": "see [the page](" + srv.URL + "/slow)\n",
	})

	var out, errOut strings.Builder
	if err := run(args(dir), &out, &errOut); err != nil {
		t.Fatalf("a server error was treated as a dead link: %v", err)
	}
	if !strings.Contains(out.String(), "was not judged") {
		t.Fatalf("a link nothing could judge was passed over in silence: %q", out.String())
	}
	if !strings.Contains(out.String(), "0 reachable, 0 gone, 1 not judged") {
		t.Fatalf("the report does not count it: %q", out.String())
	}
}

// This repository's own documents carry no external link today. A run over a
// tree like that has to say it read the documents, or it cannot be told from a
// run that read none.
func TestATreeWithNoExternalLinkStillSaysWhatItRead(t *testing.T) {
	dir := checkout(t, map[string]string{
		"README.md": "prose naming `internal/authz` and nothing off this machine\n",
	})

	var out, errOut strings.Builder
	if err := run(args(dir), &out, &errOut); err != nil {
		t.Fatalf("run: %v (%s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "0 external link(s) across 1 tracked document(s)") {
		t.Fatalf("the pass line does not say what it read: %q", out.String())
	}
}

// An address this program cannot even build a request for is an answer about
// the address rather than a crash, and it is not a refusal, because nothing
// asked the other end anything.
func TestAnAddressThatIsNotARequestIsReportedRatherThanFatal(t *testing.T) {
	dir := checkout(t, map[string]string{
		// A percent escape that is not one. This never leaves the process,
		// which is what the case needs: the default run has no route out, so an
		// address that would be looked up is not a thing to write into a case.
		"docs/layout.md": "prose\nsee [the page](http://%zz/one)\n",
	})

	var out, errOut strings.Builder
	if err := run(args(dir), &out, &errOut); err != nil {
		t.Fatalf("an unbuildable address stopped the run: %v", err)
	}
	if !strings.Contains(out.String(), "docs/layout.md:2") || !strings.Contains(out.String(), "was not judged") {
		t.Fatalf("the address was not reported: %q", out.String())
	}
}

func TestATreeHoldingNoDocumentIsARefusalRatherThanAGreenRun(t *testing.T) {
	dir := checkout(t, map[string]string{"main.go": "package main\n"})

	var out, errOut strings.Builder
	err := run(args(dir), &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "no document") {
		t.Fatalf("a tree with no document did not refuse: %v", err)
	}
}

func TestSomewhereThatIsNotACheckoutIsRefusedWithGitsOwnWords(t *testing.T) {
	var out, errOut strings.Builder
	err := run(args(t.TempDir()), &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "git ls-files") {
		t.Fatalf("a directory that is not a checkout did not refuse: %v", err)
	}
}

func TestAnUnexpectedArgumentIsRefused(t *testing.T) {
	var out, errOut strings.Builder
	err := run([]string{"docs/layout.md"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("an argument this command does not take was accepted: %v", err)
	}
}

func TestAnUnknownFlagIsRefusedByTheFlagSet(t *testing.T) {
	var out, errOut strings.Builder
	if err := run([]string{"-nosuchflag"}, &out, &errOut); err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}

func TestADocumentGitHoldsAndTheWorkingTreeDoesNotIsRefused(t *testing.T) {
	dir := checkout(t, map[string]string{"README.md": "prose\n"})
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	var out, errOut strings.Builder
	err := run(args(dir), &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "read README.md") {
		t.Fatalf("a document that could not be read did not refuse: %v", err)
	}
}
