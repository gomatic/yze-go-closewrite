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
