package closewrite

import (
	"go/ast"
	"go/token"
	"go/types"
)

// span is a half-open source range, `from` inclusive and `to` exclusive.
type span struct {
	from token.Pos
	to   token.Pos
}

// failing is the set of spans in one function body that are reached only once
// an error check has already established a non-nil error. A discarded Close
// inside one of them is cleanup on a path the caller is about to be told about,
// not a loss — see discardedCloses.
type failing []span

// covers reports whether a position sits inside any failing span.
func (f failing) covers(pos token.Pos) bool {
	for _, at := range f {
		if pos >= at.from && pos < at.to {
			return true
		}
	}
	return false
}

// failingSpans collects the already-failing regions of one function body,
// including those inside a deferred closure. The closure runs on every path;
// the body of an `if err != nil` INSIDE it does not, and that shape is the
// rollback idiom — see onSuccessPath.
func failingSpans(info *types.Info, body *ast.BlockStmt) failing {
	var spans failing
	walkOwn(body, func(node ast.Node, _ deferredness) {
		spans = append(spans, failingBranch(info, node)...)
	})
	return spans
}

// failingBranch yields the branch of an if statement reached only with a
// non-nil error in hand: the body of `if err != nil`, and the else of
// `if err == nil`.
//
// Only a comparison of an error-typed operand against the predeclared nil
// counts. The question is whether the branch runs with a failure already in
// hand, and no other condition answers it — a count, a flag, or a comparison
// against a sentinel all leave the close error the last word on the file.
//
// The language admits two such comparisons and no more, an interface value
// being comparable to nil and orderable against nothing, so a condition that
// reaches the last line is `==` and there is no third shape hiding in it.
//
// The operator token is what separates the two, and THREE spellings of that
// were written here and measured against the suite. None passes. The comparison
// below draws yze/enumdiscrim; its prescribed remedy, an exhaustive switch,
// draws `exhaustive` naming all 80 token.Token members the arms do not list;
// and a `map[token.Token]` from operator to arm — which has no fall-through at
// all — draws `exhaustive` on the map literal. The comparison is kept because
// it is the simplest of the three and the only one whose finding is a rule
// documented as a permanent probe, and the finding is left standing rather than
// silenced with a per-repo soft entry this repository does not otherwise have.
//
// The remaining alternatives are worse than a finding: reading the operator's
// String() is a stringly-typed evasion, wrapping token.Token in a local type is
// a rename that changes only which analyzer looks, and deciding the arm from
// what the branch returns instead reports every cleanup that continues a loop.
func failingBranch(info *types.Info, node ast.Node) []span {
	stmt, ok := node.(*ast.IfStmt)
	if !ok {
		return nil
	}
	binary, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || !comparesErrorToNil(info, binary) {
		return nil
	}
	if binary.Op == token.NEQ {
		return []span{{from: stmt.Body.Pos(), to: stmt.Body.End()}}
	}
	return elseSpan(stmt)
}

// elseSpan is an if statement's else branch, which is the already-failing path
// when the condition is `err == nil`. A check written without an else opens no
// failing region at all.
func elseSpan(stmt *ast.IfStmt) []span {
	if stmt.Else == nil {
		return nil
	}
	return []span{{from: stmt.Else.Pos(), to: stmt.Else.End()}}
}

// comparesErrorToNil reports a comparison of an error-typed operand against the
// predeclared nil, written in either order.
func comparesErrorToNil(info *types.Info, binary *ast.BinaryExpr) bool {
	return isNilIdent(info, binary.Y) && isErrorOperand(info, binary.X) ||
		isNilIdent(info, binary.X) && isErrorOperand(info, binary.Y)
}

// isNilIdent reports the PREDECLARED nil, resolved through the type info so a
// local variable spelled nil is not mistaken for it.
func isNilIdent(info *types.Info, expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return info.ObjectOf(ident) == types.Universe.Lookup("nil")
}

// isErrorOperand reports an expression whose type is an error.
func isErrorOperand(info *types.Info, expr ast.Expr) bool {
	return isErrorType(info.TypeOf(expr))
}
