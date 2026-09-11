package terminal

import (
	"os"
	"testing"

	"github.com/ianf339/roambench/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunWithIsolatedTmux(m.Run))
}
