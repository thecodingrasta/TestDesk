package manager

import (
	"testing"
	"time"
)

// A clock that repeats, as Windows wall-clock time can, must not issue the same run identifier twice.
func TestRunIDsStayUniqueAndOrderedWhenTheClockRepeats(t *testing.T) {
	fixed := time.Date(2026, 11, 22, 0, 42, 28, 500349200, time.UTC)
	source := &runIDSource{now: func() time.Time { return fixed }}
	seen := map[string]bool{}
	previous := ""
	for i := 0; i < 1000; i++ {
		id := source.next()
		if seen[id] {
			t.Fatalf("run identifier %s was issued twice at call %d", id, i)
		}
		if previous != "" && id <= previous {
			t.Fatalf("run identifier %s does not follow %s", id, previous)
		}
		seen[id] = true
		previous = id
	}
}

func TestProcessRunIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := newRunID()
		if seen[id] {
			t.Fatalf("run identifier %s was issued twice", id)
		}
		seen[id] = true
	}
}
