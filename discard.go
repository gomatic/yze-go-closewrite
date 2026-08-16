package closewrite

import "go/ast"

// discardedCloses collects the Close calls in body whose error goes nowhere on
// a path the analyzer can see is UNCONDITIONAL.
//
// Where a discard sits is what discriminates, and it is what keeps this rule
// quiet. A close reached unconditionally is the last remaining signal that the
// file is bad, whether it is spelled `defer f.Close()`, `f.Close()` or
// `_ = f.Close()` — all three throw the same error away and lose the same data,
// so a rule that turns on the `defer` keyword makes evasion one token cheaper
// than compliance, which is what an earlier revision did.
//
// A discard inside a BRANCH is not judged at all. The shape the exemption
// exists for is cleanup on a path already failing — `if err != nil { _ = f.Close(); return err }` —
// and deciding which branch is the failing one from the syntax cannot be done:
// an earlier draft of this file recognised `if err != nil` and the else of
// `if err == nil`, and an adversarial pass reported the identical cleanup
// written five other ways — errors.Is, a compound condition, a `switch` with an
// error case, a `goto fail` label, and an error accumulated in a slice and
// checked by length. That list has no end, which is the negative-universal
// shape docs/s03.md says no enumeration closes. Worse, recognising the check
// made it FORGEABLE: `if errAlways != nil { defer f.Close() }`, over a
// package-level non-nil error, silenced a finding the previous revision
// reported — two lines, no change in behaviour, a disablement the fix invented.
// So the branch is left alone, silence is the answer wherever the path is not
// plainly unconditional, and the cost is stated in the package comment.
//
// Inside a deferred closure a `return` reaches nobody, so the discarding shapes
// there are wider: any statement of `defer func() { … }()` that throws a call
// away. The named-return closure `defer func() { err = f.Close() }()` binds the
// error and is silent — recognising it matters more than it looks, because it
// is the correct way to return a Close error from a defer, and a rule that
// flagged it would push authors away from the very fix it is asking for.
func discardedCloses(body *ast.BlockStmt) []*ast.CallExpr {
	discarded := unconditionalDiscards(body.List, inlineDiscards)
	walkOwn(body, func(node ast.Node, _ deferredness) {
		if deferred, ok := node.(*ast.DeferStmt); ok {
			discarded = append(discarded, deferredDiscards(deferred)...)
		}
	})
	return discarded
}

// unconditionalDiscards applies a per-statement collector to the statements of
// ONE block and no deeper, so a discard reached only through a branch, a loop
// or a label is never collected.
func unconditionalDiscards(stmts []ast.Stmt, discardsIn func(ast.Node) []*ast.CallExpr) []*ast.CallExpr {
	discarded := make([]*ast.CallExpr, 0, len(stmts))
	for _, stmt := range stmts {
		discarded = append(discarded, discardsIn(stmt)...)
	}
	return discarded
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
// deferred call itself, plus the discarding statements a deferred closure runs
// UNCONDITIONALLY.
//
// Deferring is itself the unconditional act — once the statement executes the
// call is queued on every exit — so a `defer f.Close()` is judged wherever it
// is written, which is also what keeps `if x { defer f.Close() }` from being a
// two-line silence. Inside the closure the ordinary rule applies again: a close
// the closure runs only under a guard is the rollback idiom,
// `defer func() { if err != nil { f.Close(); os.Remove(f.Name()) } }()`, which
// is cleanup on the failing path written in the one place it can run. Reporting
// it was a false positive against internal/fuzz in the standard library.
func deferredDiscards(deferred *ast.DeferStmt) []*ast.CallExpr {
	literal, ok := deferred.Call.Fun.(*ast.FuncLit)
	if !ok {
		return []*ast.CallExpr{deferred.Call}
	}
	return unconditionalDiscards(literal.Body.List, deferredDiscardsIn)
}

// deferredDiscardsIn yields the calls one statement of a deferred closure
// throws away.
func deferredDiscardsIn(node ast.Node) []*ast.CallExpr {
	if call, discards := discardedIn(node); discards {
		return []*ast.CallExpr{call}
	}
	if assign, ok := node.(*ast.AssignStmt); ok {
		return blankAssignedAll(assign)
	}
	return nil
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
