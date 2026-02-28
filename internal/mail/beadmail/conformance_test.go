package beadmail

import (
	"testing"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/mail"
	"github.com/steveyegge/gascity/internal/mail/mailtest"
)

func TestBeadmailConformance(t *testing.T) {
	mailtest.RunProviderTests(t, func(_ *testing.T) mail.Provider {
		return New(beads.NewMemStore())
	})
}
