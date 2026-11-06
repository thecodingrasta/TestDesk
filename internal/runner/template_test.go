package runner

import (
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// The expected strings were verified against real `dotnet test --filter` runs
// on Windows and Linux, so they must be identical on every platform.
func TestEscapeDotNetFilterBackslashRuns(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", `a.b:c`, `a.b:c`},
		{"quote", `q"x`, `q%22x`},
		{"backslash before quote", `b\"x`, `b\\\%22x`},
		{"two backslashes before quote", `b\\"x`, `b\\\\\\\%22x`},
		{"three backslashes before quote", `b\\\"x`, `b\\\\\\\\\\\%22x`},
		{"backslash in the middle", `a\b`, `a\\b`},
		{"two backslashes in the middle", `a\\b`, `a\\\\b`},
		{"trailing backslash", `trail\`, `trail\\`},
		{"leading quote then backslash", `"\`, `%22\\`},
		{"filter grammar characters", `a,b(c)`, `a%2Cb\(c\)`},
		{"percent and semicolon", `100%;`, `100%25%3B`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := escapeDotNetFilter(test.in); got != test.want {
				t.Fatalf("escapeDotNetFilter(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestDotNetParameterizedFilter(t *testing.T) {
	spec := domain.CommandSpec{Exe: "dotnet", Args: []string{"test", "--filter", "FullyQualifiedName={{test.selector}}"}}
	selector := `Fixture.Cases.Select(value: "json {\"k\":[1,2]} & A|B = C! (~) 100%;")`
	data := TemplateData{Provider: domain.Provider{Kind: domain.ProviderDotNet}, Test: &domain.TestCase{Selector: selector}}
	got, err := RenderCommand(spec, data)
	if err != nil {
		t.Fatal(err)
	}
	// One backslash before a quote becomes 4n-1 = 3 on every operating system.
	backslashes := `\\\`
	want := `DisplayName=Fixture.Cases.Select\(value: %22json {` + backslashes + `%22k` + backslashes + `%22:[1%2C2]} \& A\|B \= C\! \(\~\) 100%25%3B%22\)`
	if got.Args[2] != want {
		t.Fatalf("got %q, want %q", got.Args[2], want)
	}
	data.Test.Selector = "Fixture.Cases.Fact"
	got, err = RenderCommand(spec, data)
	if err != nil || got.Args[2] != "FullyQualifiedName=Fixture.Cases.Fact" {
		t.Fatalf("ordinary test: %+v %v", got, err)
	}
	spec.Args[2] = "FullyQualifiedName=custom-{{test.selector}}"
	data.Test.Selector = "Fixture.Cases.Select(value: 1)"
	got, err = RenderCommand(spec, data)
	if err != nil || !strings.HasPrefix(got.Args[2], "FullyQualifiedName=custom-") {
		t.Fatalf("custom filter: %+v %v", got, err)
	}
}

func TestDotNetProjectScopeOnlyAppliesToBuiltInRun(t *testing.T) {
	spec := domain.CommandSpec{Exe: "dotnet", Args: []string{"test", "{{provider.root}}", "--filter", "FullyQualifiedName={{test.selector}}", "--logger", "trx", "--results-directory", "{{artifact.dir}}"}}
	data := TemplateData{Provider: domain.Provider{Kind: domain.ProviderDotNet, Root: "."}, Test: &domain.TestCase{Selector: "Example.Case", DotNetProject: "path/to/project.csproj"}}
	got, err := RenderCommand(spec, data)
	if err != nil || got.Args[1] != data.Test.DotNetProject {
		t.Fatalf("built-in project scope: %+v %v", got, err)
	}
	spec.Args[5] = "custom-logger"
	got, err = RenderCommand(spec, data)
	if err != nil || got.Args[1] != "." || len(got.Args) != len(spec.Args) {
		t.Fatalf("custom command changed: %+v %v", got, err)
	}
}
