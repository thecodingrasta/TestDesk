package discovery

import "testing"

func TestGoJSONProgressKeepsInterleavedPackagesSeparate(t *testing.T) {
	var progress GoJSONProgress
	for _, raw := range []string{
		`{"Action":"output","Package":"a","Output":"TestFi"}`,
		`{"Action":"output","Package":"b","Output":"TestSe"}`,
	} {
		if names := progress.Push(raw); len(names) != 0 {
			t.Fatalf("reported incomplete name: %v", names)
		}
	}
	if names := progress.Push(`{"Action":"output","Package":"a","Output":"rst\nTestNext\n"}`); len(names) != 2 || names[0] != "TestFirst" || names[1] != "TestNext" {
		t.Fatalf("first package: %v", names)
	}
	if names := progress.Push(`{"Action":"output","Package":"b","Output":"cond\n"}`); len(names) != 1 || names[0] != "TestSecond" {
		t.Fatalf("second package: %v", names)
	}
}
