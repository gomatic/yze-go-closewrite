package closewrite

import "go/ast"

// discardedCloses collects the DEFERRED Close calls in body whose error goes
// nowhere.
//
// Deferred is the whole point, and it is what keeps this rule quiet. A
// deferred close runs on the SUCCESS path, where its discarded error is the
// only remaining signal that the file is bad — that is the defect. An inline
// `_ = f.Close()` sitting just before `return nil, err` is the opposite: a
// deliberate cleanup on a path that is already failing for a reason the caller
// is about to be told. Nothing was written, nothing is lost, and reporting it
// buries the real findings. Measured across 186 modules, that single
// distinction accounted for four of the six false positives.
//
// Two shapes discard the error, and they are the only two: `defer f.Close()`,
// and `defer func() { _ = f.Close() }()`. The named-return closure
// `defer func() { err = f.Close() }()` binds the error and is silent here —
// recognising it matters more than it looks, because it is the correct way to
// return a Close error from a defer, and a rule that flagged it would push
// authors away from the very fix it is asking for.
func discardedCloses(body *ast.BlockStmt) []*ast.CallExpr {
	var discarded []*ast.CallExpr
	walkOwn(body, func(node ast.Node) {
		if deferred, ok := node.(*ast.DeferStmt); ok {
			discarded = append(discarded, deferredDiscards(deferred)...)
		}
	})
	return discarded
}

// deferredDiscards yields the calls a deferred statement throws away: the
// deferred call itself, plus any discarding statement in a deferred closure.
func deferredDiscards(deferred *ast.DeferStmt) []*ast.CallExpr {
	literal, ok := deferred.Call.Fun.(*ast.FuncLit)
	if !ok {
		return []*ast.CallExpr{deferred.Call}
	}
	var discarded []*ast.CallExpr
	ast.Inspect(literal.Body, func(node ast.Node) bool {
		if call, ok := discardedIn(node); ok {
			discarded = append(discarded, call)
		}
		return true
	})
	return discarded
}

// discardedIn yields the call a statement discards, if it discards one.
func discardedIn(node ast.Node) (*ast.CallExpr, bool) {
	switch stmt := node.(type) {
	case *ast.ExprStmt:
		call, ok := stmt.X.(*ast.CallExpr)
		return call, ok
	case *ast.AssignStmt:
		return blankAssigned(stmt)
	}
	return nil, false
}

// boundCalls collects the calls in body whose result IS bound — assigned to at
// least one non-blank name, or returned to the caller — nested calls included,
// so `return errors.Join(err, f.Close())` carries the Close.
//
// Binding is the whole test. An earlier revision collected every call that was
// not itself a deferred discard, and the hole swallowed the rule: writing
// through `fmt.Fprintln(f, …)`, wrapping with `bufio.NewWriter(f)`, or even
// passing the file to a function that returns nothing all counted as
// "handling", and the analyzer's own canonical target — a routine
// `defer f.Close()` over a written file — went unreported whenever the write
// was spelled as a call taking the file. A call that binds nothing proves
// nothing; only a bound result can carry the close error somewhere.
func boundCalls(body *ast.BlockStmt) []*ast.CallExpr {
	var bound []*ast.CallExpr
	walkOwn(body, func(node ast.Node) {
		switch stmt := node.(type) {
		case *ast.AssignStmt:
			if !allBlank(stmt.Lhs) {
				bound = append(bound, callsUnder(stmt.Rhs...)...)
			}
		case *ast.ReturnStmt:
			bound = append(bound, callsUnder(stmt.Results...)...)
		}
	})
	return bound
}

// blankAssigned yields the call of an assignment that throws every result away.
func blankAssigned(assign *ast.AssignStmt) (*ast.CallExpr, bool) {
	if len(assign.Rhs) != 1 {
		return nil, false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok || !allBlank(assign.Lhs) {
		return nil, false
	}
	return call, true
}

// allBlank reports whether every target is the blank identifier.
func allBlank(targets []ast.Expr) bool {
	for _, target := range targets {
		ident, ok := target.(*ast.Ident)
		if !ok || ident.Name != "_" {
			return false
		}
	}
	return len(targets) > 0
}
