// internal/term/term_test.go
package term_test

import (
	"os"
	"testing"

	"github.com/host452b/yoyo/internal/term"
)

func TestTerm_NewDoesNotPanic(t *testing.T) {
	// Can't easily test raw mode in unit tests (stdin may not be a tty).
	// Just ensure New() works and Restore() is safe to call without EnableRaw.
	t.Run("restore without enable is safe", func(t *testing.T) {
		tr := term.New(os.Stdin)
		tr.Restore() // should not panic
	})
}
