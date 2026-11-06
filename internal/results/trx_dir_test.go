package results

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTRXDirAggregatesReportsAndRejectsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"first.trx", "second.trx"} {
		body := `<TestRun><Results><UnitTestResult testName="` + name + `" outcome="Passed" /></Results></TestRun>`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Parse("trx-dir", dir, "")
	if err != nil || len(got) != 2 {
		t.Fatalf("got %d results, error %v", len(got), err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.trx"), []byte("<TestRun>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse("trx-dir", dir, ""); err == nil {
		t.Fatal("malformed report was ignored")
	}
}
