package closewrite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	closewrite "github.com/gomatic/yze-go-closewrite"
)

// TestAnalyzerReportsDiscardedClosesOnWrittenFilesOnly drives the analyzer over
// the corpus in testdata, which carries both directions: the three shapes that
// discard a Close error on a write-opened file, and the four that must stay
// silent.
//
// The silent half is the load-bearing half. This rule replaced a proposal to
// turn on errcheck's check-blank, which measurement showed would add 1032
// findings across 97 repositories and catch ZERO of the Close defects it was
// meant to find — golangci's EXC0001 suppresses .*Close unconditionally. A
// replacement that merely fires more often would be the same mistake wearing a
// narrower name, so the reader's Close, the read-only OpenFile, the checked
// Close, and the named-return closure all have to stay quiet.
func TestAnalyzerReportsDiscardedClosesOnWrittenFilesOnly(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "a")
}

// TestRegistrationIsWellFormed pins the analyzer's declaration to the suite:
// the framework validates the shape, and a malformed registration would be
// discovered only when the aggregate refused to build.
func TestRegistrationIsWellFormed(t *testing.T) {
	t.Parallel()

	require.NoError(t, closewrite.Registration.Validate())
	assert.Equal(t, "yze/closewrite", closewrite.Registration.RuleID())
	assert.Same(t, closewrite.Analyzer, closewrite.Registration.Analyzer)
}

// TestOnlyABoundCallSettlesAFile pins the binding rule the settlement rests
// on, in both directions: writing through fmt, handing the file to a
// result-less call, wrapping it in bufio, discarding the close twice, and the
// var-declaration open all still report — while the safety-net pair, the
// return-bound close, the errors.Join'd close, the seamed close, and the
// documented bound-write heuristic stay silent.
func TestOnlyABoundCallSettlesAFile(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "bound")
}

// TestEveryWriteFlagSpellingIsRecognised pins flag resolution beyond the
// literal selector: a folded named constant, a local flag variable, and each
// write flag alone — O_WRONLY, O_RDWR, O_APPEND, O_TRUNC, and the documented
// O_CREATE decision — while the read-only open stays silent.
func TestEveryWriteFlagSpellingIsRecognised(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "flags")
}

// TestAClosureIsItsOwnFunction pins the one-visit-per-function property: an
// open-and-discard inside a literal is the literal's finding exactly once —
// analysistest fails on an extra diagnostic, so a double report cannot pass.
func TestAClosureIsItsOwnFunction(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), closewrite.Analyzer, "closures")
}
