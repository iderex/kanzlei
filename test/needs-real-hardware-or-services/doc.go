// Package needsreal holds the suites that cannot be proved without the real
// thing: a model runtime with weights loaded on real hardware, an identity
// provider issuing real tokens, a source system with real permissions on real
// files.
//
// Every file here except this one is behind the `needsreal` build constraint,
// so the default run compiles this package and finds no tests in it. README.md
// in this directory says what each suite needs and how to ask for it.
//
// This file carries no build constraint on purpose. A package whose files are
// all constrained out is not a package the default run can load, and the error
// it produces reads like a broken tree rather than like a suite nobody asked
// for.
package needsreal
