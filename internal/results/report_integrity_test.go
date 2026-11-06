package results

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReportFilesRejectEmptyAndUnreadableInput(t *testing.T) {
	for _, format := range []string{"trx", "junit", "vitest-json"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			empty := filepath.Join(dir, "empty")
			if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(format, empty, ""); err == nil {
				t.Fatal("existing empty report was accepted")
			}
			if _, err := Parse(format, dir, ""); err == nil {
				t.Fatal("unreadable report path was accepted")
			}
			if got, err := Parse(format, filepath.Join(dir, "missing"), ""); err != nil || len(got) != 0 {
				t.Fatalf("missing report: got %v, error %v", got, err)
			}
		})
	}
}

func TestReportDirectoriesRejectEmptyReportsAlongsidePassingResults(t *testing.T) {
	for _, tc := range []struct{ format, extension, passing string }{
		{"trx-dir", ".trx", `<TestRun><Results><UnitTestResult testName="case" outcome="Passed"/></Results></TestRun>`},
		{"junit-dir", ".xml", `<testsuite><testcase classname="Fixture" name="case"/></testsuite>`},
	} {
		t.Run(tc.format, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "01-passing"+tc.extension), []byte(tc.passing), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "02-incomplete"+tc.extension), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := Parse(tc.format, dir, ""); err == nil || len(got) != 0 {
				t.Fatalf("accepted partial reports: %+v", got)
			}
		})
	}
}

func TestTRXRejectsWrongXMLRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wrong.trx")
	data := `<NotATestRun><Results><UnitTestResult testName="case" outcome="Passed"/></Results></NotATestRun>`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ParseTRXFile(path); err == nil {
		t.Fatalf("accepted non-TRX XML as test results: %+v", got)
	}
}
