// internal/proxy/proxy_test.go
package proxy_test

import (
	"testing"

	"yoyo/internal/proxy"
)

// TestProxy_PackageCompiles is a compile-time check.
// Full integration test is in Task 14.
func TestProxy_PackageCompiles(t *testing.T) {
	// Verify proxy.Config fields compile correctly
	_ = proxy.Config{
		Delay:   0,
		Enabled: true,
	}
}
