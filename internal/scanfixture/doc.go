// Package scanfixture holds one deliberately defective file, kept out of every
// build by a constraint, whose only purpose is to make the code scanning gate
// go red on demand.
//
// A gate that has never refused anything is a gate nobody has proved. The
// coverage floor and the suppression check are both proved by fixtures the
// suite runs; code scanning cannot be, because the analyser runs on the hosting
// service and there is nothing in this tree for a test to hand it. So the proof
// is a run: the workflow takes an input that adds the build tag below, the
// analyser then sees the defective file, and the run is expected to be refused.
// A run of that shape is what stands behind the claim that the gate bites, and
// the pull request that added the gate carries the result of one.
//
// The file beside this one is excluded from the shipped build by
// `//go:build scanfixture`, and nothing in this module imports this package.
// This file carries no constraint so that the directory is still a package the
// toolchain can see, which is the same shape test/needs-real-hardware-or-services
// uses for the same reason.
package scanfixture
