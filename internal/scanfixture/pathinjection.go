//go:build scanfixture

// This file is defective on purpose and is excluded from every build that does
// not name the tag above. Do not copy anything in it, and do not repair it: the
// defect is the fixture.
//
// What it is a fixture for: a value taken from a request reaching a file system
// path with nothing between the two. filepath.Join cleans a path but does not
// confine it, so a name of "../../etc/passwd" leaves the directory the handler
// meant to serve from. That is the shape this project has to be refused for,
// because a permission-aware retrieval service whose handler can be walked out
// of has no permission model at all.
//
// The near miss is deliberate. The handler looks careful: it joins against a
// fixed root, it checks the error, it sets a content type. Everything a reader
// scanning the diff would look for is present, and the defect is the one thing
// that is not there, which is why the analyser is the thing that has to catch
// it rather than the reader.

package scanfixture

import (
	"net/http"
	"os"
	"path/filepath"
)

// documentRoot is the directory this handler means to serve from, and the
// directory it does not stay inside.
const documentRoot = "/srv/kanzlei/documents"

// ServeDocument writes the named document to the response.
func ServeDocument(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "no document named", http.StatusBadRequest)
		return
	}

	body, err := os.ReadFile(filepath.Join(documentRoot, name))
	if err != nil {
		http.Error(w, "no such document", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := w.Write(body); err != nil {
		return
	}
}
