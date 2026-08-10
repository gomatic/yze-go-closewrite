// Package bound exercises the binding test at the heart of the settlement
// rule: only a call whose result is bound can carry a close error somewhere,
// so only bound calls settle a file.
package bound

import (
	"bufio"
	"errors"
	"fmt"
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

// BoundWrite pins the documented heuristic silence: a bound write taking the
// file settles it even though a checked write is not a handled close. The
// trade is deliberate — see closedWithHandling — and this fixture keeps it a
// choice rather than drift.
func BoundWrite(path, data string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, data); err != nil {
		return err
	}
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
