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

// PermissionIsNotAFlag opens READ-ONLY with a permission whose bits collide
// with the write-flag mask on darwin. Write intent is read from the flag
// argument and from nothing else, so this is silent — and it is silent on every
// platform, which is the point: judging the permission made the verdict a
// function of the machine the gate ran on.
func PermissionIsNotAFlag(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o666)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

// PermissionCollidingOnLinuxOnly is PermissionIsNotAFlag with the permission
// changed in one place, to the value that collides with the mask on linux
// rather than on darwin. One source, one verdict, both platforms.
func PermissionCollidingOnLinuxOnly(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0o700)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, 16)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

// PermissionOnAWriteOpen deviates from PermissionIsNotAFlag in exactly one
// place: the FLAG carries a write bit. The colliding permission is unchanged,
// so what reports here is the flag and nothing else.
func PermissionOnAWriteOpen(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// UnresolvedFlagIsSilent takes its flag from a parameter, which resolves to
// nothing. The documented scope limitation, pinned: the permission beside it
// carries write bits and does not stand in as evidence.
func UnresolvedFlagIsSilent(path string, flag int) error {
	f, err := os.OpenFile(path, flag, 0o666)
	if err != nil {
		return err
	}
	defer f.Close()
	return nil
}

// ParenthesisedFlag wraps the flag variable in parentheses, which the chase
// unwraps before resolving the name.
func ParenthesisedFlag(path string) error {
	flags := os.O_WRONLY | os.O_CREATE
	f, err := os.OpenFile(path, (flags), 0o644)
	if err != nil {
		return err
	}
	defer f.Close() // want `the Close error on f is discarded`
	_, err = f.WriteString("data")
	return err
}

// parseMode returns a name and a flag set, so a caller binds the flag at index
// 1 of a tuple whose right-hand side is a single call.
func parseMode(spec string) (string, int) { return spec, os.O_WRONLY | os.O_CREATE }

// TupleBoundFlagIsSilent binds the flag at a position the chase must not index
// into a shorter right-hand side. The flag resolves to nothing — a call's
// second result is beyond one level of assignment — so the open is silent, and
// the chase must reach that silence rather than panic on the way.
func TupleBoundFlagIsSilent(spec, path string) error {
	_, flags := parseMode(spec)
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("data")
	return err
}

// TupleDeclaredFlagIsSilent is the var-declaration twin of the same binding,
// and the same index.
func TupleDeclaredFlagIsSilent(spec, path string) error {
	var _, flags = parseMode(spec)
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("data")
	return err
}

// openSpec yields all three of os.OpenFile's arguments at once.
func openSpec(path string) (string, int, os.FileMode) { return path, os.O_WRONLY, 0o644 }

// TupleSpreadOpenIsSilent passes the whole argument list through as one call's
// results. There is no separate flag argument to read, so nothing resolves and
// the open is silent — the alternative being to read the ONE argument as a flag,
// which is how the permission came to be judged as one.
func TupleSpreadOpenIsSilent(path string) error {
	f, err := os.OpenFile(openSpec(path))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("data")
	return err
}

// TempFile pins os.CreateTemp, the third write opener the package doc names.
func TempFile(dir, pattern string) error {
	f, err := os.CreateTemp(dir, pattern)
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
