# Contributing

Thanks for helping. Bug reports, fixes, new test-tool support, doc corrections and "this confused me" are all useful, and nothing is too small.

## Easiest ways to help

- **Try TestDesk on a Mac.** Nobody on the project owns one. Install a [release](https://github.com/thecodingrasta/testdesk/releases), run it on a real project and file a [compatibility report](https://github.com/thecodingrasta/testdesk/issues/new?template=compatibility_report.yml) with your macOS version and chip. Working and broken are both useful.
- **Try a test tool we haven't tested.** The [provider table](docs/PROVIDERS.md) lists which ones. Tell us how discovery, running a single test and diagnostics behaved.
- **Report a bug** with the [issue form](https://github.com/thecodingrasta/testdesk/issues/new/choose). Include your TestDesk version, OS and chip, and the smallest project that reproduces it.
- **Fix the docs** when something is wrong, missing or unclear.

For vulnerabilities, use [SECURITY.md](SECURITY.md) instead of a public issue. Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## Set up to change code

You need Go (version in `go.mod`), Node.js 22+, Python 3.12+ and the Wails CLI at the version `go.mod` pins. Platform libraries and exact build commands are in [INSTALL.md](docs/INSTALL.md#build-from-source-advanced). The frontend is plain JavaScript, HTML and CSS with no npm install.

Run the checks the same way CI does:

```bash
bash scripts/validate.sh        # Linux and macOS
```
```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/validate.ps1   # Windows
```

They cover formatting, `go vet`, Go and frontend tests, documentation links, agent-asset drift and version strings. On Linux they use the `webkit2_41` build tag. Use focused `go test` runs while you work and the full script before you open a pull request. `python scripts/package.py` builds the same native package a release ships.

The sample project in `examples/sample-project` uses simulated providers, so you can try TestDesk without installing every framework. It isn't evidence that a real runner works.

Tests against real frameworks are opt-in through `TESTDESK_REAL_*` variables. CI runs them for Cargo, .NET, Jest and Vitest. To run them yourself, install Rust with a working linker, .NET 8 and 10, Node.js 22 and npm 11.6.2, then:

```bash
python scripts/prepare_provider_tests.py --root <new-empty-directory>
```

Add `<that directory>/node/node_modules/.bin` to PATH. Set `TESTDESK_REAL_CARGO_TESTS=1`, `TESTDESK_REAL_DOTNET_TESTS=1`, and point `TESTDESK_JEST_INSTALL` and `TESTDESK_VITEST_FIXTURE` at its `node` folder. Then run `go test ./internal/manager -count=1 -timeout=25m`. On Windows, use a Visual C++ developer shell. The script installs test tools into that directory and fills your user NuGet cache. Pass `--npm-cli path/to/npm-cli.js` to use a separate npm.

## Guidelines

- Keep changes focused and keep existing custom provider configurations working.
- Format Go with `gofmt`. Test the change, including failure and cancellation paths where they matter.
- Keep result evidence and test identity intact. A command that exits 0 doesn't prove the selected test ran.
- Update user documentation when commands, configuration or behaviour change.
- Keep binaries, caches, logs, local preferences, screenshots and private notes out of Git. `python scripts/check_repository.py` and `git diff --check` catch most of it.

A new provider needs tests for discovery, exact selection, passing, failing and skipped outcomes, missing or malformed reports, and paths with spaces. Say which framework versions and platforms you ran. A parser fixture alone doesn't show a framework works.

Agent assets are generated from `internal/agents/skills.go`. After changing them, run `go run ./scripts/sync-agent-assets`. Add `-check` to see drift without writing files.

## Pull requests

Use the pull request template. Describe the problem, the fix, what you ran and what you couldn't run (for instance, "no Mac available"). Leave out credentials, personal paths, generated packages and unrelated cleanup.

Contributions are licensed under [GPL-3.0](LICENSE). Check [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before adding or copying third-party material.

Maintainers: see [Releases](docs/RELEASING.md).
