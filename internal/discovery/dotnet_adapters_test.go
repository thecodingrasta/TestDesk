package discovery

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"testing"
)

func TestIdentifyDotNetAdapters(t *testing.T) {
	normal := []domain.TestCase{{FullName: `Example.One.Select("a.b:c")`}, {FullName: `Example.Xunit.Select(value: 1)`}}
	alternate := []domain.TestCase{{FullName: `Example:One:Select("a:b:c")`}, {FullName: `Example.Xunit.Select(value: 1)`}}
	if err := IdentifyDotNetAdapters(normal, alternate); err != nil {
		t.Fatal(err)
	}
	if normal[0].DotNetFilterProperty != "FullyQualifiedName" || normal[1].DotNetFilterProperty != "" {
		t.Fatalf("incorrect adapter identity: %+v", normal)
	}
}

func TestIdentifyDotNetAdaptersRejectsAmbiguousOrIncompleteLists(t *testing.T) {
	for _, tc := range []struct{ normal, alternate []domain.TestCase }{
		{[]domain.TestCase{{FullName: "A.B:C"}, {FullName: "A:B.C"}}, []domain.TestCase{{FullName: "A:B:C"}, {FullName: "A:B:C"}}},
		{[]domain.TestCase{{FullName: "A.B"}}, nil},
		{[]domain.TestCase{{FullName: "A.B"}}, []domain.TestCase{{FullName: "A.B"}, {FullName: "A.B"}}},
	} {
		if err := IdentifyDotNetAdapters(tc.normal, tc.alternate); err == nil {
			t.Fatalf("accepted ambiguous lists: %+v", tc)
		}
	}
}
