module github.com/iderex/kanzlei

// The toolchain is pinned here rather than inherited from whatever the machine
// happens to have. This line is the authority for the version: the gate reads
// it instead of naming one of its own, so a version that is green here cannot
// be red elsewhere for a reason nobody can see. docs/decisions/0001-means.md
// argues Go and names 1.26 as the floor; this is the exact pin above it.
go 1.26.5

// No requirements. Everything this module uses is in the standard library, so
// there is no go.sum: a lock file with nothing to lock is not written by the
// toolchain and would not be read by it. When the first dependency lands, the
// requirement appears above and go.sum appears beside this file.
