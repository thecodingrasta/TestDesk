package discovery

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
)

func TestGoJSONDiscoveryRequiresEachPackageToFinish(t *testing.T) {
	provider := config.GoTestProvider()
	for _, input := range []string{
		"",
		`{}`,
		`{"Action":"start","Package":"example/a"}` + "\n" + `{"Action":"output","Package":"example/a","Output":"TestFirst\n"}`,
		`{"Action":"start","Package":"example/a"}` + "\n" + `{"Action":"pass","Package":"example/a"}` + "\n" + `{"Action":"start","Package":"example/b"}`,
		`{"Action":"start","Package":"example/a"}` + "\n" + `{"Action":"fail","Package":"example/a"}`,
		`{"Action":"start","Package":"example/a"}` + "\n" + `{"Action":"pass","Package":"example/a","Test":"TestFirst"}`,
	} {
		if cases, err := Parse(provider, input); err == nil {
			t.Fatalf("accepted incomplete stream %q: %+v", input, cases)
		}
	}
	complete := `{"Action":"start","Package":"example/a"}` + "\n" + `{"Action":"output","Package":"example/a","Output":"TestFirst\n"}` + "\n" + `{"Action":"pass","Package":"example/a"}` + "\n" + `{"Action":"start","Package":"example/empty"}` + "\n" + `{"Action":"skip","Package":"example/empty"}`
	if cases, err := Parse(provider, complete); err != nil || len(cases) != 1 {
		t.Fatalf("completed and empty packages: %+v, %v", cases, err)
	}
}
