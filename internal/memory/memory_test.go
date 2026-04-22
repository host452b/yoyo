// internal/memory/memory_test.go
package memory_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/host452b/yoyo/internal/memory"
)

func TestMemory_NotSeenInitially(t *testing.T) {
	m := memory.New()
	if m.Seen("abc123") {
		t.Error("expected hash to not be seen initially")
	}
}

func TestMemory_RecordThenSeen(t *testing.T) {
	m := memory.New()
	m.Record("abc123")
	if !m.Seen("abc123") {
		t.Error("expected hash to be seen after Record")
	}
}

func TestMemory_DifferentHashesIndependent(t *testing.T) {
	m := memory.New()
	m.Record("hash1")
	if m.Seen("hash2") {
		t.Error("hash2 should not be seen after recording hash1")
	}
}

// Sanity: 10 000 unique records don't panic or OOM. Memory.Memory has no
// eviction — this locks in that we know it, and that the data structure
// at least survives a realistic long-session volume.
func TestMemory_LargeVolume(t *testing.T) {
	m := memory.New()
	for i := 0; i < 10000; i++ {
		m.Record(fmt.Sprintf("hash-%d", i))
	}
	// Spot-check that a mid-range entry is still seen.
	if !m.Seen("hash-5000") {
		t.Error("expected mid-range entry to still be present after 10k records")
	}
	// Totally new hash should still be correctly absent.
	if m.Seen("hash-not-recorded") {
		t.Error("expected new hash to not be seen")
	}
}

// Concurrent Record/Seen from multiple goroutines must be race-free.
// The proxy's outputCh handler calls Record/Seen from the single main
// goroutine today, but the package should stay safe if anyone ever
// reaches in from another goroutine (e.g. a background janitor task).
func TestMemory_ConcurrentAccess(t *testing.T) {
	m := memory.New()
	const goroutines = 16
	const perG = 500
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				h := fmt.Sprintf("g%d-%d", g, i)
				m.Record(h)
				if !m.Seen(h) {
					t.Errorf("just-recorded %q not seen", h)
				}
			}
		}(g)
	}
	wg.Wait()
}
