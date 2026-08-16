// Package bound exercises the binding test at the heart of the settlement
// rule: only a call whose result is bound can carry a close error somewhere,
// so only bound calls settle a file.
package bound

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

// FprintWrite writes through fmt and defers the close: the canonical target.
// The write call binds nothing here, so it settles nothing.
func FprintWrite(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	fmt.Fprintln(f, data)
	return nil
}

// PassedToNothing hands the file to a call that returns nothing and binds
// nothing: that proves no handling, and the deferred discard reports.
func PassedToNothing(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	observe(f)
	return nil
}

// observe takes the file and proves nothing about its close.
func observe(*os.File) {}

// BufferedWrite wraps the file and defers the close: NewWriter binds no close
// error, and the buffered writer makes the deferred loss worse, not handled.
func BufferedWrite(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	w := bufio.NewWriter(f)
	_, err = w.WriteString(data)
	return err
}

// InlineBlankClose discards the close twice — once inline, once deferred — and
// binds it nowhere: two discards are not handling.
func InlineBlankClose(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, err = f.WriteString(data); err != nil {
		_ = f.Close()
		return err
	}
	return nil
}

// VarDeclared opens through a var declaration: the binding form does not
// change what was opened.
func VarDeclared(path string) error {
	var f, err = os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// SafetyNet pairs the deferred discard with a bound close on the success
// path: the deferred close is a second close whose error is rightly ignored.
func SafetyNet(path, data string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err = f.WriteString(data); err != nil {
		return err
	}
	return f.Close()
}

// JoinedClose binds the close error through a nested call in return position.
func JoinedClose(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = f.WriteString(data)
	return errors.Join(err, f.Close())
}

// SeamedClose hands the file to a bound call, which is what the seamed-close
// repair looks like: the bound result is the evidence the close is handled.
func SeamedClose(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err = f.WriteString(data); err != nil {
		return err
	}
	return closeOutput(f)
}

// closeOutput is the seam a test replaces to force the close failure.
var closeOutput = func(f *os.File) error { return f.Close() }

// BoundWrite pins that a checked WRITE settles nothing. It differs from
// FprintWrite above in exactly one place — the write's error is bound — and
// that is not a fact about the close: whether the bytes reached the disk is
// decided at Close, which this function still throws away.
func BoundWrite(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, err := fmt.Fprintln(f, data); err != nil {
		return err
	}
	return nil
}

// CopiedInto is the analyzer's canonical target written the commonest way there
// is. io.Copy takes the file and hands back a COUNT beside its error, which is
// what separates a write from a close: a close has nothing to count.
func CopiedInto(path string, src io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, err := io.Copy(f, src); err != nil {
		return err
	}
	return nil
}

// describe takes the file and reports something that is not its close.
func describe(f *os.File) (string, error) { return f.Name(), nil }

// transfer takes the file and something else, and hands back nothing but an
// error: the shape a seam test keyed on the argument's position would refuse.
func transfer(dst *os.File, src io.Reader) error { _, err := io.Copy(dst, src); return err }

// SecondArgumentStillSettles deviates from SeamedClose in one place: the bound
// error-returning call takes a second argument. That changes nothing about what
// it hands back, and a close helper taking a logger or a context is ordinary —
// an earlier revision required the file to be the only argument and reported
// this, which is a finding no author can act on.
func SecondArgumentStillSettles(path string, src io.Reader) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("data"); err != nil {
		return err
	}
	return transfer(f, src)
}

// OneCallSettlesEveryFileHandedToIt pins the declared limit of the seam rule:
// no signature separates a helper that closes both files from one that closes
// neither, so both are settled and this is silent.
func OneCallSettlesEveryFileHandedToIt(p, q string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	g, err := os.Create(q)
	if err != nil {
		return err
	}
	defer g.Close()
	return closeBoth(f, g)
}

// closeBoth takes two files and hands back nothing but an error.
func closeBoth(a, b *os.File) error { return errors.Join(a.Close(), b.Close()) }

// OneArgumentWriteSettlesToo is the seam rule's declared hole, fixtured rather
// than hidden: a bound write taking the file as its only argument and returning
// nothing but an error is indistinguishable from a close, so it settles.
func OneArgumentWriteSettlesToo(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeAll(f)
}

// writeAll writes and hands back nothing but an error, closing nothing.
func writeAll(f *os.File) error { _, err := f.WriteString("data"); return err }

// ParenthesisedSeamArgumentStillSettles wraps the handed-over file in
// parentheses, which the seam unwraps before resolving the name.
func ParenthesisedSeamArgumentStillSettles(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(data); err != nil {
		return err
	}
	return closeOutput((f))
}

// TupleResultSeam pins the second half of the seam shape: one argument, but a
// second RESULT, so the call was asked for something besides the close.
func TupleResultSeam(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, err := describe(f); err != nil {
		return err
	}
	return nil
}

// LiteralArgumentSeam binds a one-argument error-returning call whose argument
// is not a name: no file was handed to it, so it settles nothing.
func LiteralArgumentSeam(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if failure := errors.New("boom"); failure != nil {
		return failure
	}
	return nil
}

// verdict is a plain struct: only *verdict implements error, so a verdict VALUE
// is not assignable to error and errors.Is cannot be called on it.
type verdict struct{ msg string }

// Error makes the POINTER an error, and the value not one.
func (v *verdict) Error() string { return v.msg }

// inspect returns a verdict by value and closes nothing.
func inspect(f *os.File) verdict { return verdict{msg: f.Name()} }

// PointerOnlyErrorSeam pins that a result whose POINTER implements error is not
// an error: the caller holds a value carrying nothing about the close.
func PointerOnlyErrorSeam(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	if _, err := f.WriteString(data); err != nil {
		return err
	}
	result := inspect(f)
	_ = result
	return nil
}

// ClosureAside binds a closure that closes the file INSIDE its own body: a
// literal is its own function, so nothing in it settles this function's file
// and the deferred discard still reports.
func ClosureAside(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	closer := func() error { return f.Close() }
	_ = closer
	_, err = f.WriteString("data")
	return err
}

// notify observes the file and reports its own error, which the defer runtime
// throws away below.
func notify(*os.File) error { return nil }

// DeferredReturnBindsNothing pins the deferred-return rule: a return inside a
// deferred closure reaches no caller, so it binds nothing and settles nothing.
func DeferredReturnBindsNothing(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	defer func() error { return notify(f) }()
	_, err = f.WriteString("data")
	return err
}

// NamedReturnClose pins the sanctioned named-return closure: the assignment
// inside the deferred closure binds the close error to the function's own
// result, which IS this function's flow.
func NamedReturnClose(path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = f.Close() }()
	_, err = f.WriteString("data")
	return err
}

// TupleBlankDiscard discards two closes in one all-blank tuple assignment:
// both are members of the blank-assign family and both report.
func TupleBlankDiscard(p, q string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	g, err := os.Create(q)
	if err != nil {
		return err
	}
	defer func() { _, _ = f.Close(), g.Close() }() // want `the Close error on f is discarded` `the Close error on g is discarded`
	_, err = f.WriteString("data")
	if err != nil {
		return err
	}
	_, err = g.WriteString("data")
	return err
}

// pathErrClose is a seam whose result is a concrete error type: it carries the
// close error as surely as one returning error.
var pathErrClose = func(f *os.File) *os.PathError {
	if err := f.Close(); err != nil {
		return &os.PathError{Op: "close", Path: f.Name(), Err: err}
	}
	return nil
}

// ConcreteErrorSeam settles through the concretely-typed seam: the safety-net
// defer is a second close whose error is rightly ignored.
func ConcreteErrorSeam(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(data); err != nil {
		return err
	}
	if perr := pathErrClose(f); perr != nil {
		return perr
	}
	return nil
}
