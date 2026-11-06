# Security policy

## Reporting

Report suspected vulnerabilities through [GitHub private vulnerability reporting](https://github.com/thecodingrasta/testdesk/security/advisories/new). If the private reporting form is unavailable, open an issue asking for a private contact without disclosing the vulnerability. Include the affected commit or version, impact and reproduction steps. Redact credentials and personal data.

Maintainers assess reports as availability permits. There is no guaranteed response time or support SLA. Security fixes target the current release and main branch; older builds may require an upgrade.

## Trust boundary

TestDesk is a local developer tool, not a sandbox. Provider commands in `testdesk.json` run with the permissions and environment of the TestDesk process. Adding or running an untrusted repository can execute arbitrary code. Do not run TestDesk as administrator or root for ordinary work.

MCP clients can run configured commands, register local projects and install agent instructions. Only connect trusted clients. The stdio bridge connects to a desktop-owned loopback service with a local token; it is not a public remote API. Do not forward that service or share its lifecycle state/token files.

Runtime state is local. Logs, reports, command arguments and configuration can contain secrets; review and redact diagnostics before sharing them. Use environment variables for sensitive provider configuration and keep `.testdesk/`, local overrides and MCP configuration backups out of Git. External runners may access the network or upload data according to their own settings.

Cancellation controls managed process trees, with platform-specific limits. Programs that escape their process group or job may need separate cleanup. TestDesk cannot make arbitrary test code safe.

Release builds should use a currently patched Go toolchain and pass `govulncheck`. A clean scan covers known advisories and does not prove that no vulnerabilities exist.
