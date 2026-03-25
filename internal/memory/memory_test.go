// internal/memory/memory_test.go
package memory_test

import (
	"testing"

	"yoyo/internal/memory"
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
