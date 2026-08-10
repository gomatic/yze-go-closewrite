package closewrite

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

const closeMethod = "Close"

// importPath is a package's import path, e.g. "os". Resolution goes through the
// type checker, so an aliased import is still recognized and a local variable
// spelled os is not mistaken for the package.
type importPath string

// writeOpeners are the os functions whose result is a file the caller intends
// to WRITE. os.Open is deliberately absent: a reader's unclosed error loses
// nothing, and reporting it would bury the real findings under the single most
// common line in Go.
var writeOpeners = map[string]bool{
	"Create":     true,
	"CreateTemp": true,
	"OpenFile":   false, // conditional: only with a write flag, see opensForWrite
}

// writeFlags are the os open flags that establish an intent to write. O_CREATE
// is here deliberately even though it can accompany O_RDONLY: creating a file
// and discarding its Close error still reports success over an entry that may
// not have reached the directory, and the createonly fixture pins that
// reading.
var writeFlags = map[string]bool{
	"O_WRONLY": true,
	"O_RDWR":   true,
	"O_CREATE": true,
	"O_APPEND": true,
	"O_TRUNC":  true,
}

// writeOpened collects the variables in body assigned from a write-opening
// call, keyed by the object so shadowing in a nested scope cannot be confused
// with the outer file. Both binding forms count — `f, err := os.Create(p)`
// and `var f, err = os.Create(p)` open the same file — and the walk stays
// within this function's own flow, so a literal's opens belong to the
// literal's own visit.
func writeOpened(info *types.Info, body *ast.BlockStmt) map[types.Object]bool {
	opened := map[types.Object]bool{}
	walkOwn(body, func(node ast.Node, _ deferredness) {
		switch at := node.(type) {
		case *ast.AssignStmt:
			collectOpened(info, body, at.Lhs, at.Rhs, opened)
		case *ast.ValueSpec:
			collectOpenedSpec(info, body, at, opened)
		}
	})
	return opened
}

// collectOpenedSpec records a var declaration's first name when its value
// opens a file for writing.
func collectOpenedSpec(info *types.Info, body *ast.BlockStmt, spec *ast.ValueSpec, opened map[types.Object]bool) {
	values := make([]ast.Expr, len(spec.Values))
	copy(values, spec.Values)
	names := make([]ast.Expr, len(spec.Names))
	for i, name := range spec.Names {
		names[i] = name
	}
	collectOpened(info, body, names, values, opened)
}

// collectOpened records the binding's first name when its right-hand side
// opens a file for writing.
func collectOpened(info *types.Info, body *ast.BlockStmt, lhs, rhs []ast.Expr, opened map[types.Object]bool) {
	if len(rhs) != 1 || len(lhs) == 0 {
		return
	}
	call, ok := rhs[0].(*ast.CallExpr)
	if !ok || !opensForWrite(info, body, call) {
		return
	}
	name, ok := lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	if object := info.ObjectOf(name); object != nil {
		opened[object] = true
	}
}

// opensForWrite reports whether call is an os open establishing write intent.
func opensForWrite(info *types.Info, body *ast.BlockStmt, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isOSPackage(info, selector.X) {
		return false
	}
	if writeOpeners[selector.Sel.Name] {
		return true
	}
	return selector.Sel.Name == "OpenFile" && hasWriteFlag(info, body, osImported(info, selector.X), call)
}

// osImported resolves the imported os package the selector's qualifier names.
// opensForWrite has already established the qualifier IS the os package, so
// the lookups are exact by construction.
func osImported(info *types.Info, expr ast.Expr) *types.Package {
	ident := expr.(*ast.Ident)
	return info.ObjectOf(ident).(*types.PkgName).Imported()
}

// hasWriteFlag reports whether any argument carries an os flag that implies
// writing, in any spelling: a literal `os.O_*` selector, a constant expression
// the checker folded (`const mode = os.O_WRONLY | os.O_CREATE`), or a local
// variable whose assignments BEFORE the open carry either — what the variable
// becomes after the open opened nothing.
func hasWriteFlag(info *types.Info, body *ast.BlockStmt, osPkg *types.Package, call *ast.CallExpr) bool {
	mask := writeFlagMask(osPkg)
	for _, arg := range call.Args {
		if flagWrites(info, body, mask, arg, call.Pos()) {
			return true
		}
	}
	return false
}

// flagMask is the OR of write-implying open-flag values.
type flagMask int64

// writeFlagMask is the OR of the os package's write-implying flag values,
// read from the type-checked os package itself rather than hardcoding any
// platform's numbers.
func writeFlagMask(osPkg *types.Package) flagMask {
	var mask flagMask
	for name, implies := range writeFlags {
		if implies {
			mask |= flagValue(osPkg, flagName(name))
		}
	}
	return mask
}

// flagName is the spelled name of an os open-flag constant.
type flagName string

// flagValue is one named os constant's folded value, zero when absent.
func flagValue(osPkg *types.Package, name flagName) flagMask {
	c, ok := osPkg.Scope().Lookup(string(name)).(*types.Const)
	if !ok {
		return 0
	}
	v, exact := constant.Int64Val(c.Val())
	if !exact {
		return 0
	}
	return flagMask(v)
}

// flagWrites reports whether one flag expression implies writing, chasing a
// local variable's assignments one level.
func flagWrites(info *types.Info, body *ast.BlockStmt, mask flagMask, expr ast.Expr, before token.Pos) bool {
	if constWrites(info, mask, expr) || selectorWrites(info, expr) {
		return true
	}
	ident, ok := ast.Unparen(expr).(*ast.Ident)
	if !ok {
		return false
	}
	return assignedFlagWrites(info, body, mask, info.ObjectOf(ident), before)
}

// constWrites reports a checker-folded constant expression carrying a write
// bit.
func constWrites(info *types.Info, mask flagMask, expr ast.Expr) bool {
	value := info.Types[expr].Value
	if value == nil {
		return false
	}
	v, exact := constant.Int64Val(constant.ToInt(value))
	return exact && flagMask(v)&mask != 0
}

// selectorWrites reports a literal os.O_* mention anywhere beneath expr.
func selectorWrites(info *types.Info, expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && isOSPackage(info, selector.X) && writeFlags[selector.Sel.Name] {
			found = true
		}
		return !found
	})
	return found
}

// assignedFlagWrites chases a local flag variable to the expressions assigned
// to it anywhere in this body — `flags := os.O_WRONLY | os.O_CREATE` or a
// later `flags |= os.O_APPEND` — and judges those.
func assignedFlagWrites(
	info *types.Info,
	body *ast.BlockStmt,
	mask flagMask,
	object types.Object,
	before token.Pos,
) bool {
	if object == nil {
		return false
	}
	found := false
	walkOwn(body, func(node ast.Node, _ deferredness) {
		if found || node.End() >= before {
			return
		}
		switch at := node.(type) {
		case *ast.AssignStmt:
			found = assignsFlagTo(info, mask, object, at.Lhs, at.Rhs)
		case *ast.ValueSpec:
			found = specAssignsFlagTo(info, mask, object, at)
		}
	})
	return found
}

// assignsFlagTo reports an assignment binding a write-implying expression to
// the object.
func assignsFlagTo(info *types.Info, mask flagMask, object types.Object, lhs, rhs []ast.Expr) bool {
	for i, target := range lhs {
		ident, ok := target.(*ast.Ident)
		if !ok || info.ObjectOf(ident) != object || i >= len(rhs) {
			continue
		}
		if constWrites(info, mask, rhs[i]) || selectorWrites(info, rhs[i]) {
			return true
		}
	}
	return false
}

// specAssignsFlagTo reports a var declaration binding a write-implying
// expression to the object.
func specAssignsFlagTo(info *types.Info, mask flagMask, object types.Object, spec *ast.ValueSpec) bool {
	for i, name := range spec.Names {
		if info.ObjectOf(name) != object || i >= len(spec.Values) {
			continue
		}
		if constWrites(info, mask, spec.Values[i]) || selectorWrites(info, spec.Values[i]) {
			return true
		}
	}
	return false
}

// osPath is the import path this analyzer resolves against. It is a constant
// rather than a parameter because every call site passes it: the analyzer is
// about os file opens specifically, so a package parameter was a generalization
// nothing used, which is what unparam reported.
const osPath importPath = "os"

// isOSPackage reports whether expr names the imported os package, resolved
// through the type info rather than the identifier's spelling — an aliased
// import must still be recognized, and a local variable called os must not be.
func isOSPackage(info *types.Info, expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := info.ObjectOf(ident).(*types.PkgName)
	return ok && importPath(pkg.Imported().Path()) == osPath
}
