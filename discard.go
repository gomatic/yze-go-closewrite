package closewrite

import (
	"go/ast"
	"go/types"
)

// discardedCloses collects the Close calls in body whose error goes nowhere on
// a path that is not ALREADY FAILING.
//
// The path is what discriminates, and it is what keeps this rule quiet. A close
// reached on the success path is the last remaining signal that the file is
// bad, whether it is spelled `defer f.Close()`, `f.Close()` or
// `_ = f.Close()` — all three throw the same error away and lose the same data.
// An `_ = f.Close()` sitting inside `if err != nil { … return err }` is the
// opposite: a deliberate cleanup on a path already failing for a reason the
// caller is about to be told, where reporting it would bury the real findings.
// Measured across 186 modules, that single distinction accounted for four of
// the six false positives.
//
// An earlier revision discriminated on the `defer` keyword instead, which is
// not the same predicate: it left `f.Close(); return nil` — the identical loss,
// one token cheaper than the honest remedy — entirely silent, so evading the
// rule cost less than complying with it.
//
// Inside a deferred closure a `return` reaches nobody, so the discarding shapes
// there are wider: `defer f.Close()`, and any statement of
// `defer func() { … }()` that throws a call away. The named-return closure
// `defer func() { err = f.Close() }()` binds the error and is silent —
// recognising it matters more than it looks, because it is the correct way to
// return a Close error from a defer, and a rule that flagged it would push
// authors away from the very fix it is asking for.
func discardedCloses(info *types.Info, body *ast.BlockStmt) []*ast.CallExpr {
	var discarded []*ast.CallExpr
	walkOwn(body, func(node ast.Node, isDeferred deferredness) {
		if deferred, ok := node.(*ast.DeferStmt); ok {
			discarded = append(discarded, deferredDiscards(deferred)...)
			return
		}
		if isDeferred == ownFlow {
			discarded = append(discarded, inlineDiscards(node)...)
		}
	})
	return onSuccessPath(failingSpans(info, body), discarded)
}

// onSuccessPath keeps the discards that are NOT reached with a failure already
// in hand, wherever they sit.
//
// Applying this to a deferred closure's discards as well as to the ordinary
// ones is the whole reason it is a separate pass. The rollback idiom —
// `defer func() { if err != nil { f.Close(); os.Remove(f.Name()) } }()` over a
// named result — is exactly the cleanup this exemption exists for, written in
// the one place it can run, and an earlier draft of this file declared the
// opposite in a comment: that a deferred closure has no failing branch because
// it runs on every path. The closure runs on every path; its `if err != nil`
// body does not. That comment was a limitation blessed rather than a property,
// and it was found by adjudicating a finding this rule newly reported against
// internal/fuzz in the standard library.
func onSuccessPath(failing failing, calls []*ast.CallExpr) []*ast.CallExpr {
	var kept []*ast.CallExpr
	for _, call := range calls {
		if !failing.covers(call.Pos()) {
			kept = append(kept, call)
		}
	}
	return kept
}

// inlineDiscards yields the calls one ordinary statement throws away: a bare
// call statement, and an assignment whose targets are all blank. A `return` is
// not one here — outside a deferred closure it reaches the caller.
func inlineDiscards(node ast.Node) []*ast.CallExpr {
	assign, ok := node.(*ast.AssignStmt)
	if !ok {
		if call, discards := bareCall(node); discards {
			return []*ast.CallExpr{call}
		}
		return nil
	}
	if call, discards := blankAssigned(assign); discards {
		return []*ast.CallExpr{call}
	}
	return blankAssignedAll(assign)
}

// bareCall yields a statement's call when the statement is nothing but that
// call, so its results go nowhere at all.
func bareCall(node ast.Node) (*ast.CallExpr, bool) {
	stmt, ok := node.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	return call, ok
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
		if assign, ok := node.(*ast.AssignStmt); ok {
			discarded = append(discarded, blankAssignedAll(assign)...)
		}
		return true
	})
	return discarded
}

// discardedIn yields the call a statement discards, if it discards one. A
// return statement discards here too: these statements live inside a DEFERRED
// closure, whose return value the defer runtime throws away.
func discardedIn(node ast.Node) (*ast.CallExpr, bool) {
	switch stmt := node.(type) {
	case *ast.AssignStmt:
		return blankAssigned(stmt)
	case *ast.ReturnStmt:
		return returnedCall(stmt)
	}
	return bareCall(node)
}

// returnedCall yields a return statement's single call result, if that is
// what it returns.
func returnedCall(stmt *ast.ReturnStmt) (*ast.CallExpr, bool) {
	if len(stmt.Results) != 1 {
		return nil, false
	}
	call, ok := stmt.Results[0].(*ast.CallExpr)
	return call, ok
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
	walkOwn(body, func(node ast.Node, isDeferred deferredness) {
		bound = append(bound, boundIn(node, isDeferred)...)
	})
	return bound
}

// boundIn yields the calls one statement binds: a non-blank assignment's
// right-hand calls, and a return's results — unless the return sits in
// deferred flow, where the runtime discards the value and it binds nothing.
func boundIn(node ast.Node, isDeferred deferredness) []*ast.CallExpr {
	switch stmt := node.(type) {
	case *ast.AssignStmt:
		if !allBlank(stmt.Lhs) {
			return callsUnder(stmt.Rhs...)
		}
	case *ast.ReturnStmt:
		if isDeferred == ownFlow {
			return callsUnder(stmt.Results...)
		}
	}
	return nil
}

// blankAssigned yields the call of an assignment that throws every result
// away — the single-call form; the tuple form `_, _ = f.Close(), g.Close()`
// discards each call on its right-hand side and is collected whole by
// blankAssignedAll.
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

// blankAssignedAll yields every call an all-blank tuple assignment discards.
func blankAssignedAll(assign *ast.AssignStmt) []*ast.CallExpr {
	if !allBlank(assign.Lhs) || len(assign.Rhs) < 2 {
		return nil
	}
	var discarded []*ast.CallExpr
	for _, rhs := range assign.Rhs {
		if call, ok := rhs.(*ast.CallExpr); ok {
			discarded = append(discarded, call)
		}
	}
	return discarded
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
