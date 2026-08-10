// Package closures pins the one-visit-per-function property: an open and its
// deferred discard written inside a function literal belong to the literal's
// own visit, and are reported exactly once — analysistest fails on an extra
// diagnostic, so a double report cannot pass this fixture.
package closures

import "os"

// Runner returns a closure that opens, writes, and discards its close: one
// finding, at the closure's own defer.
func Runner(path string) func() error {
	return func() error {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close() // want `the Close error on f is discarded`
		_, err = f.WriteString("data")
		return err
	}
}

// Outer opens at function level and settles it on the success path; the
// nested closure's own file is the closure's business.
func Outer(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err = f.WriteString("outer"); err != nil {
		return err
	}
	return f.Close()
}

// DeferredWorkspace opens, writes, and discards entirely inside a DEFERRED
// closure: the enclosing function's walk owns that flow, and the finding is
// reported exactly once — an extra diagnostic fails this fixture.
func DeferredWorkspace(path string) {
	defer func() {
		f, err := os.Create(path)
		if err != nil {
			return
		}
		defer f.Close() // want `the Close error on f is discarded`
		_, _ = f.WriteString("state")
	}()
}
