package closewrite

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The helpers below carry guards the go/analysis driver cannot reach: it only
// ever hands the analyzer well-formed nodes of the kinds the Preorder filter
// names. The guards still have to hold, because they are what keeps a
// malformed or unexpected shape from becoming a panic inside a linter that the
// whole fleet's gate depends on — a crash there fails every repository at
// once. Driving them directly is the design being testable, not a workaround.

// TestBodyOfYieldsNilForAnythingButAFunction pins the type switch's default.
func TestBodyOfYieldsNilForAnythingButAFunction(t *testing.T) {
	t.Parallel()

	body := &ast.BlockStmt{}

	assert.Same(t, body, bodyOf(&ast.FuncDecl{Body: body}))
	assert.Same(t, body, bodyOf(&ast.FuncLit{Body: body}))
	assert.Nil(t, bodyOf(&ast.ReturnStmt{}), "a non-function node has no body")
}

// TestClosedObjectRejectsAnythingButANamedReceiversClose pins the receiver
// resolution.
func TestClosedObjectRejectsAnythingButANamedReceiversClose(t *testing.T) {
	t.Parallel()

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}

	_, ok := closedObject(info, &ast.CallExpr{Fun: ast.NewIdent("Close")})
	assert.False(t, ok, "a bare call has no receiver")

	sync := &ast.SelectorExpr{X: ast.NewIdent("f"), Sel: ast.NewIdent("Sync")}
	_, ok = closedObject(info, &ast.CallExpr{Fun: sync})
	assert.False(t, ok, "only Close counts")

	field := &ast.SelectorExpr{
		X:   &ast.SelectorExpr{X: ast.NewIdent("h"), Sel: ast.NewIdent("f")},
		Sel: ast.NewIdent("Close"),
	}
	_, ok = closedObject(info, &ast.CallExpr{Fun: field})
	assert.False(t, ok, "a field receiver is not a local file this pass owns")

	unresolved := &ast.SelectorExpr{X: ast.NewIdent("f"), Sel: ast.NewIdent("Close")}
	_, ok = closedObject(info, &ast.CallExpr{Fun: unresolved})
	assert.False(t, ok, "an unresolved receiver names no object")
}

// TestIsErrorTypeRejectsAValueWhosePointerIsTheError names the invariant
// isErrorType documents: only a type assignable to error carries one. A value
// whose POINTER has the Error method is not — the caller holds something it
// cannot assign to error and cannot call errors.Is on, so nothing about the
// close was handled by receiving it.
func TestIsErrorTypeRejectsAValueWhosePointerIsTheError(t *testing.T) {
	t.Parallel()

	assert.False(t, isErrorType(nil), "a call the checker recorded no type for carries nothing")
	assert.False(t, isErrorType(types.Typ[types.Int]), "an int is not an error")
	assert.True(t, isErrorType(types.Universe.Lookup("error").Type()), "error is")
	assert.False(
		t,
		isErrorType(pointerOnlyError()),
		"a value whose POINTER implements error is not assignable to error",
	)
	assert.True(
		t,
		isErrorType(types.NewPointer(pointerOnlyError())),
		"its pointer is, which is what a seam returning *os.PathError looks like",
	)
}

// TestOnlyALoneErrorResultCanCarryACloseError pins what a seam's result must
// be: an error and nothing beside it. A tuple means the call was asked for
// something other than the close.
func TestOnlyALoneErrorResultCanCarryACloseError(t *testing.T) {
	t.Parallel()

	errType := types.Universe.Lookup("error").Type()
	intType := types.Typ[types.Int]
	carries := func(at types.Type) bool {
		call := &ast.CallExpr{Fun: ast.NewIdent("f")}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{call: {Type: at}}}
		return resultIsError(info, call)
	}

	assert.False(t, carries(nil), "no recorded type carries nothing")
	assert.False(t, carries(intType), "a lone non-error result carries nothing")
	assert.True(t, carries(errType), "a lone error result carries the close error")
	assert.False(t, carries(types.NewTuple(
		types.NewVar(0, nil, "", intType), types.NewVar(0, nil, "", errType))),
		"a tuple carrying an error was still asked for something besides the close")
}

// pointerOnlyError builds a named struct whose Error method has a POINTER
// receiver, so the type itself does not implement error and its pointer does.
func pointerOnlyError() *types.Named {
	name := types.NewTypeName(token.NoPos, nil, "verdict", nil)
	named := types.NewNamed(name, types.NewStruct(nil, nil), nil)
	receiver := types.NewVar(token.NoPos, nil, "v", types.NewPointer(named))
	results := types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String]))
	signature := types.NewSignatureType(receiver, nil, nil, nil, results, false)
	named.AddMethod(types.NewFunc(token.NoPos, nil, "Error", signature))
	return named
}

// TestSeamedCloseAcceptsOnlyACallShapedLikeFClose names the invariant
// seamedClose documents: nothing wider than f.Close()'s own shape settles a
// file. A second argument means the call was never closing on this function's
// behalf, a second result means it was asked about something else, and an
// argument that is not a name hands it no file at all.
func TestSeamedCloseAcceptsOnlyACallShapedLikeFClose(t *testing.T) {
	t.Parallel()

	info, body := typedBody(t, `package probe

type carrier struct{}

func closeIt(c *carrier) error            { return nil }
func writeTo(c *carrier, b []byte) error  { return nil }
func describe(c *carrier) (string, error) { return "", nil }
func refuse(msg string) error             { return nil }

func probe(c *carrier, b []byte) {
	_ = closeIt(c)
	_ = writeTo(c, b)
	_, _ = describe(c)
	_ = refuse("no file here")
}
`)
	calls := callsOf(body)
	require.Len(t, calls, 4)

	settled, ok := seamedClose(info, calls[0])
	assert.True(t, ok, "one argument, the file, and one error result is the seam")
	assert.Equal(t, "c", settled.Name())

	_, ok = seamedClose(info, calls[1])
	assert.False(t, ok, "a second argument means the call was not closing the file")

	_, ok = seamedClose(info, calls[2])
	assert.False(t, ok, "a second result means the call was asked about something else")

	_, ok = seamedClose(info, calls[3])
	assert.False(t, ok, "an argument that is not a name hands over no file")
}
