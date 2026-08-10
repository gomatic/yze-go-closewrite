// Package closewrite provides a go/analysis analyzer that reports a discarded
// Close error on a file opened for WRITING.
//
// Close is where a write is finally decided. The kernel may defer the failure
// of an earlier Write until the descriptor is closed — a full disk, a quota, a
// dropped network filesystem — so a discarded Close error on a written file is
// the program declaring success over a file that may be truncated, empty, or
// absent. Eight such defects were found by hand across this fleet; six were
// real data loss, and every one of them read as a routine `defer f.Close()`.
//
// The rule is deliberately narrow, because a gate that cries wolf is worse
// than no gate. It fires only where the file's own creation proves the intent
// to write: os.Create, os.CreateTemp, or os.OpenFile with a write flag. A
// reader's Close is never reported — nothing is lost by failing to close a
// file you only read — which keeps the overwhelmingly common `defer f.Close()`
// after os.Open silent, exactly as it should be.
package closewrite

import (
	"go/ast"
	"go/types"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const message = "the Close error on %s is discarded, but it was opened for writing; " +
	"a deferred write failure surfaces at Close, so this reports success over a file that may be truncated"

// Analyzer reports discarded Close errors on files opened for writing.
var Analyzer = &analysis.Analyzer{
	Name:     "closewrite",
	Doc:      "reports a discarded Close error on a file opened for writing, where the loss is real data",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// Registration declares this analyzer to the yze framework.
var Registration = goyze.Registration{
	Name:       "closewrite",
	Categories: []goyze.Category{"errors"},
	URL:        "https://docs.gomatic.dev/yze/closewrite",
	Analyzer:   Analyzer,
}

// run reports every discarded Close on a write-opened file.
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil), (*ast.FuncLit)(nil)}, func(node ast.Node) {
		if body := bodyOf(node); body != nil {
			reportBody(pass, body)
		}
	})
	return nil, nil
}

// bodyOf yields a function's body, whichever form declared it.
func bodyOf(node ast.Node) *ast.BlockStmt {
	switch fn := node.(type) {
	case *ast.FuncDecl:
		return fn.Body
	case *ast.FuncLit:
		return fn.Body
	}
	return nil
}

// reportBody reports the discarded Closes among a body's write-opened files.
//
// Scope is one function: the open and its Close live together, and a file
// handed to another function is that function's responsibility, not this one's.
// Widening it would mean guessing about ownership, and a guess is how a gate
// starts producing findings nobody can act on.
func reportBody(pass *analysis.Pass, body *ast.BlockStmt) {
	written := writeOpened(pass.TypesInfo, body)
	if len(written) == 0 {
		return
	}
	settled := closedWithHandling(pass.TypesInfo, body)
	for _, call := range discardedCloses(body) {
		name, ok := closedObject(pass.TypesInfo, call)
		if ok && written[name] && !settled[name] {
			pass.Reportf(call.Pos(), message, name.Name())
		}
	}
}

// closedWithHandling names the files whose close is already accounted for
// somewhere in the body, so a second, deferred close of the same file is not a
// defect.
//
// Two shapes settle a file, and both require the call's result to be BOUND.
// The direct one is a Close call whose error is assigned or returned. The
// other is the file being HANDED to a bound call — which is what a seamed
// close looks like: `return closeOutput(file)`, a package-level function var
// standing in for f.Close() so a test can force the failure. That indirection
// is the sanctioned way to reach this very branch, and an earlier version of
// this analyzer reported both files whose closes had just been fixed that way.
//
// The bound-argument half is a documented heuristic with a known silence: a
// bound WRITE taking the file — `if _, err := fmt.Fprintln(f, s); err != nil` —
// also settles it, though a checked write is not a handled close. The trade is
// deliberate and fixtured (see boundwrite in the testdata): erring toward
// silence on a shape that demonstrably checks its errors costs less than
// flagging the seamed-close repair this rule itself asks for.
func closedWithHandling(info *types.Info, body *ast.BlockStmt) map[types.Object]bool {
	settled := map[types.Object]bool{}
	for _, call := range boundCalls(body) {
		if name, ok := closedObject(info, call); ok {
			settled[name] = true
		}
		if !resultsIncludeError(info, call) {
			continue
		}
		for _, name := range argumentObjects(info, call) {
			settled[name] = true
		}
	}
	return settled
}

// resultsIncludeError reports whether the call produces an error among its
// results — the only kind of bound result that could be carrying a close
// error. A bound `bufio.NewWriter(f)` binds a writer and proves nothing.
func resultsIncludeError(info *types.Info, call *ast.CallExpr) bool {
	at := info.TypeOf(call)
	if at == nil {
		return false
	}
	if tuple, ok := at.(*types.Tuple); ok {
		for i := range tuple.Len() {
			if isErrorType(tuple.At(i).Type()) {
				return true
			}
		}
		return false
	}
	return isErrorType(at)
}

// isErrorType reports the universe error type.
func isErrorType(at types.Type) bool {
	return types.Identical(at, types.Universe.Lookup("error").Type())
}

// argumentObjects names the variables passed directly as arguments to a call.
func argumentObjects(info *types.Info, call *ast.CallExpr) []types.Object {
	var passed []types.Object
	for _, arg := range call.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		if object := info.ObjectOf(ident); object != nil {
			passed = append(passed, object)
		}
	}
	return passed
}

// closedObject resolves the variable a Close call was made on.
func closedObject(info *types.Info, call *ast.CallExpr) (types.Object, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != closeMethod {
		return nil, false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := info.ObjectOf(receiver)
	return object, object != nil
}
