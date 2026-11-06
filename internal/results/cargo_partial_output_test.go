package results

import (
	"strings"
	"testing"
)

func TestCargoTextDiscardsResultsOnScannerError(t *testing.T) {
	output := "test known ... ok\n" + strings.Repeat("x", 4*1024*1024+1) + "\n"
	results, err := ParseCargoText(output)
	if err == nil || len(results) != 0 {
		t.Fatalf("partial results returned after scanner failure: %d results, %v", len(results), err)
	}
}
