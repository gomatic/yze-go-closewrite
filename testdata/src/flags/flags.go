// Package flags exercises every spelling of a write-implying open flag, and
// each write flag individually, so no single flag can fall out of the set
// unnoticed.
package flags

import "os"

// mode is a folded constant carrying write bits without a literal selector in
// the argument list.
const mode = os.O_WRONLY | os.O_CREATE

// ConstMode opens through the named constant.
func ConstMode(path string) error {
	f, err := os.OpenFile(path, mode, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// VarMode opens through a local variable assigned from flag constants.
func VarMode(path string) error {
	flags := os.O_WRONLY | os.O_APPEND
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// WronlyOnly pins O_WRONLY alone.
func WronlyOnly(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// RdwrOnly pins O_RDWR alone.
func RdwrOnly(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// AppendOnly pins O_APPEND alone.
func AppendOnly(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// TruncOnly pins O_TRUNC alone.
func TruncOnly(path string) error {
	f, err := os.OpenFile(path, os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// CreateOnly pins the documented O_CREATE decision: creating a file and
// discarding its Close error reports success over a directory entry that may
// not exist, even when the descriptor is otherwise read-only.
func CreateOnly(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close() // want `the Close error on f is discarded`
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

// ReadOnly stays silent: no write flag, nothing lost at close.
func ReadOnly(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

// DeclaredVarMode opens through a flag variable bound by a var declaration,
// the second binding form the chase must see.
func DeclaredVarMode(path string) error {
	var flags = os.O_RDWR | os.O_TRUNC
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// ComputedMode mixes a constant write flag with a runtime value: the whole
// expression no longer folds to a constant, and the literal selector is the
// remaining evidence.
func ComputedMode(path string, extra int) error {
	mode := os.O_WRONLY | extra
	f, err := os.OpenFile(path, mode, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// ReassignedAfterOpen pins the flow rule: what the flag variable becomes AFTER
// the open opened nothing. The reader below was opened read-only and stays
// silent although the same variable later carries a write flag.
func ReassignedAfterOpen(p, q string) error {
	flags := os.O_RDONLY
	r, err := os.OpenFile(p, flags, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	flags = os.O_WRONLY
	w, err := os.OpenFile(q, flags, 0o644)
	if err != nil {
		return err
	}
	defer w.Close() // want `the Close error on w is discarded`
	_, err = w.WriteString("data")
	return err
}
