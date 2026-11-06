# Test providers

TestDesk provides built-in provider templates for common runners plus a configurable provider schema for bespoke commands. A provider template or automatic detection path is not, by itself, proof of broad compatibility across every framework version and operating system.

## Compatibility

| Tool | Ecosystem | Discovery | Single-test run | Tested with |
| --- | --- | --- | --- | --- |
| Go test | Go | Yes | Yes | Unit tests and manager flows in CI |
| .NET (xUnit, NUnit, MSTest) | C#, F#, .NET | Yes | Yes | Real `dotnet test` projects in CI on Windows, Linux and macOS |
| Playwright | Node.js | Yes, per spec | Yes | Parser fixtures |
| Vitest | JavaScript, TypeScript | Yes, flat JSON | Vitest 4 only | A real Vitest 4 fixture in CI |
| Jest | JavaScript, TypeScript | Yes | Yes | A real Jest fixture in CI |
| pytest | Python | Yes | Yes | Not yet tested on real projects. Reports welcome |
| Cargo test | Rust | Yes | Standard libtest targets | Real Cargo fixtures in CI |
| Maven, Gradle | Java, Kotlin | Needs configured discovery | Yes | Not yet tested on real projects. Reports welcome |
| Cypress | Web end-to-end | Needs configured discovery | Yes | Not yet tested on real projects. Reports welcome |
| Custom script or executable | Any | Configurable | Configurable | Configurable |

"Tested in CI" means the real-provider job runs on Windows, Linux and macOS (Intel and Apple silicon) with the tool versions pinned in `scripts/prepare_provider_tests.py`. Other versions of a tool may work; they haven't been checked.

Vitest's built-in selection isolates one project, file and case. It needs Vitest 4 and fails closed on other major versions. Maven, Gradle and Cypress have no discovery parser in their built-in templates, so only whole-provider runs work until you configure discovery.
The built-in .NET provider performs a second, no-build discovery pass to distinguish NUnit case identities from other adapters. For a solution with mixed adapters, it also identifies the owning project. If a project contains both NUnit and another adapter, generated targeted commands use NUnit's `DiscoveryMethod=Legacy` setting so its current filter parser does not reject the other adapter's case filter. If identities or project ownership cannot be matched unambiguously, discovery reports an error and retains that provider's previous catalog. Identical selectors within a provider cannot be targeted individually. Customized discovery and run commands remain under the provider author's control. Real-project tests cover individual xUnit and NUnit parameterized cases in separate and shared projects with NUnit adapter 4.5.0, on the .NET 8 and 10 runtimes with the .NET 10 SDK. Other adapter versions need their own check.

Multi-target .NET runs can report the same case name once per target framework. TestDesk counts each report record as an execution, but marks catalog entries with indistinguishable identities as unknown for that run instead of assigning an outcome to an arbitrary entry.

---

## Report compatibility

If you use a tool the table marks as untested, or a version it doesn't list, telling us how it went helps everyone:

1. Add your project in TestDesk (**Projects**, or `testdesk_project_add` over MCP).
2. Review the detected providers in `testdesk.json`.
3. Discover and run tests from the UI or through your agent.
4. Open a [compatibility report](https://github.com/thecodingrasta/testdesk/issues/new?template=compatibility_report.yml), or send a pull request. See [CONTRIBUTING.md](../CONTRIBUTING.md).

---
## Provider Configuration Schema

Each provider in `testdesk.json` is configured with clear, reproducible command definitions:

```json
{
  "version": 1,
  "name": "My Project",
  "workspace": ".testdesk",
  "providers": [
    {
  "id": "unit-tests",
  "name": "Unit Tests",
  "kind": "go-test",
  "layer": "unit",
  "root": ".",
  "enabled": true,
  "discover": {
    "command": {
      "exe": "go",
      "args": ["test", "-json", "./...", "-list", "."],
      "timeoutSeconds": 120
    },
    "parser": "go-json-list"
  },
  "run": {
    "all": {
      "exe": "go",
      "args": ["test", "./...", "-json"],
      "timeoutSeconds": 1800
    },
    "test": {
      "exe": "go",
      "args": ["test", "{{test.goPackage}}", "-run", "^{{test.selector}}$", "-json"],
      "timeoutSeconds": 900
    }
  },
  "results": {"format": "go-json", "path": ""},
  "tags": ["core", "fast"]
    }
  ]
}
```

The Go discovery stream supplies a package identity for each test. After upgrading an older generated Go provider, rediscover before selecting an existing test: older catalog entries do not contain the package needed to target it safely. Custom provider commands are preserved. Selected Go benchmarks run with `-bench` in their owning package; successful results require measurement output and a successful package result.

The built-in Cargo provider builds test artifacts and lists each standard libtest target separately. Selected runs specify the package and target and use an exact case name; they require a matching case result, so a zero-match exit is not treated as a pass. Rediscover after upgrading an older generated provider: old catalog entries lack the target identity. Custom Cargo commands are preserved and require their own selection and result-evidence review. Documentation tests are not currently discovered as individual cases. Custom harnesses, additional target types and platforms have not been qualified; unsupported artifact kinds or nonstandard listings fail discovery rather than replacing the prior catalog.

### Result Formats
TestDesk supports structured parsing for:
- `go-json` - Standard Go test streaming JSON
- `vitest-json` - Vitest JSON reporter
- `junit` - Standard JUnit XML test reports
- `trx` - .NET Visual Studio test result format
- `trx-dir` - TRX reports in a directory
- `cargo-text` - Standard libtest text results
- `junit-dir` - JUnit XML files in a result directory
- `none` - Keep process status without structured result parsing

## Version-specific selection

Vitest built-in targeted execution uses the version 4 programmatic API to select one file, named project and collected case. Other major versions fail closed. Generated Jest commands use exact file paths and named-project selection; they require the configured JUnit reporter and report identity. Customized commands remain the provider author's responsibility. Python Playwright is not auto-detected by the Node.js Playwright template; configure it as a custom provider.
