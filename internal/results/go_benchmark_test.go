package results

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestGoBenchmarkEvidenceRequiresMeasurementAndPackagePass(t *testing.T) {
	start := `{"Action":"run","Package":"example/p","Test":"BenchmarkOne"}`
	output := `{"Action":"output","Package":"example/p","Test":"BenchmarkOne","Output":"BenchmarkOne-8\t1\t20.0 ns/op\n"}`
	pass := `{"Action":"pass","Package":"example/p"}`
	for _, input := range []string{start + "\n" + pass, start + "\n" + output, start + "\n" + output + "\n" + `{"Action":"pass","Package":"example/other"}`, start + "\n" + pass + "\n" + output} {
		got, err := ParseGoJSON(input)
		if err != nil || len(got) != 0 {
			t.Fatalf("incomplete evidence yielded %+v, %v", got, err)
		}
	}
	got, err := ParseGoJSON(strings.Join([]string{start, output, pass}, "\n"))
	if err != nil || len(got) != 1 || got[0].FullName != "example/p.BenchmarkOne" || got[0].Status != domain.StatusPassed {
		t.Fatalf("measurement evidence: %+v, %v", got, err)
	}
}

func TestGoBenchmarkEvidenceHandlesSplitAndLongOutput(t *testing.T) {
	long, err := json.Marshal(map[string]string{"Action": "output", "Package": "example/p", "Test": "BenchmarkOne", "Output": strings.Repeat("x", 128*1024) + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"Action":"run","Package":"example/p","Test":"BenchmarkOne"}`,
		string(long),
		`{"Action":"output","Package":"example/p","Test":"BenchmarkOne","Output":"BenchmarkOne-8\t"}`,
		`{"Action":"output","Package":"example/p","Test":"BenchmarkOne","Output":"1\t20.0 ns/op\n"}`,
		`{"Action":"pass","Package":"example/p"}`,
	}, "\n")
	got, err := ParseGoJSON(input)
	if err != nil || len(got) != 1 || got[0].Status != domain.StatusPassed || len(got[0].Output) > 8192 {
		t.Fatalf("split measurement after long output: %+v %v", got, err)
	}
}

func TestGoBenchmarkParentRequiresConclusiveChildren(t *testing.T) {
	parent := `{"Action":"run","Package":"example/p","Test":"BenchmarkParent"}`
	child := `{"Action":"run","Package":"example/p","Test":"BenchmarkParent/child"}`
	skip := `{"Action":"skip","Package":"example/p","Test":"BenchmarkParent/child"}`
	pass := `{"Action":"pass","Package":"example/p"}`
	got, err := ParseGoJSON(strings.Join([]string{parent, child, pass}, "\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("unresolved child yielded %+v, %v", got, err)
	}
	got, err = ParseGoJSON(strings.Join([]string{parent, child, skip, pass}, "\n"))
	if err != nil || len(got) != 2 {
		t.Fatalf("skipped child: %+v, %v", got, err)
	}
	for _, result := range got {
		if result.Status != domain.StatusSkipped {
			t.Fatalf("skipped tree reported %+v", got)
		}
	}
}
