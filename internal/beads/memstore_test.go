package beads_test

import (
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/beads/beadstest"
)

func TestMemStore(t *testing.T) {
	beadstest.RunStoreTests(t, func() beads.Store {
		return beads.NewMemStore()
	})
}
