package mcp

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"testing"
)

func TestCompactItemPreservesOverlapWarnings(t *testing.T) {
	got := compactItem(domain.RunItem{Warnings: []string{"possible overlap"}})
	warnings, ok := got["warnings"].([]string)
	if !ok || len(warnings) != 1 || warnings[0] != "possible overlap" {
		t.Fatalf("warnings missing: %+v", got)
	}
}
