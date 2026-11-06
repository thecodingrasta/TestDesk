package discovery

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
)

func TestGoJSONDiscoveryRetainsPackageAndRejectsMalformedEvents(t *testing.T) {
	provider := config.GoTestProvider()
	firstOutput := `{"Action":"output","Package":"example.test/first","Output":"TestShared\n"}`
	input := `{"Action":"start","Package":"example.test/first"}` + "\n" + `{"Action":"start","Package":"example.test/second"}` + "\n" + firstOutput + "\n" + `{"Action":"output","Package":"example.test/second","Output":"TestShared\n"}` + "\n" + `{"Action":"output","Package":"example.test/first","Output":"Test setup completed\n"}` + "\n" + `{"Action":"pass","Package":"example.test/first"}` + "\n" + `{"Action":"pass","Package":"example.test/second"}`
	cases, err := Parse(provider, input)
	if err != nil || len(cases) != 2 || cases[0].ID == cases[1].ID || cases[0].GoPackage == cases[1].GoPackage {
		t.Fatalf("package identities: %+v %v", cases, err)
	}
	if ok, name := IdentifyStreamedTestLine(provider, firstOutput); !ok || name != "TestShared" {
		t.Fatalf("streamed discovery: %v %q", ok, name)
	}
	for _, bad := range []string{`{"Action":`, `{"Action":"output","Output":"TestMissing\n"}`} {
		if _, err := Parse(provider, bad); err == nil {
			t.Fatalf("accepted incomplete discovery %q", bad)
		}
	}
}
