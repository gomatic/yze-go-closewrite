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
// to write: os.Create, os.CreateTemp, or os.OpenFile whose FLAG argument
// carries O_WRONLY, O_RDWR, O_CREATE, O_APPEND or O_TRUNC. The permission
// argument is never read as a flag: it is a mode bitmask that collides with the
// flag values, so reading it reported read-only opens, and reported them
// differently on darwin and on linux.
//
// # What is discarded, and where
//
// A Close whose error goes nowhere is reported wherever it sits on a path that
// is not already failing — `defer f.Close()`, `f.Close()` and `_ = f.Close()`
// throw away the same error and lose the same data, so the rule cannot turn on
// the `defer` keyword without making evasion one token cheaper than compliance.
// The remedy in every spelling is to bind the error: return it, assign it, or
// take it out of a deferred closure with `defer func() { err = f.Close() }()`.
//
// # Every exemption
//
// Each of these silences a Close that the clauses above would otherwise report.
// They are the complete set; anything else that goes unreported is a defect.
//
//  1. A reader's Close. Nothing is lost by failing to close a file you only
//     read, which keeps the overwhelmingly common `defer f.Close()` after
//     os.Open silent, exactly as it should be.
//  2. A discard on an ALREADY-FAILING path — a Close inside `if err != nil { … }`
//     or inside the else of `if err == nil { … }`. The caller is about to be
//     told about a failure that is not this one, and reporting it buries the
//     real findings.
//  3. A close already handled elsewhere in the same body: a bound `f.Close()`,
//     or a bound SEAM — a call taking the file as its only argument and
//     returning nothing but an error, which is what `return closeOutput(file)`
//     looks like and is the sanctioned repair for this rule. Nothing wider
//     settles anything; in particular a checked WRITE taking the file does not,
//     because whether the bytes reached the disk is decided at Close.
//
// # Every scope limitation
//
// These are not exemptions — nothing here was judged and forgiven. They are
// shapes the analyzer cannot see, and each is a silence a reader should not
// mistake for approval.
//
//  1. One function body. A file handed to another function is that function's
//     responsibility, and widening this would mean guessing about ownership.
//  2. Plain identifiers on both sides. A file held in a field —
//     `h.f, err = os.Create(p)` with `defer h.f.Close()` — is neither opened
//     nor closed as far as this rule can tell.
//  3. Flag resolution reaches a literal `os.O_*` selector, a constant the
//     checker folded, and one level of assignment to a local variable in the
//     same body. A flag arriving as a parameter, bound from a call's second
//     result, or spread from a call supplying the whole argument list, resolves
//     to nothing and the open is silent.
//  4. Test files are judged like any other source: this analyzer declares no
//     test scope and the yze runner does not list it as source-only.
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

// run reports every discarded Close on a write-opened file. A DEFERRED
// function literal is skipped here: it runs as part of its enclosing
// function's exit, so the enclosing function's own walk carries it — visiting
// it again would report its findings twice.
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.WithStack(
		[]ast.Node{(*ast.FuncDecl)(nil), (*ast.FuncLit)(nil)},
		func(node ast.Node, isPush bool, stack []ast.Node) bool {
			if !isPush || isDeferredLiteral(node, stack) {
				return true
			}
			if body := bodyOf(node); body != nil {
				reportBody(pass, body)
			}
			return true
		},
	)
	return nil, nil
}

// isDeferredLiteral reports a function literal that is the deferred call of a
// defer statement — `defer func() { … }()` — whose body belongs to the
// enclosing function's flow.
func isDeferredLiteral(node ast.Node, stack []ast.Node) bool {
	lit, ok := node.(*ast.FuncLit)
	if !ok || len(stack) < 3 {
		return false
	}
	call, ok := stack[len(stack)-2].(*ast.CallExpr)
	if !ok || call.Fun != lit {
		return false
	}
	deferred, ok := stack[len(stack)-3].(*ast.DeferStmt)
	return ok && deferred.Call == call
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
	for _, call := range discardedCloses(pass.TypesInfo, body) {
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
// Two shapes settle a file, and both require the call's result to be BOUND. The
// direct one is a Close call whose error is assigned or returned. The other is
// a seam — see seamedClose.
func closedWithHandling(info *types.Info, body *ast.BlockStmt) map[types.Object]bool {
	settled := map[types.Object]bool{}
	for _, call := range boundCalls(body) {
		if name, ok := closedObject(info, call); ok {
			settled[name] = true
		}
		if name, ok := seamedClose(info, call); ok {
			settled[name] = true
		}
	}
	return settled
}

// seamedClose names the file a bound call closes on this function's behalf.
//
// The shape is f.Close()'s own, and nothing wider: ONE argument, the file
// itself, and ONE result, an error. That is what a seam looks like —
// `return closeOutput(file)`, a package-level function var standing in for
// f.Close() so a test can force the failure — and it is the sanctioned repair
// for this very rule, which an earlier version of this analyzer reported.
//
// The narrowness is the point, and the reason is this analyzer's own canonical
// target. An earlier revision settled a file whenever ANY bound
// error-returning call took it as an argument, and that swallowed the rule:
// `io.Copy(f, src)` into a created file — the commonest way there is to write
// one — went silent, while the byte-identical `f.Write(b)` was reported, so the
// verdict turned on how the write was SPELLED. A checked write is not a handled
// close: it says the bytes reached the buffer, and whether they reached the
// disk is decided at Close, which is the failure this rule exists to catch. A
// call taking a second argument is not closing on your behalf; one returning a
// second result is telling you about something other than the close.
func seamedClose(info *types.Info, call *ast.CallExpr) (types.Object, bool) {
	if len(call.Args) != 1 || !resultIsError(info, call) {
		return nil, false
	}
	ident, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := info.ObjectOf(ident)
	return object, object != nil
}

// resultIsError reports a call whose ONE result is an error, which is the
// contract: a second result means the call was asked for something besides the
// close, and a bound `bufio.NewWriter(f)` binds a writer and proves nothing.
//
// A call with several results has a *types.Tuple type, and a tuple implements
// no interface, so it is not an error type and needs no separate test. An
// explicit tuple guard was written here and then removed as inert: with it
// deleted the whole suite stays green and every corpus verdict is unchanged, so
// no case could ever discriminate it.
func resultIsError(info *types.Info, call *ast.CallExpr) bool {
	return isErrorType(info.TypeOf(call))
}

// isErrorType reports a type that IS an error: the universe error itself or any
// type implementing it, since a seam returning *os.PathError carries the close
// error as surely as one returning error. A call the checker recorded no type
// for carries nothing.
//
// A type whose POINTER implements error is deliberately not one. The caller of
// such a seam receives a plain value that is not assignable to error and on
// which errors.Is cannot be called, so nothing about the close was handled —
// and accepting it silenced a genuine discarded Close on a written file.
func isErrorType(at types.Type) bool {
	if at == nil {
		return false
	}
	errType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	return types.Implements(at, errType)
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
