
package cleaner

import (
	"testing"
)

func TestAlwaysFails(t *testing.T) {
	t.Error("This test always fails - used for CI failure testing")
}