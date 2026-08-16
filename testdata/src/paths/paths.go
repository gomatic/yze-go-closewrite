// Package paths exercises the predicate that decides whether a discarded Close
// is judged: whether the analyzer can see that it is UNCONDITIONAL, not the
// keyword it is spelled with. The near-misses here deviate from their positive
// in exactly one place.
//
// The silent half is the load-bearing half, and every shape in it was
// constructed by an adversarial pass against a revision that tried to recognise
// the failing branch instead. Each is the same cleanup as the first, written a
// way that revision did not recognise and therefore reported.
package paths

import (
	"errors"
	"os"
)

// InlineOnSuccess throws the close error away unconditionally with a bare call.
// Nothing guards it, so nothing else will tell the caller.
func InlineOnSuccess(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, werr := f.WriteString("data"); werr != nil {
		return werr
	}
	f.Close() // want `the Close error on f is discarded`
	return nil
}

// BlankOnSuccess deviates from InlineOnSuccess in one place: the discard is
// spelled as a blank assignment. The same loss, so the same finding.
func BlankOnSuccess(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, werr := f.WriteString("data"); werr != nil {
		return werr
	}
	_ = f.Close() // want `the Close error on f is discarded`
	return nil
}

// DeferredOnSuccess deviates from InlineOnSuccess in one place: the discard is
// spelled with defer. Again the same loss and the same finding — which is the
// whole point, because a rule keyed on the keyword makes evasion one token
// cheaper than compliance.
func DeferredOnSuccess(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, werr := f.WriteString("data"); werr != nil {
		return werr
	}
	return nil
}

// CleanupInsideACheck is the shape the exemption exists for: the file is handed
// back to the caller on success, so its close is the caller's, and the discard
// runs only inside the branch that already failed.
func CleanupInsideACheck(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, werr := f.WriteString(data); werr != nil {
		_ = f.Close()
		return nil, werr
	}
	return f, nil
}

// CleanupOutsideTheCheck deviates from CleanupInsideACheck in exactly one
// place: the discard sits after the branch rather than inside it, so nothing
// guards it and the close error is the only thing left to report.
func CleanupOutsideTheCheck(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, werr := f.WriteString(data); werr != nil {
		return nil, werr
	}
	_ = f.Close() // want `the Close error on f is discarded`
	return f, nil
}

// CleanupBehindErrorsIs writes the same check with errors.Is. A revision that
// recognised `err != nil` and nothing else reported this one.
func CleanupBehindErrorsIs(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, werr := f.WriteString(data); errors.Is(werr, os.ErrPermission) {
		_ = f.Close()
		return nil, werr
	}
	return f, nil
}

// CleanupBehindACompoundCondition adds a second disjunct to the check, which is
// enough to make a syntactic error-check matcher miss it.
func CleanupBehindACompoundCondition(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if n, werr := f.WriteString(data); werr != nil || n != len(data) {
		_ = f.Close()
		return nil, werr
	}
	return f, nil
}

// CleanupInsideASwitch writes the same check as a switch case.
func CleanupInsideASwitch(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	_, werr := f.WriteString(data)
	switch {
	case werr != nil:
		_ = f.Close()
		return nil, werr
	}
	return f, nil
}

// CleanupBehindAGoto reaches the same cleanup through a label, where there is
// no condition at the discard at all.
func CleanupBehindAGoto(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	var werr error
	if _, werr = f.WriteString(data); werr != nil {
		goto fail
	}
	return f, nil
fail:
	_ = f.Close()
	return nil, werr
}

// CleanupBehindACollectedError checks a length rather than an error, which is
// how a function accumulating failures spells the same thing.
func CleanupBehindACollectedError(path, data string) (*os.File, []error) {
	var errs []error
	f, err := os.Create(path)
	if err != nil {
		return nil, []error{err}
	}
	if _, werr := f.WriteString(data); werr != nil {
		errs = append(errs, werr)
	}
	if len(errs) > 0 {
		_ = f.Close()
		return nil, errs
	}
	return f, nil
}

// CleanupInsideALoop closes inside a loop body, where whether the iteration is
// the failing one is not a property of the statement.
func CleanupInsideALoop(paths []string) error {
	f, err := os.Create(paths[0])
	if err != nil {
		return err
	}
	for range paths {
		f.Close()
	}
	return nil
}

// alwaysFailing is never nil, which is what makes the next function a forgery
// rather than a cleanup.
var alwaysFailing = errors.New("always")

// ForgedErrorCheckDoesNotSilence wraps an ordinary deferred close in a check
// that is always taken, over an error that is unrelated, never nil and never
// propagated. A revision that exempted the body of an error check silenced this
// — two lines, no change in behaviour, a disablement the rule invented — so a
// DEFER is judged wherever it sits.
func ForgedErrorCheckDoesNotSilence(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if alwaysFailing != nil {
		defer f.Close() // want `the Close error on f is discarded`
	}
	_, err = f.WriteString(data)
	return err
}

// TupleDiscardOnSuccess throws two closes away in one unconditional all-blank
// tuple assignment: both are members of the blank-assign family.
func TupleDiscardOnSuccess(p, q string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	g, err := os.Create(q)
	if err != nil {
		return err
	}
	_, _ = f.Close(), g.Close() // want `the Close error on f is discarded` `the Close error on g is discarded`
	return nil
}

// ReceiveIsNotADiscardedCall pins that a statement which is an expression but
// not a call discards nothing, while the file's own close still reports.
func ReceiveIsNotADiscardedCall(path string, ready <-chan struct{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	<-ready
	f.Close() // want `the Close error on f is discarded`
	return nil
}

// ReaderInlineDiscardIsSilent deviates from InlineOnSuccess in exactly one
// place: the open. Nothing was written, so nothing is lost at close.
func ReaderInlineDiscardIsSilent(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if _, rerr := f.Read(nil); rerr != nil {
		return rerr
	}
	f.Close()
	return nil
}

// RollbackInsideADeferIsSilent is the rollback idiom: the deferred closure
// closes only under a guard, which is cleanup on the failing path written in
// the one place it can run.
func RollbackInsideADeferIsSilent(path, data string) (f *os.File, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	if _, err = f.WriteString(data); err != nil {
		return nil, err
	}
	return f, nil
}

// RollbackWithoutTheGuard deviates from RollbackInsideADeferIsSilent in exactly
// one place: the deferred closure closes unconditionally, so it runs on every
// path and the close error is the last word on the file.
func RollbackWithoutTheGuard(path, data string) (f *os.File, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close() // want `the Close error on f is discarded`
	}()
	if _, err = f.WriteString(data); err != nil {
		return nil, err
	}
	return f, nil
}

// RollbackThroughAHelperClosureIsSilent puts the guarded close one closure
// deeper. A literal is its own function, so nothing in it belongs to the
// deferred closure's own statements.
func RollbackThroughAHelperClosureIsSilent(path, data string) (f *os.File, err error) {
	f, err = os.Create(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup := func() {
			if err != nil {
				f.Close()
			}
		}
		cleanup()
	}()
	if _, err = f.WriteString(data); err != nil {
		return nil, err
	}
	return f, nil
}

// DeferredReturnedCloseIsDiscarded pins the return shape inside a deferred
// closure: the defer runtime throws the value away, so returning the close is
// discarding it.
func DeferredReturnedCloseIsDiscarded(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() error { return f.Close() }() // want `the Close error on f is discarded`
	_, err = f.WriteString(data)
	return err
}
