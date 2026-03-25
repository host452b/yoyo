// internal/memory/memory.go
package memory

import "sync"

// Memory tracks approved prompt hashes within a session.
type Memory struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func New() *Memory {
	return &Memory{seen: make(map[string]struct{})}
}

// Seen reports whether this hash was previously approved.
func (m *Memory) Seen(hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.seen[hash]
	return ok
}

// Record marks a hash as approved.
func (m *Memory) Record(hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[hash] = struct{}{}
}
