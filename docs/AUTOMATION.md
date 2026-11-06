# Repository automation

What the GitHub Actions workflows do, and the repository settings they rely on. For the release steps, see [Releases](RELEASING.md).

## Workflows

| Workflow | Runs | Purpose |
| --- | --- | --- |
| [Native validation](../.github/workflows/ci.yml) | Pull requests, pushes to `main`, manual, and as the first step of a release | Tests and validates on Windows, Linux and both macOS architectures, with real-provider tests, a vulnerability scan and workflow linting |
| [Draft release](../.github/workflows/release.yml) | Manual, with an existing tag | Builds the packages, tests each download on a clean runner and creates a draft |
| [CodeQL](../.github/workflows/codeql.yml) | Pull requests, pushes to `main`, weekly | Code scanning for Go, JavaScript and Python |

The real-provider job prepares disposable, version-pinned test tools and a NuGet cache before it turns on the opt-in integration tests. The shipped frontend has no npm dependencies; `scripts/fixtures/providers` is only test tooling.

Validation and CodeQL have read-only tokens. Only the draft-creation job can write repository contents. Actions are pinned by commit, checkout doesn't keep credentials, and fork pull requests get no secrets. No workflow publishes a release or merges anything.

GitHub's [standard hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners) are free for public repositories. Private repositories use the account's Actions allowance. Artifacts expire after 14 days, so the draft release is the permanent copy.

CodeQL's job asks for `actions: read` as well as `security-events: write`. A private repository needs the first so CodeQL can read workflow-run details; without it the run fails with "Resource not accessible by integration".

Code scanning is free for public repositories. A private repository needs a paid [GitHub Code Security](https://docs.github.com/en/code-security/code-scanning/introduction-to-code-scanning/about-code-scanning) licence, and without one the upload fails with "Code scanning is not enabled for this repository". CodeQL's [terms](https://github.com/github/codeql-cli-binaries/blob/main/LICENSE.md) also limit its use to open-source codebases, and give "code in a private repo in GitHub" as an example of what isn't covered, unless you hold a GitHub Advanced Security licence. So the workflow doesn't run the analysis and discard the results.

Instead, each CodeQL job first asks the GitHub API whether the repository is private. If it is, the job posts a notice ("CodeQL not run") on the run summary, skips the analysis steps and finishes green. It starts analysing by itself once the repository is public. If you hold a licence for a private repository, enable code scanning under **Settings → Advanced Security** and set the repository variable `CODEQL_PRIVATE_REPOSITORY` to `true` under **Settings → Secrets and variables → Actions → Variables**.

While the repository is private, the **Vulnerabilities** job (`govulncheck`) is the security scan that runs.

## Dependency and security checks

[Dependabot](../.github/dependabot.yml) opens weekly updates for Go modules, GitHub Actions and the test-only npm fixtures, and groups them. The Wails command-line version is read from `go.mod`, so a Wails update needs no workflow edit. Review compatibility and regenerate third-party notices after Go updates. Vitest major upgrades are held back because the exact-selection adapter targets version 4.

[CodeQL](../.github/workflows/codeql.yml) builds the Go code with the Linux desktop dependencies. If you use this workflow, switch off GitHub's CodeQL default setup so the two don't conflict.

`govulncheck` reports known advisories in production imports. It doesn't replace review or testing.

The test-only Jest fixture inherits [GHSA-gcqx-w48m-w2h4](https://github.com/advisories/GHSA-gcqx-w48m-w2h4), a low-severity `deepmerge` advisory with no patched release. It handles repository-controlled fixture configuration in disposable, unprivileged jobs and isn't shipped. Provider setup rejects moderate or higher npm findings. Overrides for fixable glob and minimatch advisories can go once upstream updates make them unnecessary.

## CodeRabbit

[CodeRabbit's open-source plan](https://www.coderabbit.ai/faq) reviews pull requests for free. [`.coderabbit.yaml`](../.coderabbit.yaml) asks for concrete correctness, security and portability feedback and skips licence texts and generated bindings. A maintainer has to [connect CodeRabbit](https://docs.coderabbit.ai/getting-started/quickstart) and grant its GitHub App access; the file alone doesn't install anything. Draft pull requests aren't reviewed.

## Repository settings

Set these once in GitHub:

- Protect the default branch and require the Native validation jobs (Validate, Real providers, Vulnerabilities, Workflow syntax) before merging.
- Set the default workflow token permission to read-only.
- Turn on private vulnerability reporting, the dependency graph, Dependabot alerts and security updates, secret scanning and push protection.
- Make the repository public to get free code scanning and the live badges in the README.
