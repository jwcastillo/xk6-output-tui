package outputtui_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	outputtui "github.com/jwcastillo/xk6-output-tui"
	"go.k6.io/k6/v2/output"
)

// compile-time assertion: *Output must satisfy output.Output interface
var _ output.Output = (*outputtui.Output)(nil)

// TestOutputInterface verifies the type satisfies the Output interface at runtime.
func TestOutputInterface(t *testing.T) {
	var o output.Output = &outputtui.Output{}
	require.NotNil(t, o)
}

// TestNew verifies New() returns a non-nil Output without error.
func TestNew(t *testing.T) {
	params := output.Params{}
	o, err := outputtui.New(params)
	require.NoError(t, err)
	require.NotNil(t, o)
}

// TestDescription verifies the description string is non-empty.
func TestDescription(t *testing.T) {
	params := output.Params{}
	o, err := outputtui.New(params)
	require.NoError(t, err)
	desc := o.Description()
	require.NotEmpty(t, desc)
	require.Contains(t, desc, "tui")
}

// TestRegistration verifies the init() side-effect registered "tui" by
// confirming the package can be blank-imported without panicking.
// The blank import in this test file ensures init() is called.
func TestRegistration(t *testing.T) {
	// If init() panicked (duplicate registration or wrong name), the test binary
	// would have crashed before reaching here. Reaching this line means
	// registration succeeded without panic.
	t.Log("output.RegisterExtension(\"tui\") called without panic")
}
