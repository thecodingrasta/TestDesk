package discovery

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
)

func TestParseDotNetList(t *testing.T) {
	provider := config.DotNetProvider()
	provider.Discover.Parser = "dotnet-list"
	out := `The following Tests are available:
    Acme.Tests.Users.LoginTests.ValidCredentials_passes
    Acme.Tests.Orders.CreateOrderTests.ValidOrder_passes
`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(tests))
	}
	if tests[0].ProviderID != "dotnet" || tests[0].Layer != "backend" {
		t.Fatalf("unexpected provider metadata: %+v", tests[0])
	}
}

func TestParseDotNetListIgnoresBuildWarnings(t *testing.T) {
	provider := config.DotNetProvider()
	out := `C:\\sdk\\targets(10,2): warning MSB3277: conflicting reference Example.dll [C:\\repo\\src\\App.csproj]
The following Tests are available:
    Acme.Tests.Users.LoginTests.ValidCredentials_passes
    Acme.Tests.Orders.CreateOrderTests.ValidOrder_passes
Test Run Successful.
Total tests: 2
`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected only the two test identifiers, got %d: %+v", len(tests), tests)
	}
	for _, test := range tests {
		if test.Name == "csproj]" || test.Name == "targets(10,2)" {
			t.Fatalf("warning was catalogued as a test: %+v", test)
		}
	}
}

func TestParseDotNetListRejectsBuildOutputAndKeepsParameterizedDisplayNameStable(t *testing.T) {
	provider := config.DotNetProvider()
	out := `Intellagent.UnitTests -> C:\repo\tests\Intellagent.UnitTests\bin\Debug\net10.0\Intellagent.UnitTests.dll
Test run for C:\repo\tests\Intellagent.UnitTests\bin\Debug\net10.0\Intellagent.UnitTests.dll (.NETCoreApp,Version=v10.0)
The following Tests are available:
    Intellagent.UnitTests.AuthenticationEmailLinkParserTests.Parse_RejectsWrongBoundaryOrAmbiguousInput(value: "intellagent://auth/confirm?token_hash=hash&type=recovery")
    Intellagent.UnitTests.ProviderProfileCatalogTests.Catalog_ProvidesProviderConfigurationWithoutStaticModels(providerId: "ollama", endpoint: "http://127.0.0.1:11434/v1")
  Intellagent.UnitTests -> C:\repo\tests\Intellagent.UnitTests\bin\Debug\net10.0\Intellagent.UnitTests.dll
`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected two test identifiers, got %d: %+v", len(tests), tests)
	}
	if tests[0].Name != "Parse_RejectsWrongBoundaryOrAmbiguousInput" || tests[1].Name != "Catalog_ProvidesProviderConfigurationWithoutStaticModels" {
		t.Fatalf("expected method-only display names, got %+v", tests)
	}
	for _, test := range tests {
		if test.FullName != test.Selector || test.FullName == "" {
			t.Fatalf("expected full identifier selector, got %+v", test)
		}
		if test.Name == "dll" || test.Name == "v1\")" || test.Name == "confirm?token_hash=hash&type=recovery\")" {
			t.Fatalf("non-test build output or parameter value was catalogued: %+v", test)
		}
	}
}

func TestParseDotNetListInfersModulesOnlyFromQualifiedIdentifiers(t *testing.T) {
	provider := config.DotNetProvider()
	out := `The following Tests are available:
    Intellagent.UnitTests.ProviderProfileCatalogTests.Catalog_ProvidesProviderConfigurationWithoutStaticModels(providerId: "ollama", endpoint: "http://127.0.0.1:11434/v1")
    Intellagent.UnitTests.NativeScreenshotCaptureTests.ResolvePixelSize_ProducesPositiveCeilingBounds(width: 10.1, height: 20.100000000000001, expectedWidth: 11, expectedHeight: 21)
    Intellagent.UnitTests.AuthenticationEmailLinkParserTests.Parse_RejectsWrongBoundaryOrAmbiguousInput(value: "intellagent://auth/confirm?token_hash=hash&type=recovery")
    Intellagent.UnitTests.AuthoritativeProtectedOperationServiceTests.AuthorizeAsync_PreservesTrustedLicenceFailureReason(licenceReason: "Licence expired.")
    Intellagent.UnitTests.SupabaseAuthenticationClientTests.SignInAsync_RejectsEitherMissingCredentialWithoutTransport(email: "user@example.com")
`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 5 {
		t.Fatalf("expected five parameterised tests, got %d: %+v", len(tests), tests)
	}
	wantModules := map[string]string{
		"Catalog_ProvidesProviderConfigurationWithoutStaticModels":   "ProviderProfileCatalogTests",
		"ResolvePixelSize_ProducesPositiveCeilingBounds":             "NativeScreenshotCaptureTests",
		"Parse_RejectsWrongBoundaryOrAmbiguousInput":                 "AuthenticationEmailLinkParserTests",
		"AuthorizeAsync_PreservesTrustedLicenceFailureReason":        "AuthoritativeProtectedOperationServiceTests",
		"SignInAsync_RejectsEitherMissingCredentialWithoutTransport": "SupabaseAuthenticationClientTests",
	}
	for _, test := range tests {
		want, ok := wantModules[test.Name]
		if !ok {
			t.Fatalf("unexpected parsed test: %+v", test)
		}
		if test.Module != want {
			t.Fatalf("%s module = %q, want %q", test.Name, test.Module, want)
		}
	}
}

func TestParseDotNetListNestedAndGenericClasses(t *testing.T) {
	provider := config.DotNetProvider()
	out := `The following Tests are available:
    MyProject.UnitTests.AccountServiceTests+CreateAccount.ReturnsSuccess
    MyProject.UnitTests.GenericServiceTests.Execute<string, int>()
    SingleNamespace.SimpleTest
    MyProject.Tests.ScenarioTests+NestedScenario+SubNested.AssertAll
`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, test := range tests {
		names[test.Name] = true
	}
	if !names["ReturnsSuccess"] || !names["Execute<string, int>"] || !names["SimpleTest"] || !names["AssertAll"] {
		t.Fatalf("unexpected test names: %+v", tests)
	}
}

func TestParseDotNetListPreservesRestoreMethodsAndTheoryInstances(t *testing.T) {
	out := `The following Tests are available:
    Intellagent.UnitTests.GovernedWorkspaceFileServiceTests.ExecuteAsync_AuthoritativeProjectGrantAllowsRetryAndTombstoneRestoresPrompt
    Intellagent.UnitTests.PluginCatalogServiceTests.RestoreAsync_AppliesOnlyPersistedInstalledPluginEnablement
    Intellagent.WindowsIntegrationTests.SqliteMigrationIntegrationTests.BackupRestore_RecoversStateAndRejectsCorruptInputWithoutMutation
    Intellagent.UnitTests.AuthenticationEmailLinkParserTests.Parse_RejectsWrongBoundaryOrAmbiguousInput(value: "intellagent://auth/confirm?token_hash=hash&type=re"···)
    Intellagent.UnitTests.AuthenticationEmailLinkParserTests.Parse_RejectsWrongBoundaryOrAmbiguousInput(value: "intellagent://auth/confirm?token_hash=hash&type=re"···)
`
	provider := config.DotNetProvider()
	provider.Discover.Parser = "dotnet-list"
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 5 {
		t.Fatalf("expected 5 tests preserving restore methods and theory instances, got %d: %+v", len(tests), tests)
	}
	if tests[3].ID == tests[4].ID {
		t.Fatalf("expected duplicate theory signatures to receive distinct stable IDs: id1=%s id2=%s", tests[3].ID, tests[4].ID)
	}
}

func TestInferModuleUsesFilePathsAndRejectsBareTestNames(t *testing.T) {
	if got := inferModule("checkout/cart.spec.ts", "Cart > adds item"); got != "checkout" {
		t.Fatalf("file-backed module = %q, want checkout", got)
	}
	if got := inferModule("", "crate::module::test_name"); got != "module" {
		t.Fatalf("qualified module = %q, want module", got)
	}
	if got := inferModule("", "TestStandalone"); got != "general" {
		t.Fatalf("bare test module = %q, want general", got)
	}
}

func TestParsePlaywrightList(t *testing.T) {
	provider := config.PlaywrightProvider()
	out := `[chromium] › tests/login.spec.ts:12:5 › Login screen › accepts valid user
[firefox] › tests/orders.spec.ts:23:1 › Orders screen › opens detail
Total: 2 tests in 2 files`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(tests))
	}
	if tests[0].File == "" || tests[0].Line == 0 {
		t.Fatalf("expected file and line metadata: %+v", tests[0])
	}
}

func TestParseVitestJSONFlexibleShape(t *testing.T) {
	provider := config.VitestProvider()
	out := `[{"name":"CartScreen","type":"suite","tasks":[{"name":"adds item","type":"test","filepath":"checkout/cart.spec.ts","line":12}]}]`
	tests, err := Parse(provider, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 {
		t.Fatalf("expected 1 test, got %d", len(tests))
	}
	if tests[0].Name != "adds item" || tests[0].Module != "checkout" {
		t.Fatalf("unexpected vitest parse: %+v", tests[0])
	}
}
