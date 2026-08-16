package closewrite

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typedBody parses one snippet, type-checks it, and yields the checker's info
// with the body of its `probe` function — so a helper can be driven on nodes
// the checker actually produced rather than on a hand-built approximation of
// them, which is how an earlier guard test came to be satisfied by the walk
// falling short instead of by the guard.
func typedBody(t *testing.T, source probeSource) (*types.Info, *ast.BlockStmt) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "probe.go", string(source), 0)
	require.NoError(t, err)

	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	_, err = (&types.Config{}).Check("probe", fset, []*ast.File{file}, info)
	require.NoError(t, err)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "probe" {
			return info, fn.Body
		}
	}
	require.FailNow(t, "the snippet declares no probe function")
	return nil, nil
}

// probeSource is the Go source of one type-checkable snippet.
type probeSource string

// statementsOf yields the assignments of a body in source order, which is how a
// test addresses a particular statement without hard-coding a position.
func statementsOf(body *ast.BlockStmt) []ast.Stmt {
	var found []ast.Stmt
	ast.Inspect(body, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.AssignStmt, *ast.IncDecStmt:
			found = append(found, node.(ast.Stmt))
		}
		return true
	})
	return found
}

// callsOf yields the call expressions of a body in source order.
func callsOf(body *ast.BlockStmt) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			found = append(found, call)
		}
		return true
	})
	return found
}

// TestFailingSpansCoverOnlyAnAlreadyFailingBranch names the invariant
// failingSpans documents: a span is opened by an error check and never by
// anything else, and it covers the branch reached WITH the failure in hand —
// the body of `err != nil`, the else of `err == nil` — so a discard on the
// success side of either is not swallowed by one.
func TestFailingSpansCoverOnlyAnAlreadyFailingBranch(t *testing.T) {
	t.Parallel()

	info, body := typedBody(t, `package probe

func probe(err error, other error, n int, ready bool) int {
	total := 0
	if err != nil {
		total++
	}
	if err == nil {
		total += 2
	} else {
		total += 3
	}
	if n != 0 {
		total += 4
	}
	if err != other {
		total += 5
	}
	if ready {
		total += 6
	}
	if n != 0 {
	} else {
		total += 7
	}
	return total
}
`)
	spans := failingSpans(info, body)
	at := statementsOf(body)
	require.Len(t, at, 8, "the initialiser and the seven guarded statements")

	assert.False(t, spans.covers(at[0].Pos()), "the initialiser is on no branch at all")
	assert.True(t, spans.covers(at[1].Pos()), "the body of `err != nil` runs with the failure in hand")
	assert.False(t, spans.covers(at[2].Pos()), "the body of `err == nil` is the SUCCESS side")
	assert.True(t, spans.covers(at[3].Pos()), "its else is the failing side")
	assert.False(t, spans.covers(at[4].Pos()), "a count compared to zero establishes no failure")
	assert.False(t, spans.covers(at[5].Pos()), "an error compared to another error establishes no failure")
	assert.False(t, spans.covers(at[6].Pos()), "a bare boolean is not a comparison")
	assert.False(t, spans.covers(at[7].Pos()), "the else of a non-error check is not a failing branch")
}
