package closewrite

import (
	"go/ast"
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

// writeFlags are the os open flags that establish an intent to write.
var writeFlags = map[string]bool{
	"O_WRONLY": true,
	"O_RDWR":   true,
	"O_CREATE": true,
	"O_APPEND": true,
	"O_TRUNC":  true,
}

// writeOpened collects the variables in body assigned from a write-opening
// call, keyed by the object so shadowing in a nested scope cannot be confused
// with the outer file.
func writeOpened(info *types.Info, body *ast.BlockStmt) map[types.Object]bool {
	opened := map[types.Object]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if ok {
			collectOpened(info, assign, opened)
		}
		return true
	})
	return opened
}

// collectOpened records the assignment's first name when its right-hand side
// opens a file for writing.
func collectOpened(info *types.Info, assign *ast.AssignStmt, opened map[types.Object]bool) {
	if len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
		return
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok || !opensForWrite(info, call) {
		return
	}
	name, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return
	}
	if object := info.ObjectOf(name); object != nil {
		opened[object] = true
	}
}

// opensForWrite reports whether call is an os open establishing write intent.
func opensForWrite(info *types.Info, call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !isPackage(info, selector.X, "os") {
		return false
	}
	if writeOpeners[selector.Sel.Name] {
		return true
	}
	return selector.Sel.Name == "OpenFile" && hasWriteFlag(info, call.Args)
}

// hasWriteFlag reports whether any argument mentions an os flag that implies
// writing. The flags arrive as an OR of constants, so the whole argument list
// is scanned rather than a single position parsed.
func hasWriteFlag(info *types.Info, args []ast.Expr) bool {
	found := false
	for _, arg := range args {
		ast.Inspect(arg, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && isPackage(info, selector.X, "os") && writeFlags[selector.Sel.Name] {
				found = true
			}
			return !found
		})
	}
	return found
}

// isPackage reports whether expr names the given imported package, resolved
// through the type info rather than the identifier's spelling — an aliased
// import must still be recognized, and a local variable called os must not be.
func isPackage(info *types.Info, expr ast.Expr, want importPath) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := info.ObjectOf(ident).(*types.PkgName)
	return ok && importPath(pkg.Imported().Path()) == want
}
