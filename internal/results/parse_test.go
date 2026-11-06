package results

import (
	"os"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestParseJUnit(t *testing.T) {
	data := []byte(`<testsuite name="suite"><testcase classname="A" name="pass" time="0.25"/><testcase classname="A" name="fail" time="0.10"><failure message="bad">stack</failure></testcase></testsuite>`)
	results, err := ParseJUnit(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != domain.StatusPassed || results[1].Status != domain.StatusFailed {
		t.Fatalf("unexpected statuses: %+v", results)
	}
	if results[0].DurationMillis != 250 {
		t.Fatalf("expected 250ms, got %d", results[0].DurationMillis)
	}
}

func TestParseTRX(t *testing.T) {
	trx := []byte(`<TestRun><Results><UnitTestResult testName="Acme.Tests.Login.Valid" outcome="Passed" duration="00:00:01.5000000"/><UnitTestResult testName="Acme.Tests.Login.Bad" outcome="Failed" duration="00:00:00.1000000"><Output><ErrorInfo><Message>nope</Message></ErrorInfo></Output></UnitTestResult></Results></TestRun>`)
	file := t.TempDir() + "/result.trx"
	if err := osWrite(file, trx); err != nil {
		t.Fatal(err)
	}
	results, err := ParseTRXFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[1].Status != domain.StatusFailed {
		t.Fatalf("unexpected TRX parse: %+v", results)
	}
}

func osWrite(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
