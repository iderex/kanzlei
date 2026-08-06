// Package build reports what this binary is and where it came from.
//
// Two questions have to be answerable from a running process before anything
// else is worth debugging: which version is this, and which commit was it built
// from. An operator who cannot answer them is reporting a defect against a tree
// nobody can identify, and every answer they get back is a guess about a
// different build.
package build

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// Unknown is what every field reports when nothing supplied it. It is a word
// rather than an empty string so that a version line reads as an answer with a
// gap in it rather than as a truncated line.
const Unknown = "unknown"

// version and commit are set at link time by the release build:
//
//	-ldflags "-X github.com/iderex/kanzlei/internal/build.version=v1.2.3"
//
// A plain `go build` sets neither. The commit is then recovered from the build
// information the toolchain embeds when it builds inside a repository, so the
// documented one-command build still identifies its own source; the version
// cannot be recovered that way, because a commit is a fact about the tree and a
// version is a decision somebody made about it.
var (
	version string
	commit  string
)

// Version reports the release version, or Unknown where this binary was not
// built by the release route.
func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// Set when the binary was installed from a module version rather than
		// built from a checkout. "(devel)" is what the toolchain writes for a
		// build from an unreleased tree, which is not a version.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Unknown
}

// Commit reports the revision this binary was built from, with a "-dirty"
// suffix where the tree had uncommitted changes at build time. A build whose
// source cannot be identified reports Unknown rather than a plausible-looking
// value.
func Commit() string {
	if commit != "" {
		return commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Unknown
	}
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision == "" {
		return Unknown
	}
	if modified == "true" {
		return revision + "-dirty"
	}
	return revision
}

// GoVersion reports the toolchain that produced this binary.
func GoVersion() string { return runtime.Version() }

// String is the version block, one fact per line, in a shape a person can read
// and a script can cut on the first space.
func String() string {
	return fmt.Sprintf("kanzlei %s\ncommit %s\ngo %s\n", Version(), Commit(), GoVersion())
}

// WriteTo writes the version block to w.
func WriteTo(w io.Writer) error {
	_, err := io.WriteString(w, String())
	return err
}
