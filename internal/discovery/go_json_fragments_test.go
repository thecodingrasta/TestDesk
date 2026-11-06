package discovery

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
)

func TestGoJSONDiscoveryReassemblesInterleavedOutputLines(t *testing.T) {
	input := `{"Action":"start","Package":"example/a"}` + "\n" +
		`{"Action":"start","Package":"example/b"}` + "\n" +
		`{"Action":"output","Package":"example/a","Output":"TestLong"}` + "\n" +
		`{"Action":"output","Package":"example/b","Output":"TestOther"}` + "\n" +
		`{"Action":"output","Package":"example/a","Output":"Name\n"}` + "\n" +
		`{"Action":"output","Package":"example/b","Output":"Name\n"}` + "\n" +
		`{"Action":"pass","Package":"example/a"}` + "\n" +
		`{"Action":"pass","Package":"example/b"}`
	cases, err := Parse(config.GoTestProvider(), input)
	if err != nil || len(cases) != 2 || cases[0].Name != "TestLongName" || cases[1].Name != "TestOtherName" {
		t.Fatalf("split package output: %+v, %v", cases, err)
	}
}
