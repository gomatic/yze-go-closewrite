// Package a exercises closewrite in both directions.
package a

import "os"

// DeferredCreate discards the Close error on a created file.
func DeferredCreate(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// BlankAssigned closes inline on a path already returning: cleanup, not loss.
func BlankAssigned(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = f.WriteString("data")
	_ = f.Close()
	return err
}

// BareCall closes inline; nothing deferred runs on the success path.
func BareCall(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

// ReaderIsSilent closes a file opened only for reading: nothing can be lost.
func ReaderIsSilent(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

// ReadOnlyOpenFileIsSilent opens with no write flag.
func ReadOnlyOpenFileIsSilent(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

// CheckedIsSilent binds the error and acts on it.
func CheckedIsSilent(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if closeErr := f.Close(); closeErr != nil {
		return closeErr
	}
	return nil
}

// NamedReturnClosureIsSilent is the CORRECT deferred form; flagging it would
// push authors away from the very fix the rule asks for.
func NamedReturnClosureIsSilent(path string) (err error) {
	f, createErr := os.Create(path)
	if createErr != nil {
		return createErr
	}
	defer func() { err = f.Close() }()
	return nil
}

// DeferredClosureDiscard is the closure form of the same defect.
func DeferredClosureDiscard(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// SafetyNetWithExplicitCloseIsSilent is the CORRECT idiom: the deferred close
// guards the early returns while the success path returns the close error.
// Flagging it would ask the author to handle an error handled a line below.
func SafetyNetWithExplicitCloseIsSilent(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("data"); err != nil {
		return err
	}
	return f.Close()
}

// holder puts a file behind a field, so the Close receiver is not a plain
// identifier and the open's target is not one either.
type holder struct{ f *os.File }

// FieldOpenAndCloseIsSilent exercises the non-identifier paths in both the
// open and the close: neither can be attributed to a local variable.
func FieldOpenAndCloseIsSilent(path string) error {
	var h holder
	var err error
	h.f, err = os.Create(path)
	if err != nil {
		return err
	}
	defer h.f.Close()
	return nil
}

// DeferredNonCloseIsSilent defers a method that is not Close.
func DeferredNonCloseIsSilent(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Sync()
	return f.Close()
}

// MultiValueAssignIsNotAnOpen has two right-hand expressions, so it is not the
// single-call assignment an open takes.
func MultiValueAssignIsNotAnOpen(path string) error {
	f, g := os.Stdout, os.Stderr
	_, _ = f, g
	return nil
}

// ShadowedPackageNameIsSilent calls Create on something that is not package os.
func ShadowedPackageNameIsSilent() error {
	os := fakeFS{}
	f := os.Create()
	defer f.Close()
	return nil
}

// fakeFS mimics the os surface closely enough to prove the analyzer resolves
// packages through the type info rather than the identifier's spelling.
type fakeFS struct{}

// Create returns a closer that has nothing to do with the os package.
func (fakeFS) Create() closer { return closer{} }

type closer struct{}

// Close satisfies the shape without being a file.
func (closer) Close() error { return nil }

// closeVia is a seam standing in for f.Close(), the sanctioned way to make a
// close failure reachable from a test.
var closeVia = func(f *os.File) error { return f.Close() }

// SeamedCloseIsSilent hands the file to a seam whose error is returned. This is
// what a FIXED site looks like, and an earlier version of this rule reported it
// — flagging the very shape its own findings are repaired into.
func SeamedCloseIsSilent(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("data"); err != nil {
		return err
	}
	return closeVia(f)
}
