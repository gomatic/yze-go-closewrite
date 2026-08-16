// Package paths exercises the predicate that decides whether a discarded Close
// is reported: the PATH it sits on, not the keyword it is spelled with. The
// near-misses here deviate from their positive in exactly one place.
package paths

import "os"

// InlineOnSuccess throws the close error away on the success path with a bare
// call. Nothing is failing, so nothing else will tell the caller.
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

// CleanupOnFailingPath is the shape the exemption exists for: the file is
// handed back to the caller on success, so its close is the caller's, and the
// discard runs only where the write already failed and the caller is about to
// be told.
func CleanupOnFailingPath(path, data string) (*os.File, error) {
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

// CleanupOnSuccessPath deviates from CleanupOnFailingPath in exactly one place:
// the discard sits AFTER the check rather than inside it, so it runs when the
// write succeeded and the close error is the only thing left to report.
func CleanupOnSuccessPath(path, data string) (*os.File, error) {
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

// NilCheckElseIsTheFailingBranch writes the check the other way round. The ELSE
// of `if err == nil` is the already-failing path and its discard is silent; the
// body is the success path and its discard reports.
func NilCheckElseIsTheFailingBranch(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if _, werr := f.WriteString(data); werr == nil {
		f.Close() // want `the Close error on f is discarded`
		return f, nil
	} else {
		_ = f.Close()
		return nil, werr
	}
}

// NilCheckWithoutElse deviates from NilCheckElseIsTheFailingBranch in one
// place: there is no else, so the check opens no failing span at all.
func NilCheckWithoutElse(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	_, werr := f.WriteString(data)
	if werr == nil {
		f.Close() // want `the Close error on f is discarded`
	}
	return f, werr
}

// NonErrorConditionOpensNoFailingPath guards the discard behind a condition
// establishing nothing about an error. A count is not a failure.
func NonErrorConditionOpensNoFailingPath(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	n, werr := f.WriteString(data)
	if n > 0 {
		f.Close() // want `the Close error on f is discarded`
	}
	return f, werr
}

// NonComparisonConditionOpensNoFailingPath guards it behind a bare boolean,
// which is not a comparison at all.
func NonComparisonConditionOpensNoFailingPath(path string, ready bool) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if ready {
		f.Close() // want `the Close error on f is discarded`
	}
	return f, nil
}

// ComparedToSomethingOtherThanNil compares the error against a sentinel rather
// than against nil, which does not establish that this path is failing.
func ComparedToSomethingOtherThanNil(path, data string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	_, werr := f.WriteString(data)
	if werr != os.ErrClosed {
		f.Close() // want `the Close error on f is discarded`
	}
	return f, werr
}

// NonErrorComparedToNil compares a NON-error against nil: a nil check on
// something that is not a failure opens no failing path.
func NonErrorComparedToNil(path string, buf []byte) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if buf != nil {
		f.Close() // want `the Close error on f is discarded`
	}
	return f, nil
}

// TupleDiscardOnSuccess throws two closes away in one all-blank tuple
// assignment on the success path: both are members of the blank-assign family.
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

// RollbackCloseInsideADeferIsSilent is the rollback idiom: the deferred closure
// closes only when the NAMED RESULT carries a failure, which is the same
// cleanup as CleanupOnFailingPath written in the one place it can run. The
// closure runs on every path; its guarded body does not.
func RollbackCloseInsideADeferIsSilent(path, data string) (f *os.File, err error) {
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

// RollbackCloseWithoutTheCheck deviates from RollbackCloseInsideADeferIsSilent
// in exactly one place: the deferred closure closes unconditionally. It
// therefore runs on the success path too, where the close error is the last
// word on the file.
func RollbackCloseWithoutTheCheck(path, data string) (f *os.File, err error) {
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
